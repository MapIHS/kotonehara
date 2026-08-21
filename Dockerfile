FROM golang:1.26.6-bookworm AS builder
WORKDIR /app

RUN apt-get update && apt-get install -y \
    libwebp-dev \
    webp \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o hara cmd/bot/main.go

# build final
FROM debian:bookworm-slim
WORKDIR /app

RUN apt-get update && apt-get install -y ca-certificates \
    neofetch \
    webp \
    ffmpeg \
    imagemagick \
    fonts-noto-color-emoji \
    fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/hara /app/hara

COPY --from=builder /app/configs /app/configs

COPY --from=docker.io/tailscale/tailscale:stable /usr/local/bin/tailscaled /app/tailscaled
COPY --from=docker.io/tailscale/tailscale:stable /usr/local/bin/tailscale /app/tailscale

RUN mkdir -p /var/run/tailscale /var/cache/tailscale /var/lib/tailscale /app/data
VOLUME ["/app/data"]
COPY tailscale.sh /app/tailscale.sh
RUN chmod +x /app/tailscale.sh

CMD ["/app/tailscale.sh"]
