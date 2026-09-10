# Kotonehara

Kotonehara adalah bot WhatsApp berbasis Go yang dibangun dengan [`whatsmeow`](https://github.com/tulir/whatsmeow). Bot ini menyediakan command untuk downloader media sosial, pembuatan sticker, konversi media, upload file, diagnostics, dan AI chat melalui API yang kompatibel dengan OpenAI.

## Fitur

- Bot WhatsApp multi-command dengan prefix yang dapat dikonfigurasi.
- Login WhatsApp via QR code.
- Penyimpanan session/device menggunakan PostgreSQL atau SQLite (pilih via `DB_DRIVER`).
- Downloader untuk Instagram, TikTok, Facebook, X/Twitter, YouTube, Threads, dan Rednote/Xiaohongshu melalui API eksternal.
- Tools media: sticker, sticker meme, brat, image/video conversion, upload ke URL.
- Sticker quote dan fake quote melalui Quote API.
- Command AI via OpenAI-compatible API.
- WhatsApp outbound call via [`meowcaller`](https://github.com/purpshell/meowcaller) untuk owner.
- Docker/Podman support.

## Requirements

Untuk menjalankan langsung di host:

- Go 1.26.6 atau lebih baru.
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
| `LOGIN_METHOD` | Metode login WhatsApp: `qr` atau `pairing`. | `qr` |
| `PAIRING_PHONE_NUMBER` | Nomor telepon internasional (tanpa `+`) untuk `LOGIN_METHOD=pairing`. | kosong |
| `DB_DRIVER` | Driver database: `postgres` atau `sqlite`. | `postgres` |
| `DATABASE_URL` | Connection string database untuk WhatsApp session store. | wajib diisi |
| `OWNER` | Daftar owner JID, pisahkan dengan koma jika lebih dari satu. | kosong |
| `PREFIX` | Prefix command, contoh `.`. | `.` |
| `COOLDOWN` | Cooldown command, contoh `3s` atau `3`. Set `0` untuk nonaktif. | `3s` |
| `ADMIN_TTL` | TTL cache admin grup. | `45s` |
| `DISABLE_CONTACT_IMPORT` | Nonaktifkan import kontak WhatsApp. | `true` |
| `BASEAPI_URL` | Base URL API eksternal untuk downloader. | kosong |
| `BASES3_URL` | Base URL service upload file. | kosong |
| `REMOVEBG_URL` | Endpoint API untuk command `removebg`. | kosong |
| `QUOTE_API_URL` | Endpoint generator gambar untuk command `.qc` dan `.fqc`. | kosong |
| `OPENAI_BASE_URL` | Base URL API OpenAI-compatible. | `https://api.openai.com/v1` |
| `OPENAI_API_KEY` | API key untuk command AI. | kosong |
| `OPENAI_MODEL` | Model AI yang digunakan. | kosong |
| `OPENAI_PROVIDERS` | JSON array provider untuk rotasi/failover AI. | kosong |
| `OPENAI_PROVIDERS_FILE` | Path file JSON provider, alternatif `OPENAI_PROVIDERS`. | kosong |
| `OPENAI_TIMEOUT` | Timeout request AI. | `90s` |
| `OPENAI_SYSTEM_PROMPT` | System prompt untuk command AI. | prompt Bahasa Indonesia bawaan |

Contoh minimal:

```env
# PostgreSQL (default)
DB_DRIVER=postgres
DATABASE_URL=postgres://user:password@localhost:5432/kotonehara?sslmode=disable

# Atau SQLite (tanpa perlu server database terpisah)
# DB_DRIVER=sqlite
# DATABASE_URL=file:kotonehara.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)
OWNER=0123456789@lid
PREFIX=.
COOLDOWN=3s
ADMIN_TTL=45s
BASEAPI_URL=
BASES3_URL=https://s3.example.com
REMOVEBG_URL=
QUOTE_API_URL=

OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=
OPENAI_MODEL=
OPENAI_TIMEOUT=90s
OPENAI_SYSTEM_PROMPT=Kamu adalah Kotonehara, asisten WhatsApp yang membantu dengan jawaban jelas, ringkas, dan ramah dalam Bahasa Indonesia.
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

## Rich HTML (eksperimental)

Owner dapat me-reply pesan teks berisi HTML dengan `.richhtml` atau `.html`
(sesuaikan dengan `PREFIX` bot). Contoh teks yang dapat di-reply:

```html
<h1>Halo</h1><p>Pesan dari Kotonehara.</p>
```

Payload maksimal 64 KiB dan dikirim ke chat tempat command dijalankan melalui
AI rich response. Pesan tanpa reply teks ditolak. Tampilan HTML bergantung pada
dukungan WhatsApp penerima; konfirmasi pengiriman tidak menjamin HTML dirender.

`clients.BuildRichHTML` memakai primitive `GenAIaeacdsnwHtmlPrimitive`, mengikuti
[builder HTML Casileys](https://github.com/kyleee-max/casileys/blob/main/lib/MessageBuilder/AIRich.js).
`unifiedResponse.data` berisi JSON UTF-8 mentah sesuai
[schema whatsmeow](https://github.com/tulir/whatsmeow/blob/9ec8f76db5f1/proto/waAICommon/WAWebProtobufsAICommon.proto);
base64 hanya digunakan pada representasi JSON protobuf. ID respons pada metadata
bot dan payload disamakan. HTML dengan UTF-8 tidak valid ditolak agar isinya tidak
berubah diam-diam saat dienkode.

`trusted_sources` adalah atribusi sumber, bukan izin jaringan. Dokumentasi
[Elaina Baileys](https://www.npmjs.com/package/@rexxhayanasi/elaina-baileys)
melaporkan pembatasan jaringan pada WebView HTML Android. Keberhasilan tes browser
lokal belum membuktikan bahwa WebSocket dapat diakses dari kartu WhatsApp.

### Player musik WebSocket

Gunakan `.playws <judul lagu>` atau `.playws <link YouTube>` (alias `.playhtml`).
Bot menyiapkan audio melalui Hararest, mengirim kartu dengan tombol putar/jeda
dan posisi lagu langsung di WhatsApp.

Player memakai base URL API yang sama dari `BASEAPI_URL`; misalnya
`https://api.example.com` menjadi `wss://api.example.com/ws/player/...`.
Hararest harus sudah menjalankan modul `/api/player/sessions` dan `/ws/player/`.
Jika `BASEAPI_URL` adalah alamat internal/Tailscale yang tidak bisa diakses
perangkat penerima, isi `PLAYER_PUBLIC_URL` di Hararest dengan origin API publik.
Reverse proxy API harus meneruskan WebSocket Upgrade.

Sesi berlaku 30 menit, dengan audio maksimal 24 MiB dan durasi 10 menit. Audio
diputar setelah transfer WS selesai. Bila sesi berakhir atau backend direstart,
jalankan `.playws` lagi. Dukungan rich HTML berbeda antar-client WhatsApp;
renderer penerima perlu mendukung JavaScript, koneksi WS, dan audio.

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

Smoke test integrasi Hararest dapat dijalankan secara opt-in:

```bash
HARAREST_LIVE_URL=http://HOST:PORT go test ./internal/service/api -run '^TestHararestLive$' -v -count=1
```

Tes ini memanggil API YouTube, Pinterest, Pixiv, Waifu SFW, dan OCR, termasuk
mengunduh klip YouTube pendek. Tes tidak membuka sesi WhatsApp atau mengirim pesan.
Tanpa `HARAREST_LIVE_URL`, tes live dilewati. Jalankan satu batch pada satu waktu
agar tidak melewati limiter bersama Hararest.

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
