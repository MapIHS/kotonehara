package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type WaifuImImage struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type WaifuImResponse struct {
	Images []WaifuImImage `json:"images"`
}

func (c *Client) WaifuIm(ctx context.Context, tag string, nsfw bool) (*WaifuImImage, error) {
	reqURL := fmt.Sprintf("https://api.waifu.im/search?included_tags=%s&is_nsfw=%t", url.QueryEscape(tag), nsfw)
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

	var res WaifuImResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if len(res.Images) == 0 {
		return nil, fmt.Errorf("no images found")
	}
	return &res.Images[0], nil
}

type PurrBotResponse struct {
	Error bool   `json:"error"`
	Link  string `json:"link"`
}

func (c *Client) PurrBot(ctx context.Context, category string) (string, error) {
	reqURL := fmt.Sprintf("https://purrbot.site/api/img/nsfw/%s/gif", url.PathEscape(category))
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
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res PurrBotResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	if res.Error {
		return "", fmt.Errorf("API returned error")
	}
	return res.Link, nil
}

type Rule34Post struct {
	FileURL    string `json:"file_url"`
	SampleURL  string `json:"sample_url"`
	PreviewURL string `json:"preview_url"`
	Tags       string `json:"tags"`
	Score      int    `json:"score"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

func (c *Client) Rule34(ctx context.Context, tags string, limit int) ([]Rule34Post, error) {
	reqURL := fmt.Sprintf("https://api.rule34.xxx/index.php?page=dapi&s=post&q=index&json=1&limit=%d&tags=%s", limit, url.QueryEscape(tags))
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

	var res []Rule34Post
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res, nil
}

type DanbooruPost struct {
	ID         int    `json:"id"`
	FileURL    string `json:"file_url"`
	LargeURL   string `json:"large_file_url"`
	PreviewURL string `json:"preview_file_url"`
	Tags       string `json:"tag_string"`
	Rating     string `json:"rating"`
	Score      int    `json:"score"`
}

func (c *Client) Danbooru(ctx context.Context, tags string, limit int) ([]DanbooruPost, error) {
	reqURL := fmt.Sprintf("https://danbooru.donmai.us/posts.json?limit=%d&tags=%s+rating:explicit", limit, url.QueryEscape(tags))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res []DanbooruPost
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res, nil
}

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

type NhentaiGallery struct {
	ID      int    `json:"id"`
	MediaID string `json:"media_id"`
	Title   struct {
		English  string `json:"english"`
		Japanese string `json:"japanese"`
		Pretty   string `json:"pretty"`
	} `json:"title"`
	Images struct {
		Pages []struct {
			T string `json:"t"`
			W int    `json:"w"`
			H int    `json:"h"`
		} `json:"pages"`
		Cover struct {
			T string `json:"t"`
		} `json:"cover"`
	} `json:"images"`
	Tags []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"tags"`
	NumPages     int `json:"num_pages"`
	NumFavorites int `json:"num_favorites"`
}

type NhentaiSearchResponse struct {
	Result []NhentaiGallery `json:"result"`
}

func (c *Client) NhentaiGallery(ctx context.Context, id string) (*NhentaiGallery, error) {
	reqURL := fmt.Sprintf("https://nhentai.net/api/gallery/%s", url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res NhentaiGallery
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) NhentaiSearch(ctx context.Context, query string) ([]NhentaiGallery, error) {
	reqURL := fmt.Sprintf("https://nhentai.net/api/galleries/search?query=%s", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res NhentaiSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Result, nil
}

func nhentaiExt(t string) string {
	switch t {
	case "j":
		return "jpg"
	case "p":
		return "png"
	case "w":
		return "webp"
	default:
		return "jpg"
	}
}

func NhentaiCoverURL(mediaID, ext string) string {
	return fmt.Sprintf("https://t.nhentai.net/galleries/%s/cover.%s", mediaID, nhentaiExt(ext))
}
