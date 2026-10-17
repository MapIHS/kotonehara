package api

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
)

type TikTokResult struct {
	ID       string   `json:"id"`
	Region   string   `json:"region"`
	Title    string   `json:"title"`
	Cover    string   `json:"cover"`
	Duration int      `json:"duration"`
	Size     int64    `json:"size"`
	Video    *string  `json:"video"`
	Images   []string `json:"images"`
	Music    string   `json:"music"`

	MusicInfo music  `json:"musicInfo"`
	Played    int64  `json:"played"`
	Comments  int64  `json:"comments"`
	Share     int64  `json:"share"`
	Download  int64  `json:"download"`
	Uploaded  int64  `json:"uploaded"`
	Author    author `json:"author"`
}

type music struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Cover    string `json:"cover"`
	Author   string `json:"author"`
	Duration int    `json:"duration"`
}

type author struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func (c *Client) Tiktok(ctx context.Context, targetURL string) (*TikTokResult, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/tiktok/download"

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
		return nil, readAPIError("tiktok", resp)
	}

	var out APIResponse[TikTokResult]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

// TikTokFeed preserves the upstream cursor so callers can request another page.
type TikTokFeed struct {
	Lists  []TikTokResult `json:"lists"`
	NextID string         `json:"nextId"`
	Next   bool           `json:"next"`
}

var tikTokRegion = regexp.MustCompile(`^[A-Za-z]{2}$`)
var tikTokUsername = regexp.MustCompile(`^[A-Za-z0-9_.]{1,24}$`)

func (c *Client) tikTokFeedRequest(ctx context.Context, path string, query neturl.Values, dst any) error {
	endpoint, err := c.jobURL(path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return readAPIError("tiktok", resp)
	}
	return decodeAPIResponse(resp, dst)
}

func (c *Client) TikTokTrending(ctx context.Context, region string) ([]TikTokResult, error) {
	region = strings.ToUpper(strings.TrimSpace(region))
	if region == "" {
		region = "US"
	}
	if !tikTokRegion.MatchString(region) {
		return nil, fmt.Errorf("region harus berupa kode negara dua huruf, contoh ID")
	}
	var out APIResponse[[]TikTokResult]
	if err := c.tikTokFeedRequest(ctx, "/api/tiktok/trending", neturl.Values{"region": {region}}, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) TikTokSearch(ctx context.Context, query, cursor string) (*TikTokFeed, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("kata kunci TikTok diperlukan")
	}
	return c.tikTokFeed(ctx, "/api/tiktok/search", neturl.Values{"query": {query}}, cursor)
}

func (c *Client) TikTokUserFeed(ctx context.Context, username, cursor string) (*TikTokFeed, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if !tikTokUsername.MatchString(username) {
		return nil, fmt.Errorf("username TikTok tidak valid")
	}
	return c.tikTokFeed(ctx, "/api/tiktok/user", neturl.Values{"username": {username}}, cursor)
}

func (c *Client) tikTokFeed(ctx context.Context, path string, query neturl.Values, cursor string) (*TikTokFeed, error) {
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	var out APIResponse[TikTokFeed]
	if err := c.tikTokFeedRequest(ctx, path, query, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
