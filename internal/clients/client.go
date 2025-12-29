package clients

import (
	"net/http"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"go.mau.fi/whatsmeow"
)

type Client struct {
	WA *whatsmeow.Client

	admins *adminCache
	cfg    config.Config
	HTTP   *http.Client
}

func New(c *whatsmeow.Client, cfg config.Config) *Client {
	return &Client{
		WA:     c,
		cfg:    cfg,
		admins: newAdminCache(cfg.AdminTTL),
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}
