package clients

import (
	"net/http"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/service/httpclient"
	meowcaller "github.com/purpshell/meowcaller"
	"go.mau.fi/whatsmeow"
)

type Client struct {
	WA    *whatsmeow.Client
	Calls *meowcaller.Client

	admins *adminCache
	cfg    config.Config
	HTTP   *http.Client
}

func New(c *whatsmeow.Client, cfg config.Config, callClient *meowcaller.Client) *Client {
	return &Client{
		WA:     c,
		Calls:  callClient,
		cfg:    cfg,
		admins: newAdminCache(cfg.AdminTTL),
		HTTP:   httpclient.New("", 20*time.Second).HTTP,
	}
}
