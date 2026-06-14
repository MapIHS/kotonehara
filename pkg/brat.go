package pkg

import (
	"context"
	"fmt"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	bratimage "github.com/MapIHS/kotonehara/internal/media/brat"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
)

func brat(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	_ = cfg

	res, err := bratimage.Render(bratimage.Options{Text: m.Query})
	if err != nil {
		m.Reply(ctx, "Gagal bikin brat lokal: "+err.Error())
		return
	}

	stc, err := sticker.BuildSticker(ctx, res, m.PushName, false, false)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Ada yang salah: %s", err))
		return
	}

	if _, err := client.SendSticker(ctx, m.From, stc, false, false, m.ID); err != nil {
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
