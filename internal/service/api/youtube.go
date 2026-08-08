package api

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
)

type channelInfo struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	Name        string `json:"name"`
	Subscribers int    `json:"subscribers"`
	Verified    bool   `json:"verified"`
}

type videoInfo struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Thumbnail   string      `json:"thumbnail"`
	Description string      `json:"description"`
	Duration    float64     `json:"duration"`
	Views       int         `json:"views"`
	Likes       int         `json:"likes"`
	Comments    int         `json:"comments"`
	Channel     channelInfo `json:"channel"`
	Videos      []string    `json:"videos"`
}

func (c *Client) YoutubeInfo(ctx context.Context, targetURL string) (*videoInfo, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/youtube/info"

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
		return nil, fmt.Errorf("Youtube api http %d", resp.StatusCode)
	}

	var out APIResponse[videoInfo]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *Client) YoutubeDownload(ctx context.Context, targetURL string, quality string, isVideo bool) ([]byte, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/youtube/video"

	if !isVideo {
		u.Path = "/api/youtube/audio"
	}

	q := u.Query()
	q.Set("url", targetURL)
	q.Set("quality", quality)
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
		return nil, fmt.Errorf("youtube api http %d", resp.StatusCode)
	}

	data, err := readResponseBody(resp, maxMediaSize)
	if err != nil {
		return nil, err
	}
	return data, nil
}

type searchChannel struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type YoutubeSearchResult struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Thumbnail string        `json:"thumbnail"`
	Duration  float64       `json:"duration"`
	Views     int           `json:"views"`
	Channel   searchChannel `json:"channel"`
	URL       string        `json:"url"`
}

func (c *Client) YoutubeSearch(ctx context.Context, query string, limit int) ([]YoutubeSearchResult, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/youtube/search"

	q := u.Query()
	q.Set("q", query)
	q.Set("limit", fmt.Sprintf("%d", limit))
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
		return nil, fmt.Errorf("youtube search api http %d", resp.StatusCode)
	}

	var out APIResponse[[]YoutubeSearchResult]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}
