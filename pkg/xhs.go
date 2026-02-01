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
		Name:     "xiahongsu",
		As:       []string{"rednote", "xhs"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			args := strings.Fields(m.Query)

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.New(cfg.BASEApiURL, cfg.APIKEY, 15*time.Second)

			res, err := ap.Rednote(ctx, args[0])
			if err != nil {
				m.Reply(ctx, err.Error())
				return
			}

			totalMedia := len(res.Images)

			if totalMedia > 0 {
				for _, p := range res.Images {
					buff, err := client.FetchBytes(p.URL)
					if err != nil {
						continue
					}

					client.SendImage(ctx, m.From, buff, "", m.ID)
				}
			}

			if res.Video != nil {
				buff, err := client.FetchBytes(res.Video.URL)
				if err != nil {
					m.Reply(ctx, "Maaf Terjadi kesalahan, yaa.")
					return
				}
				client.SendVideo(ctx, m.From, buff, "", m.ID)

			}
		},
	})
}
