package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func sendBilibiliImages(ctx context.Context, result *api.BilibiliResult,
	fetch func(context.Context, api.BilibiliMedia, int64) ([]byte, error),
	sendAlbum func(context.Context, []clients.AlbumMedia) error,
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

	var albumMedia []clients.AlbumMedia
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
		if len(albumMedia) == 0 {
			itemCaption = string(caption)
		}
		isGif := http.DetectContentType(data) == "image/gif"
		albumMedia = append(albumMedia, clients.AlbumMedia{
			Data:        data,
			Caption:     itemCaption,
			IsVideo:     isGif,
			GifPlayback: isGif,
		})
	}

	if len(albumMedia) > 0 {
		return sendAlbum(ctx, albumMedia)
	}

	if lastErr != nil {
		return fmt.Errorf("gagal mengunduh media Opus: %w", lastErr)
	}
	return errors.New("tidak ada media Opus yang berhasil diunduh")
}
