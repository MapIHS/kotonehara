package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	Uploaded    float64     `json:"uploaded"`
	Channel     channelInfo `json:"channel"`
	Videos      []string    `json:"videos"`
}

func (c *Client) YoutubeInfo(ctx context.Context, targetURL string) (*videoInfo, error) {
	u, err := neturl.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/v1/youtube/video"

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
		return nil, fmt.Errorf("Youtube api http %d", resp.StatusCode)
	}

	var out APIResponse[videoInfo]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *Client) YoutubeDownload(ctx context.Context, targetURL string, quality string, isVideo bool) ([]byte, error) {
	u, err := neturl.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/v1/youtube/video/download"

	if !isVideo {
		u.Path = "/api/v1/youtube/audio/download"
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
	if c.apikey != "" {
		req.Header.Set("X-API-Key", c.apikey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("youtube api http %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}
