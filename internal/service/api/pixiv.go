package api

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
)

type PixivDownload struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	URLs        []string `json:"urls"`
}

type PixivSearchItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Author string `json:"author"`
}

type PixivSearchResponse struct {
	Results []PixivSearchItem `json:"results"`
}

func (c *Client) Pixiv(ctx context.Context, idOrURL string) (*PixivDownload, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/pixiv"

	q := u.Query()
	q.Set("id", idOrURL)
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
		return nil, fmt.Errorf("pixiv api http %d", resp.StatusCode)
	}

	var out APIResponse[PixivDownload]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *Client) PixivSearch(ctx context.Context, query string) (*PixivSearchResponse, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/pixiv/search"

	q := u.Query()
	q.Set("q", query)
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
		return nil, fmt.Errorf("pixiv search api http %d", resp.StatusCode)
	}

	var out APIResponse[PixivSearchResponse]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
