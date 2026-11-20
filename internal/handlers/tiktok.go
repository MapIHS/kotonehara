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
			err = sendTikTokMedia(res.Images, res.Video, caption, client.FetchBytes,
				func(medias []clients.AlbumMedia) error {
					_, err := client.SendAlbum(ctx, m.From, medias, m.ID)
					return err
				})
			if err != nil {
				m.Reply(ctx, err.Error())
			}

		},
	})
}

func sendTikTokMedia(images []string, video *string, caption string, fetch func(string) ([]byte, error), sendMedia func([]clients.AlbumMedia) error) error {
	var albumMedia []clients.AlbumMedia
	for _, imageURL := range images {
		if strings.TrimSpace(imageURL) == "" {
			continue
		}
		data, err := fetch(imageURL)
		if err != nil {
			continue
		}
		itemCaption := ""
		if len(albumMedia) == 0 {
			itemCaption = caption
		}
		albumMedia = append(albumMedia, clients.NewAlbumImage(data, itemCaption))
	}
	if len(albumMedia) > 0 {
		if err := sendMedia(albumMedia); err != nil {
			return fmt.Errorf("Gagal mengirim gambar TikTok: %w", err)
		}
		return nil
	}
	if video == nil || strings.TrimSpace(*video) == "" {
		return fmt.Errorf("Media TikTok tidak ditemukan.")
	}
	data, err := fetch(*video)
	if err != nil {
		return fmt.Errorf("Gagal mengunduh video TikTok: %w", err)
	}
	if err := sendMedia([]clients.AlbumMedia{clients.NewAlbumVideo(data, false, caption)}); err != nil {
		return fmt.Errorf("Gagal mengirim video TikTok: %w", err)
	}
	return nil
}
