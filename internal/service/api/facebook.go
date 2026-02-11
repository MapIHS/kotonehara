package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
)

type facebookResult struct {
	Thumbnail string    `json:"thumbnail"`
	Videos    []fbVideo `json:"videos"`
}

type fbVideo struct {
	Quality string `json:"quality"`
	URL     string `json:"url"`
	Size    int64  `json:"size"`
	FSize   string `json:"fSize"`
}

func (c *Client) Facebook(ctx context.Context, targetURL string) (*facebookResult, error) {
	u, err := neturl.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/facebook"

	q := u.Query()
	q.Set("url", targetURL)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("facebook api http %d", resp.StatusCode)
	}

	var out APIResponse[facebookResult]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
