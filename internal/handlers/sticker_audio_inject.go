package handlers

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/mediaproc"
)

const (
	maxStickerAudioSourceBytes   = 16 << 20
	maxStillStickerAudioDuration = 10 * time.Second
	stickerAudioInjectUsage      = "Balas stiker dengan file audio sebagai dokumen dan isi caption .sainject, yaa."
)

func stillStickerDuration(ctx context.Context, audio []byte) (time.Duration, error) {
	f, err := os.CreateTemp("", "sticker-audio-source")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(audio)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return 0, err
	}
	duration, err := mediaproc.ProbeDuration(ctx, f.Name())
	return min(duration, maxStillStickerAudioDuration), err
}

func injectStickerAudio(ctx context.Context, client *clients.Client, m *message.Message, _ config.Config) {
	source := message.UnwrapMessage(m.Message).GetDocumentMessage()
	target := stickerMessage(m.QuotedMsg)
	if source == nil || target == nil {
		_, _ = m.Reply(ctx, stickerAudioInjectUsage)
		return
	}
	if source.GetFileLength() > maxStickerAudioSourceBytes {
		_, _ = m.Reply(ctx, "Pilih audio maksimal 16 MiB, yaa.")
		return
	}
	if target.GetIsLottie() || target.GetMimetype() == "application/was" {
		_, _ = m.Reply(ctx, "Pilih stiker WebP biasa atau animasi, yaa.")
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	data, err := client.WA.Download(opCtx, target)
	if err != nil || len(data) == 0 {
		_, _ = m.Reply(ctx, "Stikernya belum bisa diambil, yaa.")
		return
	}
	audio, err := client.WA.Download(opCtx, source)
	if err != nil || len(audio) == 0 || len(audio) > maxStickerAudioSourceBytes {
		_, _ = m.Reply(ctx, "File audionya belum bisa diambil, yaa.")
		return
	}
	if sticker.IsAnimatedWebP(data) {
		data, err = sticker.BuildSticker(opCtx, data, m.PushName, true, true)
	} else {
		var duration time.Duration
		duration, err = stillStickerDuration(opCtx, audio)
		if err == nil {
			data, err = sticker.HoldStill(opCtx, data, m.PushName, duration)
		}
	}
	if err != nil {
		_, _ = m.Reply(ctx, "Stikernya belum bisa disiapkan untuk audio, yaa.")
		return
	}
	duration, err := sticker.AnimationDuration(data)
	if err != nil {
		_, _ = m.Reply(ctx, "Durasi animasi stikernya belum bisa dibaca, yaa.")
		return
	}
	opus, err := extractOpusAudio(opCtx, audio, duration)
	if err != nil {
		_, _ = m.Reply(ctx, "Audionya belum bisa diproses, yaa.")
		return
	}
	if _, err := client.SendAudioSticker(opCtx, m.From, data, opus, duration, m.ID); err != nil {
		_, _ = m.Reply(ctx, "Stiker audio belum bisa dikirim, yaa.")
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:        "sainject",
		As:          []string{"injectaudio"},
		Description: stickerAudioInjectUsage,
		Tags:        "convert",
		IsPrefix:    true,
		Exec:        injectStickerAudio,
	})
}
