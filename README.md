## Getting started
### Clone the repository

```bash
git clone https://github.com/MapIHS/kotonehara
cd kotonehara
```

Make sure you already installed Git


### Configuration

Copy or rename .env.sample to .env

```bash
cp .env.sample .env
# Or
mv .env.sample .env
```

```env
# Postgress database url
DATABASE_URL=YOUR_DATABASE_URL

# Basic configuration
OWNER=0123456789@lid # To use the owner features.
PREFIX=. .           # Prefix for command.
COOLDOWN=3s          # Cooldown time for the command, set to 0 to disable.
ADMIN_TTL=45s
BASEAPI_URL=
BASES3_URL=https://s3.ihsn.dev
MEMEHOST_URL=https://apimem.ihsn.dev # or api.memegen.link

# OpenAI-compatible AI command
OPENAI_BASE_URL=https://bedrock-mantle.<your-region>.api.aws/v1
OPENAI_API_KEY=YOUR_BEDROCK_API_KEY
OPENAI_MODEL=openai.gpt-oss-120b
OPENAI_TIMEOUT=90s
OPENAI_SYSTEM_PROMPT=Kamu adalah Kotonehara, asisten WhatsApp yang membantu dengan jawaban jelas, ringkas, dan ramah dalam Bahasa Indonesia.
```


### Install Webpmux to be able to create stickers.

Ubuntu / Debian
```bash
apt install -y webp
```
Fedora
```bash
dnf install -y libwebp-tools
```

## How to run
### Linux

Run it directly,
```bash
go mod download
go run cmd/bot/main.go
```

Or build it first.
```bash
go mod download
go build -o hara cmd/bot/main.go

./hara
```

### Or via Docker or Podman

```bash
podman build -t kotonehara:latest .
podman run --rm -it --env-file .env --name kotonehara kotonehara:latest
```
