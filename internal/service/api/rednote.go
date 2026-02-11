package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
)

type xhsResult struct {
	ID          string `json:"id"`
	Uploaded    int64  `json:"uploaded"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`

	Author struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	} `json:"author"`

	Tags []xhsTag `json:"tags"`

	Liked       string `json:"liked"`
	Saved       string `json:"saved"`
	Share       string `json:"share"`
	Comments    int64  `json:"comments"`
	Recommended string `json:"recommended"`

	Cover  *string    `json:"cover"`
	Images []xhsImage `json:"images"`
	Video  *xhsVideo  `json:"video"`
}

type xhsTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type xhsImage struct {
	URL       string `json:"url"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	LivePhoto bool   `json:"livePhoto"`
}

type xhsVideo struct {
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bitrate  int    `json:"bitrate"`
	Duration int    `json:"duration"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
}

func (c *Client) Rednote(ctx context.Context, targetURL string) (*xhsResult, error) {
	u, err := neturl.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/xiaohongshu"

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
		return nil, fmt.Errorf("xhs api http %d", resp.StatusCode)
	}

	var out APIResponse[xhsResult]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
