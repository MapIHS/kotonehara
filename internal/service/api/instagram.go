package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
)

type instagramResult struct {
	Thumbnail string      `json:"thumbnail,omitempty"`
	Photos    []mediaFile `json:"photos,omitempty"`
	Videos    []mediaFile `json:"videos,omitempty"`
}

type mediaFile struct {
	URL   string `json:"url"`
	Size  int64  `json:"size"`
	FSize string `json:"fSize"`
}

func (c *Client) Instagram(ctx context.Context, targetURL string) (*instagramResult, error) {
	u, err := neturl.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/v1/instagram/info"

	q := u.Query()
	q.Set("url", targetURL)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if c.apikey != "" {
		req.Header.Set("X-API-Key", c.apikey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("instagram api http %d", resp.StatusCode)
	}

	var out APIResponse[instagramResult]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
