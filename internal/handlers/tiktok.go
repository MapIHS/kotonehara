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
		Name:     "tiktok",
		As:       []string{"tt"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
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
				func(data []byte, caption string) error {
					_, err := client.SendImage(ctx, m.From, data, caption, m.ID)
					return err
				},
				func(data []byte, caption string) error {
					_, err := client.SendVideo(ctx, m.From, data, false, caption, m.ID)
					return err
				})
			if err != nil {
				m.Reply(ctx, err.Error())
			}

		},
	})
}

// Keep media dispatch separate so photo posts and failed downloads can be tested
// without a WhatsApp connection.
func sendTikTokMedia(images []string, video *string, caption string, fetch func(string) ([]byte, error), sendImage, sendVideo func([]byte, string) error) error {
	if len(images) > 0 {
		sent := 0
		for _, imageURL := range images {
			if strings.TrimSpace(imageURL) == "" {
				continue
			}
			data, err := fetch(imageURL)
			if err != nil {
				continue
			}
			imageCaption := ""
			if sent == 0 {
				imageCaption = caption
			}
			if err := sendImage(data, imageCaption); err == nil {
				sent++
			}
		}
		if sent == 0 {
			return fmt.Errorf("Gagal mengunduh atau mengirim gambar TikTok.")
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
	if err := sendVideo(data, caption); err != nil {
		return fmt.Errorf("Gagal mengirim video TikTok: %w", err)
	}
	return nil
}
