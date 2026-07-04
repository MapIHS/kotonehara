# Kotonehara

Kotonehara adalah bot WhatsApp berbasis Go yang dibangun dengan [`whatsmeow`](https://github.com/tulir/whatsmeow). Bot ini menyediakan command untuk downloader media sosial, pembuatan sticker, konversi media, upload file, diagnostics, dan AI chat melalui API yang kompatibel dengan OpenAI.

## Fitur

- Bot WhatsApp multi-command dengan prefix yang dapat dikonfigurasi.
- Login WhatsApp via QR code.
- Penyimpanan session/device menggunakan PostgreSQL.
- Downloader untuk Instagram, TikTok, Facebook, X/Twitter, YouTube, Threads, dan Rednote/Xiaohongshu melalui API eksternal.
- Tools media: sticker, sticker meme, brat, image/video conversion, upload ke URL.
- Command AI via OpenAI-compatible API.
- Docker/Podman support.

## Requirements

Untuk menjalankan langsung di host:

- Go sesuai versi di `go.mod`.
- PostgreSQL.
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
| `DATABASE_URL` | URL PostgreSQL untuk WhatsApp session store. | wajib diisi |
| `OWNER` | Daftar owner JID, pisahkan dengan koma jika lebih dari satu. | kosong |
| `PREFIX` | Prefix command, contoh `.`. | `.` |
| `COOLDOWN` | Cooldown command, contoh `3s` atau `3`. Set `0` untuk nonaktif. | `3s` |
| `ADMIN_TTL` | TTL cache admin grup. | `45s` |
| `DISABLE_CONTACT_IMPORT` | Nonaktifkan import kontak WhatsApp. | `true` |
| `BASEAPI_URL` | Base URL API eksternal untuk downloader. | kosong |
| `BASES3_URL` | Base URL service upload file. | kosong |
| `OPENAI_BASE_URL` | Base URL API OpenAI-compatible. | `https://api.openai.com/v1` |
| `OPENAI_API_KEY` | API key untuk command AI. | kosong |
| `OPENAI_MODEL` | Model AI yang digunakan. | kosong |
| `OPENAI_TIMEOUT` | Timeout request AI. | `90s` |
| `OPENAI_SYSTEM_PROMPT` | System prompt untuk command AI. | prompt Bahasa Indonesia bawaan |

Contoh minimal:

```env
DATABASE_URL=postgres://user:password@localhost:5432/kotonehara?sslmode=disable
OWNER=0123456789@lid
PREFIX=.
COOLDOWN=3s
ADMIN_TTL=45s
BASEAPI_URL=
BASES3_URL=https://s3.example.com

OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=
OPENAI_MODEL=
OPENAI_TIMEOUT=90s
OPENAI_SYSTEM_PROMPT=Kamu adalah Kotonehara, asisten WhatsApp yang membantu dengan jawaban jelas, ringkas, dan ramah dalam Bahasa Indonesia.
```

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

## Development

Jika memakai [`mise`](https://mise.jdx.dev/), task yang tersedia:

```bash
mise run dev      # go run ./cmd/bot/main.go
mise run build    # build ke bin/hara
mise run tidy     # go mod tidy dan go fmt ./...
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
internal/infra/config/   Loader konfigurasi environment
internal/infra/db/       Koneksi PostgreSQL
internal/media/          Sticker, meme, brat, dan efek gambar
internal/message/        Parser pesan WhatsApp
internal/service/        Client API eksternal, OpenAI, S3, dan HTTP helper
pkg/                     Implementasi command bot
```

## Menambah command baru

Command biasanya dibuat di package `pkg` dan didaftarkan lewat `commands.Register` di fungsi `init()`:

```go
package pkg

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

`cmd/bot/main.go` sudah mengimpor `pkg` secara blank import, sehingga command baru di `pkg` otomatis terdaftar saat aplikasi berjalan.

## License

Lihat `LICENSE`.
