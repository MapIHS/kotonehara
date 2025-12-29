package pkg

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

			m.Reply(ctx, "Tunggu Sebentar ya.")

			ap := api.New(cfg.BASEApiURL, cfg.APIKEY, 15*time.Second)

			res, err := ap.Tiktok(ctx, args[0])
			if err != nil {
				m.Reply(ctx, err.Error())
				return
			}

			buff, err := client.FetchBytes(*res.Video)
			if err != nil {
				m.Reply(ctx, "Maaf Terjadi kesalahan, yaa.")
				return
			}

			caption := fmt.Sprintf("*Title* :%s", res.Title)
			client.SendVideo(ctx, m.From, buff, caption, m.ID)

		},
	})
}
