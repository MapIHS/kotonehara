package bilibili

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/MapIHS/kotonehara/internal/service/api"
)

func mediaClient(base *http.Client) *http.Client {
	client := *base
	client.Timeout = 2 * time.Minute
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("terlalu banyak redirect media")
		}
		return validMediaURL(req.URL.String())
	}
	return &client
}

// FetchImage keeps original image/GIF bytes and uses the same bounded CDN
// downloader as videos, including range retries and redirect validation.
func FetchImage(ctx context.Context, client *http.Client, item api.BilibiliMedia, headers map[string]string, remaining int64) ([]byte, error) {
	if item.Type != "image" && item.Type != "gif" {
		return nil, errors.New("media Opus bukan gambar atau GIF")
	}
	if remaining <= 0 {
		return nil, errors.New("total media Opus melebihi batas 256 MiB")
	}
	limit := min(remaining, 32<<20)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	dir, err := os.MkdirTemp("", "kotonehara-bilibili-image-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	filename := filepath.Join(dir, "image")
	if _, err := download(ctx, mediaClient(client), item, headers, filename, limit); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/gif" && mime != "image/webp" {
		return nil, errors.New("CDN Opus tidak mengembalikan gambar yang didukung")
	}
	if item.Type == "gif" && mime != "image/gif" {
		return nil, errors.New("CDN Opus tidak mengembalikan GIF")
	}
	return data, nil
}
