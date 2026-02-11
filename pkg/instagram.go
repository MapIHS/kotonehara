package pkg

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

func init() {
	commands.Register(&commands.Command{
		Name:     "instagram",
		As:       []string{"ig"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			args := strings.Fields(m.Query)

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.New(cfg.BASEApiURL, 15*time.Second)

			res, err := ap.Instagram(ctx, args[0])
			if err != nil {
				m.Reply(ctx, "Gagal.")
				return
			}

			totalMedia := len(res.Photos) + len(res.Videos)

			if totalMedia > 0 {
				for _, p := range res.Videos {
					buff, err := client.FetchBytes(p.URL)
					if err != nil {
						continue
					}

					client.SendVideo(ctx, m.From, buff, "", m.ID)
				}

				for _, p := range res.Photos {
					buff, err := client.FetchBytes(p.URL)
					if err != nil {
						continue
					}

					client.SendImage(ctx, m.From, buff, "", m.ID)
				}
			}

		},
	})
}
