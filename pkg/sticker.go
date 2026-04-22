package pkg

import (
	"context"
	"fmt"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

func stc(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if m.Media == nil {
		m.Reply(ctx, "Kirim atau balas gambar dulu, yaa.")
		return
	}

	raw, err := client.WA.Download(ctx, m.Media)
	if err != nil || len(raw) == 0 {
		m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
		return
	}

	isGif := false

	if m.IsVideo || m.IsQuotedVideo {
		isGif = true
	}

	stc, err := sticker.BuildSticker(ctx, raw, m.PushName, m.IsQuotedSticker, m.IsVideo || m.IsQuotedVideo)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Ada yang salah: %s", err))
	}

	if _, err := client.SendSticker(ctx, m.From, stc, false, isGif, nil); err != nil {
		m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
		return
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:     "sticker",
		As:       []string{"s", "stiker"},
		Tags:     "convert",
		IsPrefix: true,
		Exec:     stc,
	})
}
