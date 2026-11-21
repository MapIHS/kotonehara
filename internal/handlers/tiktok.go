package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "tiktok",
		Description: "Download video atau slide foto dari link TikTok",
		As:          []string{"tt"},
		Tags:        "downloader",
		IsQuery:     true,
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			args := strings.Fields(m.Query)
			if len(args) == 0 || !message.IsValidURL(args[0]) {
				m.Reply(ctx, "Link tidak valid. Pastikan kamu mengirimkan link yang benar.")
				return
			}

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.Shared(cfg.BASEApiURL, 60*time.Second)

			res, err := ap.Tiktok(ctx, args[0])
			if err != nil {
				m.Reply(ctx, "Gagal.")
				return
			}

			caption := fmt.Sprintf("*Title* :%s", res.Title)
			err = sendTikTokMedia(res.Images, res.Video, caption,
				func(medias []clients.AlbumMedia) error {
					_, err := client.SendAlbum(ctx, m.From, medias, m.ID)
					return err
				})
			if err != nil {
				replyAlbumFailure(ctx, m, err, "Gagal mengirim media TikTok.")
			}

		},
	})
}

func sendTikTokMedia(images []string, video *string, caption string, sendMedia func([]clients.AlbumMedia) error) error {
	var medias []clients.AlbumMedia
	for _, url := range images {
		if strings.TrimSpace(url) == "" {
			continue
		}
		text := ""
		if len(medias) == 0 {
			text = caption
		}
		medias = append(medias, clients.NewAlbumImageURL(url, text))
	}
	if len(medias) == 0 && video != nil && strings.TrimSpace(*video) != "" {
		medias = append(medias, clients.NewAlbumVideoURL(*video, false, caption))
	}
	if len(medias) == 0 {
		return fmt.Errorf("Media TikTok tidak ditemukan.")
	}
	if err := sendMedia(medias); err != nil {
		return fmt.Errorf("Gagal mengirim media TikTok: %w", err)
	}
	return nil
}
