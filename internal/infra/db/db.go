package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"golang.org/x/net/proxy"
)

type Config struct {
	Driver string
	URL    string

	MaxOpenConns int
	MaxIdleConns int
	ConnMaxIdle  time.Duration
	ConnMaxLife  time.Duration
}

type pqDialer struct {
	proxy proxy.Dialer
}

func (d *pqDialer) Dial(network, address string) (net.Conn, error) {
	return d.proxy.Dial(network, address)
}

func (d *pqDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return d.proxy.Dial(network, address)
}

func Connect(ctx context.Context, cfg Config) (*sqlx.DB, error) {
	if cfg.Driver == "" {
		cfg.Driver = "postgres"
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("DATABASE_URL kosong")
	}

	var db *sqlx.DB
	var err error

	// Cek apakah mode Tailscale aktif
	if os.Getenv("TAILSCALE_SOCKS5") == "1" && cfg.Driver == "postgres" {
		dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:1055", nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer: %w", err)
		}
		connector, err := pq.NewConnector(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("pq connector: %w", err)
		}

		connector.Dialer(&pqDialer{proxy: dialer})

		db = sqlx.NewDb(sql.OpenDB(connector), "postgres")
	} else {
		db, err = sqlx.ConnectContext(ctx, cfg.Driver, cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("connect db: %w", err)
		}
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxIdle > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdle)
	}
	if cfg.ConnMaxLife > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLife)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}
