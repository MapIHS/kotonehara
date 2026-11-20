package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
)

type AvscanFrame struct {
	ImageName  string  `json:"image_name"`
	Similarity float64 `json:"similarity"`
	Timestamp  string  `json:"timestamp"`
	ThumbURL   string  `json:"thumb_url"`
}

type AvscanWatchUrls struct {
	Missav       string `json:"missav"`
	Javdb        string `json:"javdb"`
	Javbus       string `json:"javbus"`
	AvscanDirect string `json:"avscan_direct"`
}

type AvscanItem struct {
	VideoCode      string          `json:"video_code"`
	BestSimilarity float64         `json:"best_similarity"`
	BestTimestamp  string          `json:"best_timestamp"`
	BestThumb      string          `json:"best_thumb"`
	Frames         []AvscanFrame   `json:"frames"`
	WatchURLs      AvscanWatchUrls `json:"watch_urls"`
}

// AVScan queries Hararest's AVScan endpoint with either raw image bytes or an image URL.
func (c *Client) AVScan(ctx context.Context, imageBytes []byte, imageURL string) ([]AvscanItem, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("avscan: parse base url: %w", err)
	}
	u.Path = "/api/avscan"

	var req *http.Request
	if len(imageBytes) > 0 {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(imageBytes))
		if err != nil {
			return nil, fmt.Errorf("avscan: create request: %w", err)
		}

		contentType := http.DetectContentType(imageBytes)
		switch contentType {
		case "image/jpeg", "image/png", "image/webp":
		default:
			contentType = "image/jpeg"
		}
		req.Header.Set("Content-Type", contentType)
	} else if imageURL != "" {
		q := u.Query()
		q.Set("url", imageURL)
		u.RawQuery = q.Encode()

		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("avscan: create request: %w", err)
		}
	} else {
		return nil, fmt.Errorf("avscan: image bytes or image url required")
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("avscan: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, readAPIError("avscan", resp)
	}

	var out APIResponse[[]AvscanItem]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, fmt.Errorf("avscan: decode response: %w", err)
	}

	return out.Data, nil
}
