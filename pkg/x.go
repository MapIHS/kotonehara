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
		Name:     "x",
		As:       []string{"x", "twitter"},
		Tags:     "downloader",
		IsQuery:  true,
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			args := strings.Fields(m.Query)

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.New(cfg.BASEApiURL, 60*time.Second)

			res, err := ap.X(ctx, args[0])
			if err != nil {
				m.Reply(ctx, "Gagal.")
				return
			}

			mediaLinks := res.MediaLinks[len(res.MediaLinks)-1]

			if mediaLinks.URL != "" {
				buff, err := client.FetchBytes(mediaLinks.URL)
				if err != nil {
					m.Reply(ctx, "Gagal mengambil media.")
					return
				}

				_, err = client.SendVideo(ctx, m.From, buff, false, "", m.ID)
				if err != nil {
					m.Reply(ctx, "Gagal mengirim video.")
					return
				}
			}

		},
	})
}
