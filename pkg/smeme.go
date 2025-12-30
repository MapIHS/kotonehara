package pkg

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/s3"
)

func cleanMemeText(text string) string {
	if text == "" {
		return "_"
	}
	text = strings.ReplaceAll(text, "_", "__")
	text = strings.ReplaceAll(text, "-", "--")
	text = strings.ReplaceAll(text, " ", "_")
	text = strings.ReplaceAll(text, "?", "~q")
	text = strings.ReplaceAll(text, "%", "~p")
	text = strings.ReplaceAll(text, "#", "~h")
	text = strings.ReplaceAll(text, "/", "~s")
	return text
}

func smeme(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if m.Media == nil {
		m.Reply(ctx, "Kirim atau balas gambar dulu, yaa.")
		return
	}

	if m.IsVideo || m.IsQuotedVideo {
		return
	}

	args := strings.Join(strings.Fields(m.Body)[1:], " ")
	parts := strings.Split(args, "|")

	topText := "_"
	bottomText := "_"

	if len(parts) >= 1 {
		topText = cleanMemeText(strings.TrimSpace(parts[0]))
	}
	if len(parts) >= 2 {
		bottomText = cleanMemeText(strings.TrimSpace(parts[1]))
	}

	raw, err := client.WA.Download(ctx, m.Media)
	if err != nil || len(raw) == 0 {
		m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
		return
	}

	ext := "png"

	m.Reply(ctx, "Sebentar, sedang meracik meme...")

	s3p := s3.New(cfg.BASES3URL, 15*time.Second)
	publicURL, err := s3p.Upload("temp."+ext, raw)
	if err != nil {
		m.Reply(ctx, "Gagal upload ke server sementara: "+err.Error())
		return
	}

	targetURL := fmt.Sprintf("%s/images/custom/%s/%s.%s?background=%s",
		cfg.MemeHost,
		topText,
		bottomText,
		ext,
		publicURL,
	)

	resp, err := http.Get(targetURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	memeData, _ := io.ReadAll(resp.Body)

	stc, err := sticker.BuildSticker(ctx, memeData, m.PushName, false)
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
		Name:     "smeme",
		As:       []string{"stickermeme"},
		Tags:     "convert",
		IsPrefix: true,
		Exec:     smeme,
	})
}
