# Kotonehara

Kotonehara adalah bot WhatsApp berbasis Go yang dibangun dengan [`whatsmeow`](https://github.com/tulir/whatsmeow). Bot ini menyediakan command untuk downloader media sosial, pembuatan sticker, konversi media, upload file, diagnostics, dan AI chat melalui API yang kompatibel dengan OpenAI.

## Requirements

Untuk menjalankan langsung di host:

- Go 1.26.11 atau lebih baru (toolchain 1.26.11 direkomendasikan oleh `go.mod`).
- PostgreSQL atau SQLite (pilih salah satu, lihat `DB_DRIVER`).
- Git.
- `webp` / `webpmux` untuk fitur sticker.
- `ffmpeg` dan ImageMagick direkomendasikan untuk fitur media tertentu.

Install `webp`:

```bash
# Ubuntu / Debian
sudo apt install -y webp

# Fedora
sudo dnf install -y libwebp-tools
```

## Konfigurasi

Salin file contoh environment ke `.env`:

```bash
cp .env.sample .env
```

Lalu sesuaikan nilainya.

Variabel penting:

| Variable | Keterangan | Default |
| --- | --- | --- |
| `APP_ENV` | Environment aplikasi, misalnya `dev` atau `production`. | `dev` |
| `LOGIN_METHOD` | Metode login WhatsApp: `qr` atau `pairing`. | `qr` |
| `PAIRING_PHONE_NUMBER` | Nomor telepon internasional (tanpa `+`) untuk `LOGIN_METHOD=pairing`. | kosong |
| `DB_DRIVER` | Driver database: `postgres` atau `sqlite`. | `postgres` |
| `DATABASE_URL` | Connection string database untuk WhatsApp session store. | wajib diisi |
| `OWNER` | Daftar owner JID, pisahkan dengan koma jika lebih dari satu. | kosong |
| `PREFIX` | Prefix command, contoh `.`. | `.` |
| `COOLDOWN` | Cooldown command, contoh `3s` atau `3`. Set `0` untuk nonaktif. | `3s` |
| `FREE_DAILY_LIMIT` | Jumlah command gratis per user per hari. `0` = unlimited. | `10` |
| `QUOTA_FAIL_OPEN` | Jika `true`, command tetap jalan saat database kuota error; jika `false`, command diblokir. | `true` |
| `ADMIN_TTL` | TTL cache admin grup. | `45s` |
| `DISABLE_CONTACT_IMPORT` | Nonaktifkan import kontak WhatsApp. | `true` |
| `BASEAPI_URL` | Base URL Hararest untuk downloader dan generator quote. | kosong |
| `BASES3_URL` | Base URL go-tempupload dengan endpoint `POST /upload/presign`. Bot meminta URL lalu PUT file langsung ke storage. | kosong |
| `REMOVEBG_URL` | Endpoint API untuk command `removebg`. | kosong |
| `TELEGRAM_BOT_TOKEN` | Token bot Telegram untuk `.telesticker`. | kosong |
| `OPENAI_BASE_URL` | Base URL API OpenAI-compatible. | `https://api.openai.com/v1` |
| `OPENAI_API_KEY` | API key untuk command AI. | kosong |
| `OPENAI_MODEL` | Model AI yang digunakan. | kosong |
| `OPENAI_PROVIDERS` | JSON array provider untuk rotasi/failover AI. | kosong |
| `OPENAI_PROVIDERS_FILE` | Path file JSON provider, alternatif `OPENAI_PROVIDERS`. | kosong |
| `OPENAI_TIMEOUT` | Timeout request AI. | `90s` |
| `OPENAI_SYSTEM_PROMPT` | System prompt untuk command AI. | prompt Bahasa Indonesia bawaan |

Konfigurasi opsional:

| Variable | Keterangan | Default |
| --- | --- | --- |
| `TAILSCALE_AUTHKEY` | Auth key untuk menjalankan Tailscale dari entrypoint container. | kosong |
| `TAILSCALE_SOCKS5` | Set `1` untuk merutekan HTTP client internal melalui SOCKS5 Tailscale di `127.0.0.1:1055`. | `0` |
| `RPG_ENABLED` | Aktifkan service RPG browser. | `false` |
| `RPG_PUBLIC_URL` | Origin publik Hararest tanpa `/rpg` atau path lain. | kosong |
| `RPG_LISTEN_ADDR` | Alamat listener RPG. `PORT` dari platform mengambil prioritas. | `127.0.0.1:8089` |
| `RPG_GATEWAY_SECRET` | Secret acak minimal 32 karakter yang sama dengan Hararest. | kosong |

Konfigurasi RPG production dan panduan gateway tersedia di [dokumentasi RPG](docs/rpg.md). Daftar lengkap nilai contoh tersedia di [`.env.sample`](.env.sample).

Contoh minimal:

```env
APP_ENV=dev
OWNER=0123456789@lid
PREFIX=.
COOLDOWN=3s
ADMIN_TTL=45s
DISABLE_CONTACT_IMPORT=true
FREE_DAILY_LIMIT=10
QUOTA_FAIL_OPEN=true

LOGIN_METHOD=qr
# Wajib hanya untuk LOGIN_METHOD=pairing; tanpa '+' atau angka 0 di depan.
PAIRING_PHONE_NUMBER=6281234567890

# PostgreSQL (default)
DB_DRIVER=postgres
DATABASE_URL=postgres://user:password@localhost:5432/kotonehara?sslmode=disable

# Atau SQLite (tanpa perlu server database terpisah)
# DB_DRIVER=sqlite
# DATABASE_URL=file:kotonehara.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)

BASEAPI_URL=
BASES3_URL=https://s3.example.com
REMOVEBG_URL=
TELEGRAM_BOT_TOKEN=

OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=
OPENAI_MODEL=
OPENAI_TIMEOUT=90s
OPENAI_SYSTEM_PROMPT=Kamu adalah Kotonehara, asisten WhatsApp yang membantu dengan jawaban jelas, ringkas, dan ramah dalam Bahasa Indonesia.

# Opsional: isi hanya jika fitur terkait digunakan.
TAILSCALE_AUTHKEY=
TAILSCALE_SOCKS5=0
RPG_ENABLED=false
RPG_PUBLIC_URL=
RPG_LISTEN_ADDR=127.0.0.1:8089
RPG_GATEWAY_SECRET=
```

### Rotasi provider AI

Untuk membagi request AI dan berpindah otomatis saat upstream timeout/jaringan, HTTP `429`, atau `5xx`, buat file provider (misalnya `configs/providers.json`) lalu arahkan environment berikut:

```env
OPENAI_PROVIDERS_FILE=configs/providers.json
```

Lihat `configs/providers.json.example` untuk formatnya. Provider aktif dirotasi round-robin; `model` dapat berupa satu string atau array model. Jika konfigurasi rotasi tidak diisi, bot tetap memakai `OPENAI_BASE_URL`, `OPENAI_API_KEY`, dan `OPENAI_MODEL` seperti sebelumnya.

> Jangan commit `.env` atau secret/API key ke repository.

## Menjalankan aplikasi

Download dependency terlebih dahulu:

```bash
go mod download
```

Jalankan langsung:

```bash
go run ./cmd/bot/main.go
```

Atau build binary:

```bash
go build -o hara ./cmd/bot/main.go
./hara
```

Saat pertama kali dijalankan, bot akan menampilkan QR code di terminal. Scan QR tersebut dari aplikasi WhatsApp.

Atau, kalau tidak ingin scan QR, gunakan metode pairing dengan kode:

```env
LOGIN_METHOD=pairing
PAIRING_PHONE_NUMBER=6281234567890
```

Bot akan menampilkan kode pairing di terminal. Masukkan kode tersebut di HP: WhatsApp > Perangkat Tertaut > Tautkan dengan nomor telepon.

## Docker / Podman

Build image:

```bash
podman build -t kotonehara:latest .
```

Run dengan env file:

```bash
podman run --rm -it --env-file .env --name kotonehara kotonehara:latest
```

Dengan Docker:

```bash
docker build -t kotonehara:latest .
docker run --rm -it --env-file .env --name kotonehara kotonehara:latest
```

Image menjalankan `tailscale.sh` sebagai command default. Pastikan konfigurasi yang dibutuhkan script tersebut tersedia jika deployment memakai Tailscale.

Untuk deployment branch `dev` langsung ke Docker di VPS, termasuk lokasi file
env dan koneksi ke Hararest, lihat [Dev deployment on the VPS](docs/dev-vps.md).

## Development

Jika memakai [`mise`](https://mise.jdx.dev/), task yang tersedia:

```bash
mise run dev      # live reload dengan air
mise run run      # go run ./cmd/bot/main.go
mise run build    # build ke bin/hara
mise run tidy     # go mod tidy dan go fmt ./...
```

Untuk live reload tanpa `mise`:

```bash
air -c .air.toml
```

Format dan rapikan dependency secara manual:

```bash
go fmt ./...
go mod tidy
```

Jalankan test:

```bash
go test ./...
```

## Struktur proyek

```text
cmd/bot/                 Entry point aplikasi
internal/clients/        Wrapper whatsmeow untuk kirim pesan/media dan operasi grup
internal/commands/       Registry command, cooldown, menu, dan executor
internal/devices/        Device WhatsApp dan event handler
internal/handlers/       Implementasi command bot
internal/infra/config/   Loader konfigurasi environment
internal/infra/db/       Koneksi database (PostgreSQL atau SQLite)
internal/media/          Sticker, meme, brat, dan efek gambar
internal/message/        Parser pesan WhatsApp
internal/service/        Client API eksternal, OpenAI, S3, dan HTTP helper
```

## Profil Pinterest dan Pixiv

Command berikut memakai `BASEAPI_URL` Hararest. Hararest yang dituju harus
menyediakan endpoint `/api/pinterest/user` dan `/api/pixiv/user`.

```text
.pinuser <username atau URL profil Pinterest>
.pinterestuser <username atau URL profil Pinterest>
.pixivuser <ID atau URL profil Pixiv> [--mode safe|r18|all]
.pxuser <ID atau URL profil Pixiv> [--mode safe|r18|all]
.pixivsearch <kata kunci> [--mode safe|r18|all]
```

Profil ditampilkan sebagai teks dengan maksimal lima tautan pin/artwork.
`--mode safe` memilih karya aman, `--mode r18` memilih R18, dan `--mode all`
memilih semua kategori. Tanpa opsi, mode tetap `all` seperti pencarian lama.
Mode juga dapat ditulis `--mode=safe`; kata biasa `safe`/`all` tetap menjadi
bagian kata kunci jika tidak memakai flag. Contoh: `.pixivsearch langit biru --mode safe`.
Prefix menyesuaikan konfigurasi `PREFIX`.

## Menambah command baru

Command biasanya dibuat di package `internal/handlers` dan didaftarkan lewat `commands.Register` di fungsi `init()`:

```go
package handlers

import (
    "context"

    "github.com/MapIHS/kotonehara/internal/clients"
    "github.com/MapIHS/kotonehara/internal/commands"
    "github.com/MapIHS/kotonehara/internal/infra/config"
    "github.com/MapIHS/kotonehara/internal/message"
)

func init() {
    commands.Register(&commands.Command{
        Name:        "ping",
        Tags:        "main",
        Description: "Cek apakah bot aktif",
        IsPrefix:    true,
        Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
            _, _ = m.Reply(ctx, "pong")
        },
    })
}
```

`cmd/bot/main.go` sudah mengimpor `internal/handlers` secara blank import, sehingga command baru di `internal/handlers` otomatis terdaftar saat aplikasi berjalan.

## License

Lihat `LICENSE`.