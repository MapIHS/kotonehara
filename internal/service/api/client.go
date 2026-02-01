package api

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/proxy"
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

	httpClient := &http.Client{Timeout: timeout}

	if os.Getenv("TAILSCALE_SOCKS5") == "1" {
		if c, err := newSOCKS5HTTPClient(timeout, "127.0.0.1:1055"); err == nil {
			httpClient = c
		}
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apikey:  apikey,
		http:    httpClient,
	}
}

func newSOCKS5HTTPClient(timeout time.Duration, socksAddr string) (*http.Client, error) {
	d, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return d.Dial(network, addr)
	}

	tr := &http.Transport{
		DialContext:           dialContext,
		ForceAttemptHTTP2:     false,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}, nil
}
