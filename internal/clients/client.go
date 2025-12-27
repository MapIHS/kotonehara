package clients

import "go.mau.fi/whatsmeow"

type Client struct {
	WA *whatsmeow.Client
}

func New(c *whatsmeow.Client) *Client {
	return &Client{
		WA: c,
	}
}
