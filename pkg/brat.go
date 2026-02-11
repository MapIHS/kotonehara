package pkg

import (
	"context"
	"fmt"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func brat(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	ap := api.New(cfg.BASEApiURL, 15*time.Second)

	res, err := ap.Brat(ctx, m.Query)
	if err != nil {
		m.Reply(ctx, "Gagal.")
		return
	}

	stc, err := sticker.BuildSticker(ctx, res, m.PushName, false)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Ada yang salah: %s", err))
	}

	if _, err := client.SendSticker(ctx, m.From, stc, m.ID); err != nil {
		m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
		return
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:     "brat",
		Tags:     "convert",
		IsPrefix: true,
		IsQuery:  true,
		Exec:     brat,
	})
}
