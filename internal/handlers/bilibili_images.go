package handlers

import (
	"context"
	"errors"
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
	medias := make([]clients.AlbumMedia, 0, len(result.Media))
	for i, item := range result.Media {
		if err := ctx.Err(); err != nil {
			return err
		}
		text := ""
		if i == 0 {
			text = string(caption)
		}
		medias = append(medias, clients.AlbumMedia{
			Caption: text, IsVideo: item.Type == "gif", GifPlayback: item.Type == "gif",
			Fetch: func(ctx context.Context, limit int64) ([]byte, error) {
				return fetch(ctx, item, limit)
			},
		})
	}
	return sendAlbum(ctx, medias)
}
