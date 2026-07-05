package httpclient

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
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	httpClient := newHTTPClient(timeout)

	if os.Getenv("TAILSCALE_SOCKS5") == "1" {
		if c, err := newSOCKS5HTTPClient(timeout, "127.0.0.1:1055"); err == nil {
			httpClient = c
		}
	}

	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    httpClient,
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
		DialContext:         dialContext,
		ForceAttemptHTTP2:   false,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}
	if timeout > 0 {
		tr.ResponseHeaderTimeout = 30 * time.Second
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}, nil
}

func newHTTPClient(timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 100
	tr.MaxIdleConnsPerHost = 20
	tr.IdleConnTimeout = 90 * time.Second
	if timeout > 0 {
		tr.ResponseHeaderTimeout = 15 * time.Second
	}
	tr.ExpectContinueTimeout = time.Second

	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}
}
