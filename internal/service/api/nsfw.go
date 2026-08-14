package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Waifu.im (proxied via hararest)
// ---------------------------------------------------------------------------

type WaifuImImage struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type WaifuImResponse struct {
	Items []WaifuImImage `json:"items"`
}

// WaifuIm fetches an image from Waifu.im via the hararest NSFW proxy.
func (c *Client) WaifuIm(ctx context.Context, tag string, nsfw bool) (*WaifuImImage, error) {
	reqURL := fmt.Sprintf("%s/api/nsfw/waifu?tag=%s&nsfw=%t", c.BaseURL, url.QueryEscape(tag), nsfw)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, apiHTTPStatusError("waifu.im", resp.StatusCode, body)
	}

	// hararest returns { status: "success", data: { images: [...] } }
	var payload APIResponse[WaifuImResponse]
	if err := decodeAPIResponse(resp, &payload); err != nil {
		return nil, err
	}
	if len(payload.Data.Items) == 0 {
		return nil, fmt.Errorf("tidak ada gambar ditemukan untuk tag %s", tag)
	}

	return &payload.Data.Items[0], nil
}

// ---------------------------------------------------------------------------
// PurrBot (proxied via hararest)
// ---------------------------------------------------------------------------

type PurrBotResponse struct {
	Error bool   `json:"error"`
	Link  string `json:"link"`
}

// PurrBot fetches an NSFW gif from PurrBot via the hararest NSFW proxy.
func (c *Client) PurrBot(ctx context.Context, category string) (string, error) {
	reqURL := fmt.Sprintf("%s/api/nsfw/purrbot/%s", c.BaseURL, url.PathEscape(category))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", apiHTTPStatusError("purrbot", resp.StatusCode, body)
	}

	// hararest returns { status: "success", data: { error: false, link: "..." } }
	var payload APIResponse[PurrBotResponse]
	if err := decodeAPIResponse(resp, &payload); err != nil {
		return "", err
	}
	if payload.Data.Error {
		return "", fmt.Errorf("API returned error")
	}
	return payload.Data.Link, nil
}

// ---------------------------------------------------------------------------
// Danbooru (proxied via hararest)
// ---------------------------------------------------------------------------

type DanbooruPost struct {
	ID         int    `json:"id"`
	FileURL    string `json:"file_url"`
	LargeURL   string `json:"large_file_url"`
	PreviewURL string `json:"preview_file_url"`
	Tags       string `json:"tag_string"`
	Rating     string `json:"rating"`
	Score      int    `json:"score"`
}

// Danbooru fetches posts from Danbooru via the hararest NSFW proxy.
func (c *Client) Danbooru(ctx context.Context, tags string, limit int) ([]DanbooruPost, error) {
	reqURL := fmt.Sprintf("%s/api/nsfw/danbooru?tags=%s&limit=%d", c.BaseURL, url.QueryEscape(tags), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, apiHTTPStatusError("danbooru", resp.StatusCode, body)
	}

	// hararest returns { status: "success", data: [...posts] }
	var payload APIResponse[[]DanbooruPost]
	if err := decodeAPIResponse(resp, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// ---------------------------------------------------------------------------
// Yandere (direct access — NOT proxied, already works)
// ---------------------------------------------------------------------------

type YanderePost struct {
	FileURL    string `json:"file_url"`
	SampleURL  string `json:"sample_url"`
	PreviewURL string `json:"preview_url"`
	Tags       string `json:"tags"`
	Score      int    `json:"score"`
}

func (c *Client) Yandere(ctx context.Context, tags string, limit int) ([]YanderePost, error) {
	reqURL := fmt.Sprintf("https://yande.re/post.json?limit=%d&tags=%s+rating:explicit", limit, url.QueryEscape(tags))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res []YanderePost
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// NHentai (proxied via hararest)
// ---------------------------------------------------------------------------

type NhentaiGallery struct {
	ID      int    `json:"id"`
	MediaID string `json:"media_id"`
	Title   struct {
		English  string `json:"english"`
		Japanese string `json:"japanese"`
		Pretty   string `json:"pretty"`
	} `json:"title"`
	Cover struct {
		Path   string `json:"path"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	} `json:"cover"`
	Tags []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"tags"`
	NumPages     int `json:"num_pages"`
	NumFavorites int `json:"num_favorites"`
}

type NhentaiSearchResult struct {
	ID           int    `json:"id"`
	MediaID      string `json:"media_id"`
	EnglishTitle string `json:"english_title"`
	NumPages     int    `json:"num_pages"`
}

type NhentaiSearchResponse struct {
	Result []NhentaiSearchResult `json:"result"`
}

// NhentaiGallery fetches a gallery from NHentai via the hararest NSFW proxy.
func (c *Client) NhentaiGallery(ctx context.Context, id string) (*NhentaiGallery, error) {
	reqURL := fmt.Sprintf("%s/api/nsfw/nhentai/gallery/%s", c.BaseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, apiHTTPStatusError("nhentai", resp.StatusCode, body)
	}

	var payload APIResponse[NhentaiGallery]
	if err := decodeAPIResponse(resp, &payload); err != nil {
		return nil, err
	}
	return &payload.Data, nil
}

// NhentaiSearch searches NHentai via the hararest NSFW proxy.
func (c *Client) NhentaiSearch(ctx context.Context, query string) ([]NhentaiSearchResult, error) {
	reqURL := fmt.Sprintf("%s/api/nsfw/nhentai/search?query=%s", c.BaseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, apiHTTPStatusError("nhentai", resp.StatusCode, body)
	}

	var payload APIResponse[NhentaiSearchResponse]
	if err := decodeAPIResponse(resp, &payload); err != nil {
		return nil, err
	}
	return payload.Data.Result, nil
}

func NhentaiCoverURL(path string) string {
	return fmt.Sprintf("https://t.nhentai.net/%s", path)
}
