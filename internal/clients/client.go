package clients

import (
	"time"

	"go.mau.fi/whatsmeow"
)

type Client struct {
	WA *whatsmeow.Client

	admins *adminCache
}

func New(c *whatsmeow.Client) *Client {
	return &Client{
		WA:     c,
		admins: newAdminCache(45 * time.Second),
	}
}
