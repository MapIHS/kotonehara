package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RedditMedia struct {
	AudioURL           string `json:"audio_url"`
	IsGIF              bool   `json:"is_gif"`
	URL                string `json:"url"`
	Type               string `json:"type"`
	Format             string `json:"format"`
	Quality            string `json:"quality"`
	QualityNumber      int    `json:"quality_number"`
	RequiresConversion bool   `json:"requires_conversion"`
	HasAudio           *bool  `json:"has_audio"`
}

type RedditResult struct {
	IsGallery          bool          `json:"is_gallery"`
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

// ValidateRedditMediaURL limits downloads and redirects to Reddit's own media hosts.
func ValidateRedditMediaURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return fmt.Errorf("URL media Reddit tidak valid")
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range []string{"i.redd.it", "v.redd.it", "preview.redd.it", "external-preview.redd.it", "i.redditmedia.com"} {
		if host == allowed {
			return nil
		}
	}
	return fmt.Errorf("host media Reddit tidak didukung")
}

func (c *Client) RedditMediaBytes(ctx context.Context, item RedditMedia) ([]byte, error) {
	if item.AudioURL != "" {
		if item.Type != "video" || item.RequiresConversion {
			return nil, fmt.Errorf("track audio Reddit tidak valid")
		}
		// Validate both tracks before downloading either one.
		if err := ValidateRedditMediaURL(item.AudioURL); err != nil {
			return nil, err
		}
		if err := ValidateRedditMediaURL(item.URL); err != nil {
			return nil, err
		}
		select {
		case redditMuxSlots <- struct{}{}:
			defer func() { <-redditMuxSlots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		video, err := c.redditTrackBytes(ctx, item, maxMediaSize)
		if err != nil {
			return nil, err
		}
		audio, err := c.redditTrackBytes(ctx, RedditMedia{URL: item.AudioURL, Type: "audio", Format: "mp4"}, maxMediaSize-int64(len(video)))
		if err != nil {
			return nil, err
		}
		return mergeRedditTracks(ctx, video, audio)
	}
	return c.redditTrackBytes(ctx, item, maxMediaSize)
}

var redditMuxSlots = make(chan struct{}, 2)

func (c *Client) redditTrackBytes(ctx context.Context, item RedditMedia, limit int64) ([]byte, error) {
	if item.RequiresConversion {
		return nil, fmt.Errorf("media memerlukan konversi eksternal yang tidak didukung")
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
	data, err := readResponseBody(resp, limit)
	if err != nil {
		return nil, err
	}
	mime := http.DetectContentType(data)
	mp4Audio := item.Type == "audio" && item.Format == "mp4" && mime == "video/mp4"
	if len(data) == 0 || (!strings.HasPrefix(mime, item.Type+"/") && !mp4Audio) {
		return nil, fmt.Errorf("respons bukan media %s yang valid (%s)", item.Type, mime)
	}
	return data, nil
}

// FFmpeg receives only downloaded local files, never network URLs or manifests.
func mergeRedditTracks(ctx context.Context, video, audio []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "kotonehara-reddit-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	videoPath, audioPath, output := filepath.Join(dir, "video.mp4"), filepath.Join(dir, "audio.mp4"), filepath.Join(dir, "output.mp4")
	if err := os.WriteFile(videoPath, video, 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(audioPath, audio, 0600); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-n", "-protocol_whitelist", "file", "-i", videoPath, "-protocol_whitelist", "file", "-i", audioPath, "-map", "0:v:0", "-map", "1:a:0", "-c", "copy", "-movflags", "+faststart", "-fs", fmt.Sprint(maxMediaSize+1), output)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("gagal menggabungkan video/audio Reddit; pastikan FFmpeg terpasang")
	}
	file, err := os.Open(output)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMediaSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maxMediaSize {
		return nil, fmt.Errorf("hasil video Reddit kosong atau melebihi batas ukuran")
	}
	return data, nil
}
