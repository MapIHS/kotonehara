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
		Name:        "pixiv",
		As:          []string{"px"},
		Tags:        "downloader",
		Description: "Download artwork dari Pixiv",
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.Query == "" {
				m.Reply(ctx, "Harap sertakan ID Pixiv atau URL karyanya.\n\nContoh: `.pixiv 12345678` atau `.pixiv https://www.pixiv.net/en/artworks/12345678`")
				return
			}

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.New(cfg.BASEApiURL, 60*time.Second)
			res, err := ap.Pixiv(ctx, m.Query)
			if err != nil || res == nil {
				m.Reply(ctx, "Gagal mendapatkan data Pixiv. Pastikan ID atau URL valid.")
				return
			}

			limit := len(res.URLs)
			if limit > 5 {
				limit = 5 // Batasi maksimal 5 gambar agar tidak spam
			}

			headers := map[string]string{
				"Referer": "https://www.pixiv.net/",
			}

			caption := fmt.Sprintf("🎨 *%s*\n👤 %s", res.Title, res.Author)

			var successCount int
			for i := 0; i < limit; i++ {
				imgData, err := client.FetchBytesWithHeaders(res.URLs[i], headers)
				if err != nil {
					continue
				}
				
				cap := ""
				if i == 0 {
					cap = caption
					if len(res.URLs) > 5 {
						cap += fmt.Sprintf("\n_(Menampilkan 5 dari %d gambar)_", len(res.URLs))
					}
				}

				_, err = client.SendImage(ctx, m.From, imgData, cap, m.ID)
				if err == nil {
					successCount++
				}
			}

			if successCount == 0 {
				m.Reply(ctx, "Gagal mengirim gambar Pixiv.")
			}
		},
	})

	commands.Register(&commands.Command{
		Name:        "pixivsearch",
		As:          []string{"pxsearch"},
		Tags:        "search",
		Description: "Cari artwork di Pixiv",
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.Query == "" {
				m.Reply(ctx, "Harap sertakan kata kunci pencarian.\n\nContoh: `.pixivsearch anime girl`")
				return
			}

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.New(cfg.BASEApiURL, 60*time.Second)
			res, err := ap.PixivSearch(ctx, m.Query)
			if err != nil || res == nil || len(res.Results) == 0 {
				m.Reply(ctx, "Tidak menemukan hasil atau terjadi kesalahan saat mencari di Pixiv.")
				return
			}

			limit := 5
			if len(res.Results) < limit {
				limit = len(res.Results)
			}

			var successCount int
			headers := map[string]string{
				"Referer": "https://www.pixiv.net/",
			}

			for i := 0; i < limit; i++ {
				item := res.Results[i]
				url := fmt.Sprintf("https://www.pixiv.net/en/artworks/%s", item.ID)
				
				caption := fmt.Sprintf("🔍 *%s*\n👤 %s\n🔗 %s", item.Title, item.Author, url)
				
				imgData, err := client.FetchBytesWithHeaders(item.URL, headers)
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
				m.Reply(ctx, "Gagal mengirim gambar dari hasil pencarian Pixiv.")
			}
		},
	})
}
