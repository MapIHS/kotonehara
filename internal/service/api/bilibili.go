package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type BilibiliMedia struct {
	Type       string   `json:"type"`
	Quality    int      `json:"quality"`
	Label      string   `json:"label"`
	URL        string   `json:"url"`
	BackupURLs []string `json:"backupUrls"`
	MimeType   string   `json:"mimeType"`
	Codecs     string   `json:"codecs"`
	Bandwidth  int64    `json:"bandwidth"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Order      int      `json:"order"`
}

type BilibiliResult struct {
	ID       string  `json:"id"`
	URL      string  `json:"url"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
	Page     int     `json:"page"`
	Author   struct {
		Name string `json:"name"`
	} `json:"author"`
	Format             string            `json:"format"`
	RequestedQuality   int               `json:"requestedQuality"`
	AvailableQualities []int             `json:"availableQualities"`
	Media              []BilibiliMedia   `json:"media"`
	Headers            map[string]string `json:"headers"`
}

// Bilibili returns metadata and media URLs; DASH video still needs its audio track.
func (c *Client) Bilibili(ctx context.Context, targetURL, quality string) (*BilibiliResult, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/bilibili"
	q := u.Query()
	q.Set("url", targetURL)
	if quality != "" {
		q.Set("quality", quality)
	}
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
		return nil, readAPIError("bilibili", resp)
	}
	var out APIResponse[BilibiliResult]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	if out.Status != "success" || len(out.Data.Media) == 0 {
		return nil, fmt.Errorf("bilibili api tidak mengembalikan media")
	}
	return &out.Data, nil
}
