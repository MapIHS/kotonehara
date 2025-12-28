## How to Clone

```bash
git clone https://github.com/MapIHS/kotonehara
cd kotonehara
```

## Config

change .env.sample to .env

```bash
cp .env.sample .env
```

```bash
# database url postgres
DATABASE_URL=YOUR_DATABASE_URL

# config
OWNER=*@lid
PREFIX=.
COOLDOWN=3s
ADMIN_TTL=45s
```


## Install Webpmux for use sticker

ubuntu / debian
```bash
apt install -y webpmux
```
fedora
```bash
dnf install -y libwebp-tools
```

## How to run

### linux
```bash
go mod download
go run cmd/bot/main.go
```
or with build
```bash
go mod download
go build -o hara cmd/bot/main.go

./hara
```

#### docker or podman

```bash
podman build -t kotonehara:latest .
podman run --rm -it --env-file .env --name kotonehara kotonehara:latest
```
