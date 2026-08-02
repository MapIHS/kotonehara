package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type BraveSearchResult struct {
	Title     string `json:"title"`
	Link      string `json:"link"`
	Snippet   string `json:"snippet"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

type BraveSearchData struct {
	Results      []BraveSearchResult `json:"results"`
	TotalResults string              `json:"totalResults"`
	SearchTime   float64             `json:"searchTime"`
}

func (c *Client) SearchBrave(ctx context.Context, query string) (*BraveSearchData, error) {
	reqURL := fmt.Sprintf("%s/api/brave/search?q=%s", c.BaseURL, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create brave search request: %w", err)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute brave search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, apiHTTPStatusError("brave", resp.StatusCode, body)
	}

	var payload APIResponse[BraveSearchData]

	if err := decodeAPIResponse(resp, &payload); err != nil {
		return nil, fmt.Errorf("decode brave search response: %w", err)
	}

	return &payload.Data, nil
}
