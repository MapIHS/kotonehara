package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "pinterest",
		As:          []string{"pin"},
		Tags:        "downloader",
		Description: "Download video atau gambar dari Pinterest",
		IsPrefix:    true,
		IsQuery:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.Shared(cfg.BASEApiURL, 60*time.Second)
			res, err := ap.Pinterest(ctx, m.Query)
			if err != nil || res == nil {
				m.Reply(ctx, "Gagal mengunduh media dari Pinterest. Pastikan link valid dan coba lagi.")
				return
			}

			caption := fmt.Sprintf("📌 *%s*\n👤 %s\n\n%s", res.Title, res.Author, res.Description)

			buff, err := client.FetchBytes(res.URL)
			if err != nil {
				m.Reply(ctx, "Gagal mengunduh media.")
				return
			}

			if res.Type == "video" {
				client.SendVideo(ctx, m.From, buff, false, caption, m.ID)
			} else {
				client.SendImage(ctx, m.From, buff, caption, m.ID)
			}
		},
	})

	commands.Register(&commands.Command{
		Name:        "pinsearch",
		As:          []string{"pinterestsearch"},
		Tags:        "search",
		Description: "Cari gambar di Pinterest",
		IsPrefix:    true,
		IsQuery:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.Shared(cfg.BASEApiURL, 60*time.Second)
			res, err := ap.PinterestSearch(ctx, m.Query)
			if err != nil || res == nil || len(res.Results) == 0 {
				m.Reply(ctx, "Tidak menemukan hasil atau terjadi kesalahan.")
				return
			}

			limit := 5
			if len(res.Results) < limit {
				limit = len(res.Results)
			}

			var successCount int
			for i := 0; i < limit; i++ {
				item := res.Results[i]
				caption := fmt.Sprintf("🔍 *%s*\n🔗 %s", item.Title, item.Link)

				if len(item.Images) == 0 {
					client.SendText(ctx, m.From, caption+"\n_(Tidak ada gambar)_", m.ID)
					continue
				}

				imgData, err := client.FetchBytes(item.Images[0])
				if err != nil {
					client.SendText(ctx, m.From, caption+"\n_(Gagal memuat gambar)_", m.ID)
					continue
				}

				_, err = client.SendImage(ctx, m.From, imgData, caption, m.ID)
				if err == nil {
					successCount++
				}
			}

			if successCount == 0 {
				m.Reply(ctx, "Gagal mengirim gambar dari hasil pencarian Pinterest.")
			}
		},
	})
}
