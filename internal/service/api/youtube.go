package api

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"time"
)

type channelInfo struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	Name        string `json:"name"`
	Subscribers int    `json:"subscribers"`
	Verified    bool   `json:"verified"`
}

type videoInfo struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Thumbnail   string      `json:"thumbnail"`
	Description string      `json:"description"`
	Duration    float64     `json:"duration"`
	Views       int         `json:"views"`
	Likes       int         `json:"likes"`
	Comments    int         `json:"comments"`
	Channel     channelInfo `json:"channel"`
	Videos      []string    `json:"videos"`
}

func (c *Client) YoutubeInfo(ctx context.Context, targetURL string) (*videoInfo, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/youtube/info"

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
		return nil, readAPIError("Youtube", resp)
	}

	var out APIResponse[videoInfo]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *Client) YoutubeDownload(ctx context.Context, targetURL string, quality string, isVideo bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	input := map[string]string{"url": targetURL, "format": "audio"}
	if isVideo {
		input["format"] = "video"
	}
	if isVideo && quality != "" {
		input["quality"] = quality
	}
	job, err := c.runMediaJob(ctx, "/api/youtube/jobs", input, jobPollInterval)
	if err != nil {
		return nil, err
	}
	if job.Result.Kind != "file" || job.Result.Size <= 0 || job.Result.Size > maxMediaSize {
		return nil, fmt.Errorf("Hasil file job YouTube tidak valid atau terlalu besar")
	}
	endpoint, err := c.jobURL("/api/jobs/" + job.ID + "/file")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/octet-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, readAPIError("youtube", resp)
	}

	data, err := readResponseBody(resp, maxMediaSize)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != job.Result.Size {
		return nil, fmt.Errorf("File job YouTube tidak lengkap")
	}
	return data, nil
}

type searchChannel struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type YoutubeSearchResult struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Thumbnail string        `json:"thumbnail"`
	Duration  float64       `json:"duration"`
	Views     int           `json:"views"`
	Channel   searchChannel `json:"channel"`
	URL       string        `json:"url"`
}

func (c *Client) YoutubeSearch(ctx context.Context, query string, limit int) ([]YoutubeSearchResult, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/youtube/search"

	q := u.Query()
	q.Set("q", query)
	q.Set("limit", fmt.Sprintf("%d", limit))
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
		return nil, readAPIError("youtube search", resp)
	}

	var out APIResponse[[]YoutubeSearchResult]
	if err := decodeAPIResponse(resp, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}
