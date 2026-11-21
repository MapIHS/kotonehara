package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func threads(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	args := strings.Fields(m.Query)
	if len(args) == 0 || !message.IsValidURL(args[0]) {
		m.Reply(ctx, "Link Threads tidak valid. Pastikan kamu mengirimkan link yang benar.")
		return
	}

	m.Reply(ctx, "Tunggu Sebentar ya.")

	ap := api.Shared(cfg.BASEApiURL, 120*time.Second)
	res, err := ap.Threads(ctx, args[0])
	if err != nil {
		m.Reply(ctx, "Gagal: "+err.Error())
		return
	}

	var albumMedia []clients.AlbumMedia
	for _, item := range res.Items {
		mediaURL := strings.TrimSpace(item.BestURL())
		if mediaURL == "" {
			continue
		}

		caption := ""
		if len(albumMedia) == 0 {
			caption = threadsCaption(item)
		}
		isVideo := item.MediaType == api.ThreadsMediaTypeVideo || item.MediaType == api.ThreadsMediaTypeGIF
		albumMedia = append(albumMedia, clients.AlbumMedia{
			URL:         mediaURL,
			Caption:     caption,
			IsVideo:     isVideo,
			GifPlayback: item.MediaType == api.ThreadsMediaTypeGIF,
		})
	}

	if len(albumMedia) == 0 {
		m.Reply(ctx, "Media Threads tidak ditemukan.")
		return
	}

	if _, err := client.SendAlbum(ctx, m.From, albumMedia, m.ID); err != nil {
		replyAlbumFailure(ctx, m, err, "Gagal mengirim media Threads.")
	}
}

func threadsCaption(item api.ThreadsMediaItem) string {
	parts := make([]string, 0, 2)
	if item.Username != "" {
		parts = append(parts, item.Username)
	}
	if item.Caption != "" {
		parts = append(parts, item.Caption)
	}
	caption := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if len(caption) > 900 {
		caption = caption[:900] + "..."
	}
	return caption
}

func init() {
	commands.Register(&commands.Command{
		Name:        "threads",
		Description: "Download video/gambar dari link Threads",
		As:          []string{"thread", "threadsdl", "thdl"},
		Tags:        "downloader",
		IsQuery:     true,
		IsPrefix:    true,
		Exec:        threads,
	})
}
