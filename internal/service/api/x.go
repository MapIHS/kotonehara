package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
)

type XResult struct {
	Status        string             `json:"status,omitempty"`
	InputURL      string             `json:"input_url,omitempty"`
	DownloaderURL string             `json:"downloader_url,omitempty"`
	MediaLinks    []TwitterMediaLink `json:"media_links,omitempty"`
}

type TwitterMediaLink struct {
	Label   string `json:"label,omitempty"`
	Quality string `json:"quality,omitempty"`
	URL     string `json:"url,omitempty"`
}

func (c *Client) X(ctx context.Context, targetURL string) (*XResult, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/twitter/download"

	q := u.Query()
	q.Set("url", targetURL)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("X api http %d", resp.StatusCode)
	}

	var out APIResponse[XResult]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
