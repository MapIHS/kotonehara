package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
)

type TwitterMediaType string

const (
	TwitterMediaTypeVideo TwitterMediaType = "video"
	TwitterMediaTypeAudio TwitterMediaType = "audio"
	TwitterMediaTypeImage TwitterMediaType = "image"
)

type XResult struct {
	Status        string             `json:"status,omitempty"`
	Message       string             `json:"message,omitempty"`
	InputURL      string             `json:"input_url,omitempty"`
	DownloaderURL string             `json:"downloader_url,omitempty"`
	MediaLinks    []TwitterMediaLink `json:"media_links,omitempty"`
}

type TwitterMediaLink struct {
	Label     string           `json:"label,omitempty"`
	Quality   string           `json:"quality,omitempty"`
	MediaType TwitterMediaType `json:"media_type"`
	URL       string           `json:"url,omitempty"`
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out APIResponse[XResult]
	if err := json.Unmarshal(body, &out); err == nil {
		if len(out.Data.MediaLinks) > 0 {
			return &out.Data, nil
		}

		status := strings.ToLower(strings.TrimSpace(out.Status))
		if status != "" && status != "success" && status != "ok" {
			if out.Message != "" {
				return nil, errors.New(out.Message)
			}
			return nil, fmt.Errorf("X api status: %s", out.Status)
		}
	}

	var direct XResult
	if err := json.Unmarshal(body, &direct); err != nil {
		return nil, err
	}

	status := strings.ToLower(strings.TrimSpace(direct.Status))
	if status != "" && status != "success" && status != "ok" {
		if direct.Message != "" {
			return nil, errors.New(direct.Message)
		}
		return nil, fmt.Errorf("X api status: %s", direct.Status)
	}

	if len(direct.MediaLinks) == 0 {
		return nil, fmt.Errorf("X api tidak mengembalikan media")
	}

	return &direct, nil
}
