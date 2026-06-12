package pkg

import (
	"context"
	"fmt"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/meme"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
)

func smeme(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	_ = cfg

	if m.Media == nil {
		m.Reply(ctx, "Kirim atau balas gambar dulu, yaa.")
		return
	}

	if m.IsVideo || m.IsQuotedVideo {
		m.Reply(ctx, "Sticker meme untuk gambar dulu, yaa.")
		return
	}

	args := strings.TrimSpace(m.Query)
	parts := strings.Split(args, "|")

	topText := ""
	bottomText := ""

	if len(parts) >= 1 {
		topText = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		bottomText = strings.TrimSpace(parts[1])
	}

	raw, err := client.WA.Download(ctx, m.Media)
	if err != nil || len(raw) == 0 {
		m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
		return
	}

	memeData, err := meme.Render(raw, meme.Options{
		TopText:    topText,
		BottomText: bottomText,
	})
	if err != nil {
		m.Reply(ctx, "Gagal bikin meme lokal: "+err.Error())
		return
	}

	stc, err := sticker.BuildSticker(ctx, memeData, m.PushName, false, false)
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
		Name:     "smeme",
		As:       []string{"stickermeme"},
		Tags:     "convert",
		IsPrefix: true,
		Exec:     smeme,
	})
}
