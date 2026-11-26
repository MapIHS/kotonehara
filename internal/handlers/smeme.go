package handlers

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

func smeme(ctx context.Context, client *clients.Client, m *message.Message, _ config.Config) {
	args := strings.TrimSpace(m.Query)
	crop := sticker.CropDefault
	fields := strings.Fields(args)
	if len(fields) > 0 && (strings.EqualFold(fields[0], "-c") || strings.EqualFold(fields[0], "--crop")) {
		var ok bool
		if len(fields) >= 2 {
			crop, ok = parseStickerCrop(fields[0] + " " + fields[1])
		}
		if !ok {
			_, _ = m.Reply(ctx, "Format: .smeme -c atas teks atas|teks bawah. "+stickerCropUsage)
			return
		}
		args = strings.TrimSpace(strings.TrimPrefix(args, fields[0]))
		args = strings.TrimSpace(strings.TrimPrefix(args, fields[1]))
	}
	parts := strings.Split(args, "|")
	opts := meme.Options{TopText: strings.TrimSpace(parts[0]), Crop: crop}
	if len(parts) >= 2 {
		opts.BottomText = strings.TrimSpace(parts[1])
	}

	raw, err := client.DownloadMedia(ctx, m)
	if err != nil || len(raw) == 0 {
		m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
		return
	}

	isVideo := m.IsVideo || m.IsQuotedVideo || m.IsGif || m.IsQuotedGif ||
		m.IsQuotedStickerGif || sticker.IsGIF(raw) || sticker.IsAnimatedWebP(raw)
	var memeData []byte

	if isVideo {
		memeData, err = meme.RenderAnimated(ctx, raw, opts)
	} else {
		memeData, err = meme.Render(raw, opts)
	}

	if err != nil {
		m.Reply(ctx, "Gagal bikin meme: "+err.Error())
		return
	}

	stc, err := sticker.BuildSticker(ctx, memeData, m.PushName, isVideo, false)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Ada yang salah: %s", err))
		return
	}

	if _, err := client.SendSticker(ctx, m.From, stc, false, sticker.IsAnimatedWebP(stc), m.ID); err != nil {
		m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
		return
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:        "smeme",
		Description: "Buat stiker meme. Format: .smeme teks atas|teks bawah atau .smeme -c atas teks atas|teks bawah. " + stickerCropUsage,
		As:          []string{"stickermeme"},
		Tags:        "convert",
		IsPrefix:    true,
		IsMedia:     true,
		Exec:        smeme,
	})
}
