# HARA: Gema Arunika

Service Go `internal/rpg` mengelola profil, battle, gacha, tiket login, dan sesi browser di SQLite bot yang sama. Command `.rpg` sudah terdaftar; aktifkan service hanya setelah env koneksi diisi.

## Env VPS dev

Tambahkan ke `/opt/oohara/kotonehara-dev.env`:

```dotenv
RPG_ENABLED=true
RPG_PUBLIC_URL=https://DOMAIN-HARAREST-DEV
RPG_LISTEN_ADDR=0.0.0.0:8089
RPG_GATEWAY_SECRET=SECRET-YANG-SAMA
```

Buat secret sekali dengan `openssl rand -hex 32`. Di env Hararest, isi `RPG_UPSTREAM_URL=http://kotonehara-dev:8089` dan secret yang sama. Public URL adalah origin Hararest tanpa `/rpg`; jangan memakai hostname Docker untuk link pemain. Untuk dev lewat Tailscale, gunakan `RPG_PUBLIC_URL=http://100.89.85.96:1338` dan perangkat pemain harus terhubung ke tailnet. HTTP hanya diterima pada loopback atau IP Tailscale (`100.64.0.0/10`, `fd7a:115c:a1e0::/48`); alamat publik tetap memerlukan HTTPS. API8089 cukup di network Docker bersama dan tidak perlu dipublish ke host. Default fitur mati, listener lokal127.0.0.1:8089.

Versi1 memerlukan `DB_DRIVER=sqlite`. Pastikan **database yang sudah berisi data** benar-benar tersimpan pada `/app/data`, yang dipasang dari `/opt/oohara/kotonehara-data`. Jangan mengganti URI DB sebelum memindahkan/backup data lama secara benar. Migration menambah tabel `rpg_*` tanpa reset data bot. Backup/restore harus mencakup database ini secara konsisten.

## Command dan konten

- `.rpg mulai`, `.rpg`, `.rpg jelajah`, `.rpg lanjut`: profil + link login pribadi, berlaku2 menit dan sekali pakai.
- `.rpg profil`, `.rpg tim`: progres dan komposisi.
- `.rpg tas`: koin, Debu Bintang, XP latihan, tingkat upgrade dan jumlah equipment yang dipasang.
- `.rpg gacha 1` / `.rpg gacha 10`: wallet/pity sama dengan web; ID pesan melindungi terhadap replay.
- `.rpg peluang`, `.rpg riwayat`: aturan dan hasil.

Profil awal mendapatkan empat karakter dan1.600 Embun Bintang sekali. Ada60 karakter,100 spesies,999 jalur berurutan pada10 wilayah. First-clear biasa20 Embun, boss200; replay tanpa hadiah. Battle disimpan setiap aksi dan berlanjut setelah restart. Gacha160 per tarikan, pity4+ ke10, soft pity5 mulai61, hard pity80, unggulan50/50 dan guarantee setelah gagal.

Battle baru memakai aturan `arunika-v3`: 60 skill aktif per karakter, Bara, perisai, Tanda, pelemahan ATK/DEF, Lambat, Kabut, regenerasi dan refleksi. Lambat mengurangi damage serangan musuh 20%, bukan mengubah urutan fase. Deskripsi skill runtime berasal dari `skills.go`. Seluruh 60 karakter memiliki passive aktif otomatis, tanpa syarat awakening; efek dan angka runtime berasal dari `passives.go`. Sebagian rancangan lama memakai kontrol/kecepatan yang belum ada dalam engine; passive tersebut diterjemahkan menjadi perisai, energi atau pengurangan damage dengan deskripsi konkret pada katalog web. Battle `arunika-v1` dan `arunika-v2` yang sudah tersimpan tetap memakai aturan sebelumnya sampai selesai; passive tidak diterapkan pada battle lama.

