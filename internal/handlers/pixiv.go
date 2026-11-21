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
		IsQuery:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.Shared(cfg.BASEApiURL, 60*time.Second)
			res, err := ap.Pixiv(ctx, m.Query)
			if err != nil || res == nil {
				m.Reply(ctx, "Gagal mendapatkan data Pixiv. Pastikan ID atau URL valid.")
				return
			}

			limit := len(res.URLs)
			if limit > 5 {
				limit = 5
			}

			headers := map[string]string{
				"Referer": "https://www.pixiv.net/",
			}

			caption := fmt.Sprintf("🎨 *%s*\n👤 %s", res.Title, res.Author)

			var albumMedia []clients.AlbumMedia
			for i := 0; i < limit; i++ {
				cap := ""
				if i == 0 {
					cap = caption
					if len(res.URLs) > 5 {
						cap += fmt.Sprintf("\n_(Menampilkan 5 dari %d gambar)_", len(res.URLs))
					}
				}

				item := clients.NewAlbumImageURL(res.URLs[i], cap)
				item.Headers = headers
				albumMedia = append(albumMedia, item)
			}

			if len(albumMedia) == 0 {
				m.Reply(ctx, "Gagal mengirim gambar Pixiv.")
				return
			}

			if _, err := client.SendAlbum(ctx, m.From, albumMedia, m.ID); err != nil {
				replyAlbumFailure(ctx, m, err, "Gagal mengirim gambar Pixiv.")
			}
		},
	})

	commands.Register(&commands.Command{
		Name:        "pixivsearch",
		As:          []string{"pxsearch"},
		Tags:        "search",
		Description: "Cari artwork di Pixiv: <kata kunci> [--mode safe|r18|all]",
		IsPrefix:    true,
		IsQuery:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			query, mode, err := parsePixivQuery(m.Query)
			if err != nil {
				m.Reply(ctx, err.Error())
				return
			}
			m.Reply(ctx, "Tunggu Sebentar ya.")
			ap := api.Shared(cfg.BASEApiURL, 60*time.Second)
			res, err := ap.PixivSearch(ctx, query, mode)
			if err != nil || res == nil || len(res.Results) == 0 {
				m.Reply(ctx, "Tidak menemukan hasil atau terjadi kesalahan saat mencari di Pixiv.")
				return
			}

			limit := 5
			if len(res.Results) < limit {
				limit = len(res.Results)
			}

			headers := map[string]string{
				"Referer": "https://www.pixiv.net/",
			}

			var albumMedia []clients.AlbumMedia
			for i := 0; i < limit; i++ {
				item := res.Results[i]
				url := fmt.Sprintf("https://www.pixiv.net/en/artworks/%s", item.ID)

				caption := fmt.Sprintf("🔍 *%s*\n👤 %s\n🔗 %s", item.Title, item.Author, url)

				media := clients.NewAlbumImageURL(item.URL, caption)
				media.Headers = headers
				albumMedia = append(albumMedia, media)
			}

			if len(albumMedia) == 0 {
				m.Reply(ctx, "Gagal mengirim gambar dari hasil pencarian Pixiv.")
				return
			}

			if _, err := client.SendAlbum(ctx, m.From, albumMedia, m.ID); err != nil {
				replyAlbumFailure(ctx, m, err, "Gagal mengirim gambar dari hasil pencarian Pixiv.")
			}
		},
	})
}
