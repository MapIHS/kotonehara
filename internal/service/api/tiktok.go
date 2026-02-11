package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
)

type tiktokResult struct {
	ID       string   `json:"id"`
	Region   string   `json:"region"`
	Title    string   `json:"title"`
	Cover    string   `json:"cover"`
	Duration int      `json:"duration"`
	Size     int64    `json:"size"`
	Video    *string  `json:"video"`
	Images   []string `json:"images"`
	Music    string   `json:"music"`

	MusicInfo music  `json:"musicInfo"`
	Played    int64  `json:"played"`
	Comments  int64  `json:"comments"`
	Share     int64  `json:"share"`
	Download  int64  `json:"download"`
	Uploaded  int64  `json:"uploaded"`
	Author    author `json:"author"`
}

type music struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Cover    string `json:"cover"`
	Author   string `json:"author"`
	Duration int    `json:"duration"`
}

type author struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func (c *Client) Tiktok(ctx context.Context, targetURL string) (*tiktokResult, error) {
	u, err := neturl.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/tiktok/download"

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
		return nil, fmt.Errorf("tiktok api http %d", resp.StatusCode)
	}

	var out APIResponse[tiktokResult]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
