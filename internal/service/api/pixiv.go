package api

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
)

type PixivDownload struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	URLs        []string `json:"urls"`
}

type PixivSearchItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Author string `json:"author"`
}

type PixivSearchResponse struct {
	Results []PixivSearchItem `json:"results"`
}

func (c *Client) Pixiv(ctx context.Context, idOrURL string) (*PixivDownload, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/pixiv"

	q := u.Query()
	q.Set("id", idOrURL)
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
		return nil, readAPIError("pixiv", resp)
	}

	var out APIResponse[PixivDownload]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *Client) PixivSearch(ctx context.Context, query string, modes ...string) (*PixivSearchResponse, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/pixiv/search"
	mode, err := pixivMode(modes)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("q", query)
	q.Set("mode", mode)
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
		return nil, readAPIError("pixiv search", resp)
	}

	var out APIResponse[PixivSearchResponse]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

// pixivMode preserves the existing all-mode behavior for callers without options.
func pixivMode(modes []string) (string, error) {
	if len(modes) > 1 {
		return "", fmt.Errorf("gunakan satu mode Pixiv: safe, r18, atau all")
	}
	mode := "all"
	if len(modes) == 1 {
		mode = modes[0]
	}
	switch mode {
	case "safe", "r18", "all":
		return mode, nil
	}
	return "", fmt.Errorf("mode Pixiv harus safe, r18, atau all")
}

type PixivUserProfile struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Comment    string `json:"comment"`
	Avatar     string `json:"avatar"`
	Premium    bool   `json:"premium"`
	TwitterURL string `json:"twitterUrl"`
}

type PixivUserArtwork struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	NSFW      bool     `json:"nsfw"`
	XRestrict int      `json:"xRestrict"`
	Tags      []string `json:"tags"`
	PageCount int      `json:"pageCount"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
}

type PixivUserResponse struct {
	User     PixivUserProfile   `json:"user"`
	Artworks []PixivUserArtwork `json:"artworks"`
}

func (c *Client) PixivUser(ctx context.Context, target string, modes ...string) (*PixivUserResponse, error) {
	mode, err := pixivMode(modes)
	if err != nil {
		return nil, err
	}
	var out APIResponse[PixivUserResponse]
	if err := c.profileRequest(ctx, "/api/pixiv/user", neturl.Values{"id": {target}, "mode": {mode}}, &out); err != nil {
		return nil, err
	}
	if out.Data.User.ID == "" {
		return nil, fmt.Errorf("respons profil Pixiv tidak valid")
	}
	return &out.Data, nil
}
