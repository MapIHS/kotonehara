package s3

import (
	"github.com/MapIHS/kotonehara/internal/service/httpclient"
	"time"
)

type Client struct {
	*httpclient.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		Client: httpclient.New(baseURL, timeout),
	}
}
