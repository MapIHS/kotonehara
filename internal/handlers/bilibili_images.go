package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MapIHS/kotonehara/internal/service/api"
)

func sendBilibiliImages(ctx context.Context, result *api.BilibiliResult,
	fetch func(context.Context, api.BilibiliMedia, int64) ([]byte, error),
	send func(context.Context, []byte, bool, string) error,
) error {
	if len(result.Media) == 0 || len(result.Media) > 20 {
		return errors.New("Opus harus berisi 1–20 gambar/GIF")
	}
	for _, item := range result.Media {
		if item.Type != "image" && item.Type != "gif" {
			return errors.New("tipe media Opus tidak didukung")
		}
	}
	text := result.Description
	if text == "" {
		text = result.Title
	}
	caption := []rune(strings.TrimSpace(result.Author.Name + "\n\n" + text))
	if len(caption) > 900 {
		caption = append(caption[:900], []rune("...")...)
	}
	remaining := int64(256 << 20)
	sent := 0
	var lastErr error
	for _, item := range result.Media {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := fetch(ctx, item, remaining)
		if err != nil {
			lastErr = err
			continue
		}
		remaining -= int64(len(data))
		if remaining < 0 {
			return errors.New("total media Opus melebihi batas 256 MiB")
		}
		itemCaption := ""
		if sent == 0 {
			itemCaption = string(caption)
		}
		if err := send(ctx, data, http.DetectContentType(data) == "image/gif", itemCaption); err != nil {
			lastErr = err
			continue
		}
		sent++
	}
	if sent != len(result.Media) {
		return fmt.Errorf("%d dari %d media berhasil dikirim: %w", sent, len(result.Media), lastErr)
	}
	return nil
}
