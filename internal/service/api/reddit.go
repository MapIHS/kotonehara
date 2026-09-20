package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type RedditMedia struct {
	URL                string `json:"url"`
	Type               string `json:"type"`
	Format             string `json:"format"`
	Quality            string `json:"quality"`
	QualityNumber      int    `json:"quality_number"`
	RequiresConversion bool   `json:"requires_conversion"`
	HasAudio           *bool  `json:"has_audio"`
}

type RedditResult struct {
	SourceURL          string        `json:"source_url"`
	ResolvedURL        string        `json:"resolved_url"`
	Provider           string        `json:"provider"`
	Title              string        `json:"title"`
	Thumbnail          string        `json:"thumbnail"`
	Uploader           string        `json:"uploader"`
	Duration           *float64      `json:"duration"`
	DownloadURL        string        `json:"download_url"`
	RequiresConversion bool          `json:"requires_conversion"`
	Count              int           `json:"count"`
	Media              []RedditMedia `json:"media"`
}

func (c *Client) Reddit(ctx context.Context, target string) (*RedditResult, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/reddit/download"
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
		return nil, readAPIError("reddit", resp)
	}
	var out APIResponse[RedditResult]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	if out.Status != "success" || len(out.Data.Media) == 0 {
		return nil, fmt.Errorf("reddit api tidak mengembalikan media")
	}
	return &out.Data, nil
}

// ValidateRedditMediaURL limits downloads and redirects to the provider's media hosts.
func ValidateRedditMediaURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return fmt.Errorf("URL media Reddit tidak valid")
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range []string{"redd.it", "redditmedia.com", "sf-converter.com"} {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return nil
		}
	}
	return fmt.Errorf("host media Reddit tidak didukung")
}

func (c *Client) RedditMediaBytes(ctx context.Context, item RedditMedia) ([]byte, error) {
	if item.RequiresConversion {
		return nil, fmt.Errorf("media masih memerlukan konversi SaveFrom")
	}
	if err := ValidateRedditMediaURL(item.URL); err != nil {
		return nil, err
	}
	client := *c.HTTP
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("terlalu banyak redirect media Reddit")
		}
		return ValidateRedditMediaURL(req.URL.String())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unduhan Reddit HTTP %d", resp.StatusCode)
	}
	data, err := readResponseBody(resp, maxMediaSize)
	if err != nil {
		return nil, err
	}
	mime := http.DetectContentType(data)
	if len(data) == 0 || !strings.HasPrefix(mime, item.Type+"/") {
		return nil, fmt.Errorf("respons bukan media %s yang valid (%s)", item.Type, mime)
	}
	return data, nil
}
