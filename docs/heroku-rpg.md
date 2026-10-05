# RPG production di Heroku

Hararest melayani halaman browser. Kotonehara menjalankan bot WhatsApp dan API RPG dalam **satu dyno web**. Database bot dan RPG memakai PostgreSQL yang sama. Dev VPS tetap memakai Docker dan SQLite pada volume permanen.

```text
Pemain → https://api.oohara.dev/rpg/
       → Hararest → HTTPS domain aplikasi Kotonehara → PostgreSQL
```

## Config Vars

Di aplikasi **Kotonehara**, pertahankan `DATABASE_URL` PostgreSQL yang sudah berisi data bot. Tambahkan:

```dotenv
DB_DRIVER=postgres
RPG_ENABLED=true
RPG_PUBLIC_URL=https://api.oohara.dev
RPG_GATEWAY_SECRET=<secret acak minimal 32 karakter>
```

`RPG_PUBLIC_URL` adalah origin **Hararest**, tanpa `/rpg`. Generate secret sekali dengan `openssl rand -hex 32`, lalu gunakan nilai yang sama pada kedua aplikasi. Jangan masukkan secret ke Git.

Jangan menetapkan `PORT` sendiri: Heroku memberikannya saat dyno berjalan. Kode otomatis memakai `0.0.0.0:<PORT>`, dengan prioritas di atas `RPG_LISTEN_ADDR` lama. `RPG_LISTEN_ADDR=0.0.0.0:8089` hanya diperlukan di dev VPS. Nilai literal `0.0.0.0:$PORT` dalam Config Vars tidak diperlukan.

Di aplikasi **Hararest**:

```dotenv
RPG_UPSTREAM_URL=https://<domain-aplikasi-kotonehara>
RPG_GATEWAY_SECRET=<secret yang sama>
```

Gunakan domain HTTPS aplikasi bot yang ditampilkan Heroku, tanpa `/rpg/api`; jangan memakai domain Hararest sebagai upstream karena akan membuat proxy berulang. Hararest tetap memakai `PORT` dari Heroku untuk servernya sendiri. Koneksi RPG ini tidak membutuhkan Tailscale. Integrasi bot lain yang memakai Tailscale tetap dapat menggunakan `TAILSCALE_AUTHKEY`; tanpa variabel itu, entrypoint langsung menjalankan bot.

## Deploy dan perpindahan worker → web

Isi Config Vars sebelum merge PR ke `main`. Workflow `main.yml` berjalan pada push `main`, bukan saat pull request. Workflow dev tetap terpisah dan tidak mengubah production.

Deployment bot:

1. Menjalankan tes SQLite, seluruh suite RPG PostgreSQL sementara, race detector, vet, dan build Docker.
2. Memvalidasi config production tanpa mencetak secret. PostgreSQL wajib; config RPG diperiksa hanya bila RPG diaktifkan.
3. Menghentikan worker dan web lama bila ada, sebelum release image web. Ada jeda layanan singkat yang disengaja untuk mencegah dua bot WhatsApp berjalan bersamaan.
4. Release image `web`, menjalankan `web=1`, dan memeriksa `/health`. Worker lama tetap `0`.

Workflow tidak membuat database baru, mengganti `DATABASE_URL`, atau memindahkan data. Migration hanya menambah tabel/index `rpg_*` yang belum ada. **Data RPG SQLite dev tidak otomatis disalin ke PostgreSQL production.** Jangan mengganti koneksi database lama dengan database kosong.

Jika release gagal setelah proses lama dihentikan, workflow berstatus gagal dan tidak menghidupkan worker otomatis. Periksa log deployment dan Heroku, perbaiki penyebabnya, lalu deploy ulang. `workflow_dispatch` hanya menjalankan pemeriksaan, bukan deploy; deploy ulang melalui push `main` atau perintah release manual yang sama. Jangan menyalakan worker bersama web. Jangan memakai autoscaling/multiple web dynos untuk bot ini.

`RPG_ENABLED=false` tetap menyediakan `/health` pada `PORT`, tetapi endpoint RPG memberi 503. `/health` melaporkan HTTP dan inisialisasi database, **bukan status koneksi WhatsApp**. Login WhatsApp tetap mengikuti prosedur bot yang sudah ada.

## Verifikasi setelah deploy

- Buka `https://<domain-aplikasi-kotonehara>/health`: `status: ok`, `rpg_enabled: true` jika fitur aktif.
- Akses langsung `/rpg/api/profile` tanpa gateway secret harus mendapat 401.
- Kirim `.rpg lanjut` pada bot, lalu buka link pribadi yang diberikan. Login, gacha, dan battle diakses melalui origin Hararest.
- Setelah restart dyno, profil/battle/saldo tetap tersimpan pada PostgreSQL.

API RPG tetap memerlukan gateway secret, sesi pemain, serta origin/CSRF untuk mutasi. Cookie dibuat untuk browser pada domain Hararest; pemain tidak membuka domain bot untuk login.

## Tes PostgreSQL lokal

Suite tidak membaca `.env`, `.env.test`, atau `DATABASE_URL`. Gunakan database sementara bernama `rpg_test` pada loopback:

```sh
docker run --rm -d --name rpg-pg-test \
  -e POSTGRES_DB=rpg_test -e POSTGRES_PASSWORD=rpg-test-only \
  -p 127.0.0.1:15432:5432 postgres:16-alpine
# Tunggu pg_isready berhasil sebelum menjalankan test.
docker exec rpg-pg-test pg_isready -U postgres -d rpg_test
RPG_TEST_POSTGRES_URL='postgres://postgres:rpg-test-only@127.0.0.1:15432/rpg_test?sslmode=disable' \
  go test -race -count=1 ./internal/rpg
docker stop rpg-pg-test
```

Setiap fixture memiliki schema sementara sendiri. Suite yang sama juga berjalan di SQLite tanpa variabel tersebut. Tes meliputi migrasi berulang, restart koneksi, rollback, ekonomi, battle, sesi, dan replay request lintas dua instance service. PostgreSQL memakai transaction advisory lock untuk menserialisasi perubahan RPG; pembacaan snapshot menggunakan repeatable read.

Referensi: [runtime container Heroku](https://devcenter.heroku.com/articles/container-registry-and-runtime), [port web dyno](https://devcenter.heroku.com/articles/dyno-startup-behavior), [filesystem sementara](https://devcenter.heroku.com/articles/dyno-isolation).
