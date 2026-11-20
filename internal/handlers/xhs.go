package handlers

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

var urlPattern = regexp.MustCompile(`https?://\S+`)

func firstURLFromText(text string) string {
	match := urlPattern.FindString(text)
	if match == "" {
		return ""
	}

	return strings.TrimRight(match, ".,;:!?。。，，、）)]}>")
}

func init() {
	commands.Register(&commands.Command{
		Name:        "xiahongsu",
		Description: "Download video/gambar dari link Xiaohongshu (RedNote)",
		As:          []string{"rednote", "xhs"},
		Tags:        "downloader",
		IsQuery:     true,
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			targetURL := firstURLFromText(m.Query)
			if targetURL == "" {
				m.Reply(ctx, "Link XHS/Rednote-nya belum ada.")
				return
			}

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.Shared(cfg.BASEApiURL, 60*time.Second)

			res, err := ap.Rednote(ctx, targetURL)
			if err != nil {
				m.Reply(ctx, err.Error())
				return
			}

			var albumMedia []clients.AlbumMedia
			for _, p := range res.Images {
				buff, err := client.FetchBytes(p.URL)
				if err != nil {
					continue
				}
				albumMedia = append(albumMedia, clients.NewAlbumImage(buff))
			}

			if res.Video != nil {
				buff, err := client.FetchBytes(res.Video.URL)
				if err != nil {
					if len(albumMedia) == 0 {
						m.Reply(ctx, "Maaf Terjadi kesalahan, yaa.")
						return
					}
				} else {
					albumMedia = append(albumMedia, clients.NewAlbumVideo(buff, false))
				}
			}

			if len(albumMedia) == 0 {
				m.Reply(ctx, "Media Rednote tidak ditemukan.")
				return
			}

			if _, err := client.SendAlbum(ctx, m.From, albumMedia, m.ID); err != nil {
				m.Reply(ctx, "Gagal mengirim media Rednote.")
			}
		},
	})
}
