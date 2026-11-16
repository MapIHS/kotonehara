#!/bin/sh
set -e

if [ -n "${TAILSCALE_AUTHKEY:-}" ]; then
  mkdir -p /tmp/tailscale
  chmod 700 /tmp/tailscale
  /app/tailscaled \
    --tun=userspace-networking \
    --state=/tmp/tailscale.state \
    --statedir=/tmp/tailscale \
    --socket=/tmp/tailscaled.sock \
    --socks5-server=localhost:1055 &

  /app/tailscale --socket=/tmp/tailscaled.sock up \
    --authkey="${TAILSCALE_AUTHKEY}" \
    --hostname="heroku-hara" \
    --accept-dns=false

  echo "Tailscale started"
fi

exec /app/hara
