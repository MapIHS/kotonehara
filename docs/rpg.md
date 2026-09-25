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
- `.rpg gacha 1` / `.rpg gacha 10`: wallet/pity sama dengan web; ID pesan melindungi terhadap replay.
- `.rpg peluang`, `.rpg riwayat`: aturan dan hasil.

Profil awal mendapatkan empat karakter dan1.600 Embun Bintang sekali. Ada60 karakter,100 spesies,999 jalur berurutan pada10 wilayah. First-clear biasa20 Embun, boss200; replay tanpa hadiah. Battle disimpan setiap aksi dan berlanjut setelah restart. Gacha160 per tarikan, pity4+ ke10, soft pity5 mulai61, hard pity80, unggulan50/50 dan guarantee setelah gagal.

Versi awal memakai skill per role dan elemen; level tim mengikuti jalur. Skill/passive unik, equipment, leveling XP terpisah, dan gambar final masih pengembangan. Frontend aktif berada di `public/rpg/` Hararest, sedangkan demo `docs/rpg/prototype/preview.html` tidak terhubung ke akun. Catalog runtime berada di `internal/rpg/catalog/*.json`; perubahan catalog harus memeriksa kesesuaian RulesVersion dengan battle yang tersimpan.

## Pengujian

```bash
go test ./...
go test -race ./internal/rpg ./internal/handlers
```

Browser integration test dijalankan dari Hararest dengan `RPG_BROWSER=/usr/bin/google-chrome npm run test:rpg-browser`; checkout ini dicari sebagai `../kotonehara` atau melalui `KOTONEHARA_DIR`. Fixture test memakai database sementara dan tidak login WhatsApp. Pengiriman link pribadi/replay command diperiksa dengan handler test.

Panduan rinci di repo Hararest: `docs/rpg/integration.md` (API, retry, transaksi, keamanan sesi, dan konfigurasi HTTPS).
