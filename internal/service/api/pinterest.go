package api

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
)

type PinterestDownload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      string `json:"author"`
	URL         string `json:"url"`
	Type        string `json:"type"`
}

type PinterestSearchItem struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Images []string `json:"images"`
	Author string   `json:"author"`
	Link   string   `json:"link"`
}

type PinterestSearchResponse struct {
	Results []PinterestSearchItem `json:"results"`
}

func (c *Client) Pinterest(ctx context.Context, targetURL string) (*PinterestDownload, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/pinterest"

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
		return nil, readAPIError("pinterest", resp)
	}

	var out APIResponse[PinterestDownload]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *Client) PinterestSearch(ctx context.Context, query string) (*PinterestSearchResponse, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/pinterest/search"

	q := u.Query()
	q.Set("q", query)
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
		return nil, readAPIError("pinterest search", resp)
	}

	var out APIResponse[PinterestSearchResponse]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

type PinterestUserProfile struct {
	Username    string `json:"username"`
	FullName    string `json:"fullName"`
	Bio         string `json:"bio"`
	Avatar      string `json:"avatar"`
	Followers   *int   `json:"followers"`
	Following   *int   `json:"following"`
	PinsCount   *int   `json:"pinsCount"`
	BoardsCount *int   `json:"boardsCount"`
}

type PinterestUserResponse struct {
	User PinterestUserProfile  `json:"user"`
	Pins []PinterestSearchItem `json:"pins"`
}

func (c *Client) PinterestUser(ctx context.Context, target string) (*PinterestUserResponse, error) {
	var out APIResponse[PinterestUserResponse]
	if err := c.profileRequest(ctx, "/api/pinterest/user", neturl.Values{"username": {target}}, &out); err != nil {
		return nil, err
	}
	if out.Data.User.Username == "" {
		return nil, fmt.Errorf("respons profil Pinterest tidak valid")
	}
	return &out.Data, nil
}
