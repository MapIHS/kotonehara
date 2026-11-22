package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
)

var ErrInstagramUnavailable = errors.New("media Instagram tidak tersedia untuk akses publik")
var ErrInstagramBusy = errors.New("extractor Instagram sedang sibuk")

type instagramResult struct {
	Thumbnail string      `json:"thumbnail,omitempty"`
	Photos    []mediaFile `json:"photos,omitempty"`
	Videos    []mediaFile `json:"videos,omitempty"`
}

type mediaFile struct {
	URL   string `json:"url"`
	Size  int64  `json:"size"`
	FSize string `json:"fSize"`
}

func (c *Client) Instagram(ctx context.Context, targetURL string) (*instagramResult, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/instagram"

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
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %v", ErrInstagramUnavailable, readAPIError("instagram", resp))
		}
		if resp.StatusCode == http.StatusServiceUnavailable {
			return nil, fmt.Errorf("%w: %v", ErrInstagramBusy, readAPIError("instagram", resp))
		}
		return nil, readAPIError("instagram", resp)
	}

	var out APIResponse[instagramResult]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
