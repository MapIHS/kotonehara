package api

import (
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/service/httpclient"
)

type Client struct {
	*httpclient.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		Client: httpclient.New(baseURL, timeout),
	}
}

var (
	sharedMu      sync.Mutex
	sharedClients = map[string]*Client{}
)

func Shared(baseURL string, timeout time.Duration) *Client {
	key := baseURL + "|" + timeout.String()
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if c, ok := sharedClients[key]; ok {
		return c
	}
	c := New(baseURL, timeout)
	sharedClients[key] = c
	return c
}
