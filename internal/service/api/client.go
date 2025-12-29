package api

import (
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apikey  string
	http    *http.Client
}

func New(baseURL, apikey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apikey:  apikey,
		http:    &http.Client{Timeout: timeout},
	}
}
