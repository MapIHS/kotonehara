# Message store

Store aktif secara default. Migrasi idempotent membuat tabel `bot_messages` pada
database bot yang sama (PostgreSQL production, SQLite dev). Tidak mengubah tabel
`whatsmeow_*` dan tidak membuat koneksi pool baru.

## Pengaturan awal

| Variabel | Default |
| --- | --- |
| `MESSAGE_STORE_ENABLED` | `true` |
| `MESSAGE_STORE_RETENTION` | `24h` |
| `MESSAGE_STORE_CACHE_BYTES` | 4194304 (4 MiB) |
| `MESSAGE_STORE_QUEUE_BYTES` | 4194304 (4 MiB) |

Ini batas awal, bukan hasil pengukuran kapasitas production. Durasi nol/negatif
kembali ke 24h; byte tidak valid/nol/negatif kembali ke 4 MiB. Gunakan
`MESSAGE_STORE_ENABLED=false` untuk mematikan. Data lama tidak langsung dihapus;
cleanup juga berhenti saat store nonaktif.

Cache dibatasi 2.000 entri dan antrean 1.000 entri, selain anggaran byte.
Accounting memakai ukuran payload + string metadata + allowance overhead 1 KiB
per entri; bukan pengukuran heap/RSS persis. Batch yang sedang ditulis tetap
dihitung sebagai pending. Cache dan antrean berbagi snapshot immutable, tetapi
menghitung anggaran masing-masing secara konservatif. Snapshot protobuf maksimal
128 KiB; pesan lebih besar dilewati, termasuk quote/thumbnail yang sangat besar.

## Data yang disimpan

Protobuf mentah, account/chat/sender/alternatif PN-LID, arah pesan, timestamp,
flag view-once, serta parent/index album. Media key, hash, direct path, dan
thumbnail dipertahankan. File attachment tidak otomatis diunduh.

Hanya event live `events.Message`. Status/broadcast, newsletter, reaction,
protocol message, edit, dan pesan kedaluwarsa dilewati. Revoke/edit tidak
menimpa pesan asli. Duplikat memakai `ON CONFLICT DO NOTHING`. HistorySync tidak
diimpor. Pesan yang dikirim langsung melalui API bot belum memiliki send hook;
jangan mengandalkan store untuk seluruh pesan keluar.

## Performa dan kegagalan

`Devices` memanggil `Capture` sebelum parser/antrean command. Capture melakukan
size check dan serialisasi snapshot terbatas, tanpa query DB atau goroutine baru
per pesan. Tetap ada biaya CPU/mutex. Saat antrean penuh, penyimpanan ditolak,
counter `Dropped` naik, dan pemrosesan command dilanjutkan.

Satu worker menulis hingga 50 pesan / sekitar 512 KiB per batch, atau saat timer
1 detik berjalan. Satu pesan yang melebihi anggaran batch boleh ditulis sendiri
selama memenuhi batas per pesan/antrean. Query diberi timeout 2 detik. Cleanup
dan sampling ukuran DB berjalan serial dengan writer. Pembacaan command berbagi
pool utama dan juga diberi timeout; worker tunggal tidak mengisolasi I/O server.

Batch gagal dicatat dan dilepas dari pending, tanpa retry tanpa batas. Pesannya
masih dapat dibaca selama berada di cache. Worker mencoba batch baru setelah
jeda minimal satu interval flush. Duplikat yang masih di cache tidak otomatis
mengulang penulisan. Restart/crash dapat kehilangan batch yang belum commit.

SIGTERM menyediakan hingga 25 detik drain command dan hingga 5 detik flush store,
menggunakan context flush terpisah. Deadline membatalkan query. Flush bersifat
best effort; kegagalan tulis tetap terlihat pada log/counter. Capture setelah
shutdown ditolak.

## API fitur

`clients.Client.Messages` menyediakan:

- `Get(ctx, msgstore.Key{...})`: cache/pending dahulu, kemudian DB. Miss memakai
  `sql.ErrNoRows`; hasil ambigu memakai `msgstore.ErrAmbiguous`. Sertakan account,
  chat, sender, ID, dan `FromMe` bila diketahui. Key kurang lengkap memerlukan
  pemeriksaan DB juga.
- `Album(ctx, key, parentID, limit)`: gabungkan DB dengan cache/pending, urutkan
  berdasarkan index/waktu/ID. Limit maksimal 100. Hasil dapat belum lengkap ketika
  anggota album masih dikirim WhatsApp. Operasi ini memerlukan DB.
- `Stats()`: counter dan accounting memori saat ini, tanpa query.

Account memakai PN bot bila tersedia. Chat/sender memakai JID `ToNonAD` lengkap
dengan namespace. Alias PN/LID hanya dari event/resolver identitas; angka yang
sama pada namespace berbeda tidak dianggap alias. Hasil Get di-unmarshal menjadi
objek baru, sehingga caller tidak dapat mengubah cache.

Command `IsMedia` mencoba memulihkan quote dengan metadata download yang belum
lengkap sebelum validasi media. Parser juga membuka wrapper view-once pada quote.
Tidak menambah SELECT store untuk setiap pesan biasa. Belum ada command
anti-delete atau konversi seluruh album; API ini fondasinya. Command
[RVO](rvo.md) memakai store untuk memulihkan pesan sekali lihat.

View-once hanya dapat dipulihkan jika bot menerima event asli yang lengkap.
Store metadata tidak menjamin file masih tersedia di server WhatsApp. Tidak ada
mekanisme permintaan upload ulang media pada perubahan ini.

## Retensi dan pemantauan

Retensi dihitung dari waktu pesan; pesan offline yang sudah tua tidak disimpan
ulang. Waktu pesan di masa depan dibatasi ke waktu penerimaan. Read menolak
data kedaluwarsa termasuk cache. Worker menghapus maksimal 1.000 baris
kedaluwarsa tiap 5 menit melalui indeks expiry. Backlog bisa memerlukan beberapa
putaran. Retensi bukan hard cap ukuran database. Postgres memerlukan autovacuum
untuk memakai ulang ruang; DELETE tidak selalu mengecilkan file DB.

Log per menit mencatat accepted/skipped/dropped/written, error, pending/cache
bytes, cache/DB hits, durasi batch terakhir, serta `DB.Stats()` (koneksi dan waktu
tunggu kumulatif seluruh pool). Pada PostgreSQL satu query tambahan per menit
mengukur ukuran seluruh database serta tabel+indeks store. `.ceksystem`
menampilkan ringkasan store tanpa query tambahan.

Periksa pertumbuhan DB, RSS dyno, antrean, serta respons command sebelum
memperbesar retensi. Batas cache/antrean tidak membatasi ukuran database.

## Tes

```sh
go test -race ./internal/msgstore ./internal/message ./internal/devices ./internal/commands
```

Suite database yang sama mendukung PostgreSQL sementara:

```sh
MESSAGE_STORE_TEST_POSTGRES_URL='postgres://postgres:rpg-test-only@127.0.0.1:5432/rpg_test?sslmode=disable' \
  go test -race -count=1 ./internal/msgstore
```

Fixture hanya menerima database loopback bernama `rpg_test`, memakai schema
sementara per tes, dan tidak membaca `.env` atau `DATABASE_URL`. Workflow main
menjalankannya pada PostgreSQL CI sebelum deploy.
