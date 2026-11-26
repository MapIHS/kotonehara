package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	exec "github.com/MapIHS/kotonehara/internal/service/mediaproc"
)

var errStickerAudioStream = errors.New("media tidak memiliki audio")

func extractOpusAudio(ctx context.Context, raw []byte, duration time.Duration) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("media kosong")
	}
	if duration <= 0 {
		return nil, errors.New("durasi audio invalid")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg belum terpasang")
	}

	dir, err := os.MkdirTemp("", "audiostk")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	inFile := filepath.Join(dir, "input")
	outFile := filepath.Join(dir, "audio.opus")

	if err := os.WriteFile(inFile, raw, 0600); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-copyts", "-start_at_zero", "-i", inFile,
		"-map", "0:a:0",
		"-vn",
		// Match the animation's zero-based timeline, preserving any initial
		// audio delay and filling the end with silence when needed.
		"-af", "aresample=48000:async=1:first_pts=0,apad",
		"-t", fmt.Sprintf("%.3f", duration.Seconds()),
		"-c:a", "libopus",
		"-b:a", "128k",
		"-ac", "2",
		"-ar", "48000",
		"-application", "audio",
		"-f", "opus",
		outFile,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if bytes.Contains(out, []byte("matches no streams")) {
			return nil, errStickerAudioStream
		}
		return nil, fmt.Errorf("extract audio: %w: %s", err, bytes.TrimSpace(out))
	}

	audio, err := os.ReadFile(outFile)
	if err != nil {
		return nil, err
	}
	if len(audio) == 0 {
		return nil, errors.New("hasil audio kosong")
	}
	return audio, nil
}

func stickerAudio(ctx context.Context, client *clients.Client, m *message.Message, _ config.Config) {
	crop, ok := parseStickerCrop(m.Query)
	if !ok {
		_, _ = m.Reply(ctx, stickerCropUsage)
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	raw, err := client.DownloadMedia(opCtx, m)
	if err != nil || len(raw) == 0 {
		_, _ = m.Reply(ctx, "Media belum bisa diambil, yaa.")
		return
	}

	if sticker.IsGIF(raw) {
		_, _ = m.Reply(ctx, "GIF biasa tidak punya suara. Pilih video yang ada audionya, yaa.")
		return
	}

	stickerData, err := sticker.BuildStickerWithCrop(opCtx, raw, m.PushName, false, true, crop)
	if err != nil {
		if errors.Is(err, sticker.ErrAnimatedStickerTooLarge) {
			_, _ = m.Reply(ctx, "Stiker animasinya masih lebih dari 1 MB. Coba pakai video yang lebih pendek, yaa.")
			return
		}
		_, _ = m.Reply(ctx, fmt.Sprintf("Stikernya belum bisa dibuat: %s", err))
		return
	}
	// Compression can shorten the animation, so use the final frame timing.
	duration, err := sticker.AnimationDuration(stickerData)
	if err != nil {
		_, _ = m.Reply(ctx, "Animasinya belum bisa dibuat. Coba video yang lebih panjang, yaa.")
		return
	}

	audioData, err := extractOpusAudio(opCtx, raw, duration)
	if errors.Is(err, errStickerAudioStream) {
		_, _ = m.Reply(ctx, "Videonya tidak memiliki audio. Pilih video yang ada suaranya, yaa.")
		return
	}
	if err != nil {
		_, _ = m.Reply(ctx, "Audio belum bisa diekstrak. Pastikan videonya valid dan coba lagi, yaa.")
		return
	}

	if _, err := client.SendAudioSticker(opCtx, m.From, stickerData, audioData, duration, m.ID); err != nil {
		_, _ = m.Reply(ctx, "Stiker audio belum bisa dikirim, yaa.")
		return
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:        "stickeraudio",
		As:          []string{"sa", "audiosticker", "stkaud"},
		Description: "Ubah video bersuara menjadi stiker animasi dengan audio. " + stickerCropUsage,
		Tags:        "convert",
		IsPrefix:    true,
		IsMedia:     true,
		Exec:        stickerAudio,
	})
}
