package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
)

type ThreadsMediaType string

const (
	ThreadsMediaTypeVideo ThreadsMediaType = "video"
	ThreadsMediaTypeGIF   ThreadsMediaType = "gif"
	ThreadsMediaTypeImage ThreadsMediaType = "image"
	ThreadsMediaTypeMedia ThreadsMediaType = "media"
)

type ThreadsDownloadResult struct {
	SourceURL     string             `json:"source_url"`
	ThreadsterURL string             `json:"threadster_url"`
	ScrapedAt     string             `json:"scraped_at"`
	Count         int                `json:"count"`
	Items         []ThreadsMediaItem `json:"items"`
}

type ThreadsMediaItem struct {
	Index             int              `json:"index,omitempty"`
	Label             string           `json:"label,omitempty"`
	MediaType         ThreadsMediaType `json:"media_type,omitempty"`
	Username          string           `json:"username,omitempty"`
	Caption           string           `json:"caption,omitempty"`
	ThumbnailURL      string           `json:"thumbnail_url,omitempty"`
	ProfilePictureURL string           `json:"profile_picture_url,omitempty"`
	DownloadURL       string           `json:"download_url,omitempty"`
	OriginalMediaURL  string           `json:"original_media_url,omitempty"`
}

func (i ThreadsMediaItem) BestURL() string {
	if strings.TrimSpace(i.OriginalMediaURL) != "" {
		return i.OriginalMediaURL
	}
	return i.DownloadURL
}

func (c *Client) Threads(ctx context.Context, targetURL string) (*ThreadsDownloadResult, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/threads/download"

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

	body, err := readResponseBody(resp, maxAPIResponseSize)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiHTTPStatusError("threads", resp.StatusCode, body)
	}

	var out APIResponse[ThreadsDownloadResult]
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}

	if len(out.Data.Items) == 0 {
		return nil, fmt.Errorf("threads api tidak mengembalikan media")
	}

	return &out.Data, nil
}