Level tersimpan per karakter. Kemenangan pertama memberi 100 XP kepada setiap anggota tim (termasuk yang gugur), 100 XP latihan cadangan, serta hadiah koin/Embun yang sama seperti sebelumnya. 100 XP menaikkan satu level. Latihan manual memakai 100 XP cadangan +10 koin per level, dengan pilihan +1/+10. Batas level adalah `min(999, max(10, unlocked+1))`. Replay, kalah, dan mundur tidak memberikan XP/koin tambahan. Profil lama di-upgrade secara aditif; level karakter lama dan karakter baru dari gacha dimulai pada level jalur yang terbuka. Saldo, koleksi, dan battle tidak direset.

Bengkel menjual 9 equipment menggunakan koin: tiga slot (weapon, armor, charm), tiga tier (awal, setelah jalur99, setelah jalur399). Semua role dapat memakainya. Satu salinan hanya boleh terpasang pada satu karakter; salinan tambahan dibeli terpisah, maksimum60 per jenis. Equipment menambah HP/ATK/DEF saat battle dimulai. Latihan, pembelian dan pergantian equipment ditolak saat battle aktif, menggunakan revision + request ID untuk mencegah debit ganda. Endpoint baru: `POST /rpg/api/characters/train`, `POST /rpg/api/equipment/buy`, `PUT /rpg/api/equipment/equip`; semuanya memakai sesi, Origin dan CSRF yang sama.

Awakening A0–A5 memakai Debu Bintang dari duplikat gacha dan koin. Untuk naik ke tahap `n`, biaya adalah `20 × rarity × n` Debu dan `100 × n` koin. Setiap tahap menambah 5% HP/ATK/DEF dasar karakter (maksimum 25%), sebelum bonus equipment; jumlah salinan dan kepemilikan karakter tidak dikurangi. Tidak mengubah rarity atau level. Endpoint: `POST /rpg/api/characters/awaken`.

Upgrade equipment +0–+5 bersifat **per jenis equipment**, sehingga berlaku untuk seluruh salinan yang dimiliki maupun dibeli kemudian. Untuk naik ke `n`, biaya `harga dasar item × n` koin dan `5 × n` Debu. Tiap tahap menambah 10% stat dasar equipment (maksimum 50%), dibulatkan ke integer terdekat, stat nol tetap nol. Item harus dimiliki; tidak ada peluang gagal atau konsumsi salinan. Endpoint: `POST /rpg/api/equipment/enhance`. UI mengambil aturan biaya/batas dari katalog server. Awakening dan upgrade ditolak saat battle aktif, memakai transaksi revision + request ID yang sama untuk retry aman. Profil v3 menambah map `awakening` dan `enhancements` tanpa mengubah saldo lama.

Passive dengan batas sekali per battle/ronde menyimpan penanda dalam snapshot; reload tidak mereset batasnya. Perisai tidak ditumpuk: ambil nilai terbesar, maksimum 50% HP. Pemulihan dari perisai pecah tidak menghidupkan karakter gugur. Bonus Tanda khusus menggantikan bonus dasar; peningkatan energi tetap dibatasi 5.

Frontend aktif berada di `public/rpg/` Hararest, sedangkan demo `docs/rpg/prototype/preview.html` tidak terhubung ke akun. Hararest menyediakan gambar untuk 60 karakter, 100 spesies musuh, dan 10 arena wilayah melalui `public/rpg/assets/index.json`. Gambar dipakai di battle, koleksi, gacha, detail karakter, dan peta. Prompt serta catatan generasi tersedia di `public/rpg/assets/manifest.json` dan folder `provenance` Hararest. Berkas gambar menggunakan path versi dengan cache immutable; index selalu diperbarui tanpa cache.

## Pengujian

```bash
go test ./...
go test -race ./internal/rpg ./internal/handlers
```

Browser integration test dijalankan dari Hararest dengan `RPG_BROWSER=/usr/bin/google-chrome npm run test:rpg-browser`; checkout ini dicari sebagai `../kotonehara` atau melalui `KOTONEHARA_DIR`. Fixture test memakai database sementara dan tidak login WhatsApp. Pengiriman link pribadi/replay command diperiksa dengan handler test.

Panduan frontend dan pengujian tersedia di README repo Hararest. Aset beserta prompt generasinya dicatat di `public/rpg/assets/README.md` pada repo tersebut.
