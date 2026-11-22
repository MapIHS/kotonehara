package handlers

import (
	"context"
	"errors"
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
		Name:        "instagram",
		Description: "Download post, reel, atau story dari link Instagram",
		As:          []string{"ig", "igdl"},
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

			ap := api.Shared(cfg.BASEApiURL, 150*time.Second)

			res, err := ap.Instagram(ctx, args[0])
			if err != nil {
				m.Reply(ctx, instagramFailureReply(err))
				return
			}

			var albumMedia []clients.AlbumMedia
			for _, p := range res.Videos {
				albumMedia = append(albumMedia, clients.NewAlbumVideoURL(p.URL, false))
			}
			for _, p := range res.Photos {
				albumMedia = append(albumMedia, clients.NewAlbumImageURL(p.URL))
			}

			if _, err := client.SendAlbum(ctx, m.From, albumMedia, m.ID); err != nil {
				replyAlbumFailure(ctx, m, err, "Gagal mengirim media Instagram.")
			}

		},
	})
}

func instagramFailureReply(err error) string {
	switch {
	case errors.Is(err, api.ErrInstagramUnavailable):
		return "Media Instagram tidak tersedia untuk akses publik, yaa."
	case errors.Is(err, api.ErrInstagramBusy):
		return "Instagram sedang sibuk. Coba lagi sebentar, yaa."
	default:
		return "Gagal mengambil media Instagram. Coba lagi sebentar, yaa."
	}
}
