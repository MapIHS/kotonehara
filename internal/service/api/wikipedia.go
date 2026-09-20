package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type WikipediaSection struct {
	Title   string  `json:"title"`
	Level   int     `json:"level"`
	ID      *string `json:"id"`
	Content string  `json:"content"`
}

type WikipediaArticle struct {
	SourceURL string             `json:"source_url"`
	Title     string             `json:"title"`
	Language  string             `json:"language"`
	Summary   string             `json:"summary"`
	Content   string             `json:"content"`
	Sections  []WikipediaSection `json:"sections"`
	Images    []struct {
		URL string `json:"url"`
		Alt string `json:"alt"`
	} `json:"images"`
	Categories []string `json:"categories"`
}

func (c *Client) Wikipedia(ctx context.Context, target string) (*WikipediaArticle, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/wikipedia/scrape"
	q := u.Query()
	q.Set("url", target)
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
		return nil, readAPIError("wikipedia", resp)
	}
	var out APIResponse[WikipediaArticle]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	if out.Status != "success" || strings.TrimSpace(out.Data.Title) == "" || (strings.TrimSpace(out.Data.Summary) == "" && strings.TrimSpace(out.Data.Content) == "") {
		return nil, fmt.Errorf("wikipedia api tidak mengembalikan artikel yang dapat dibaca")
	}
	return &out.Data, nil
}

type WikipediaSearchItem struct {
	PageID  int64  `json:"page_id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type WikipediaSearchResult struct {
	Query    string                `json:"query"`
	Language string                `json:"language"`
	Count    int                   `json:"count"`
	Results  []WikipediaSearchItem `json:"results"`
}

func (c *Client) WikipediaSearch(ctx context.Context, query string) (*WikipediaSearchResult, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/wikipedia/search"
	q := u.Query()
	q.Set("q", query)
	q.Set("lang", "id")
	q.Set("limit", "5")
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
		return nil, readAPIError("wikipedia search", resp)
	}
	var out APIResponse[WikipediaSearchResult]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	if out.Status != "success" || out.Data.Results == nil {
		return nil, fmt.Errorf("respons pencarian Wikipedia tidak valid")
	}
	return &out.Data, nil
}
