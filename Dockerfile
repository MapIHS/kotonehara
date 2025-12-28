FROM golang:1.25-bookworm

WORKDIR /app

RUN apt-get update && apt-get install -y \
    libwebp-dev \
    webp \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o hara cmd/bot/main.go

CMD ["./hara"]
