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
		HTTP:   newHTTPClient(20 * time.Second),
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 100
	tr.MaxIdleConnsPerHost = 20
	tr.IdleConnTimeout = 90 * time.Second
	tr.ResponseHeaderTimeout = 15 * time.Second
	tr.ExpectContinueTimeout = time.Second

	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}
}
