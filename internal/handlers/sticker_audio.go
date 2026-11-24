package handlers

import (
	"bytes"
	"context"
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

// extractOpusAudio extracts the first audio stream from raw media bytes and
// encodes it as Opus in an OGG container. Returns the Opus bytes.
func extractOpusAudio(ctx context.Context, raw []byte) ([]byte, error) {
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
		"-i", inFile,
		"-map", "0:a:0",
		"-vn",
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
		return nil, fmt.Errorf("extract audio: %w: %s", err, bytes.TrimSpace(out))
	}

	return os.ReadFile(outFile)
}

func stickerAudio(ctx context.Context, client *clients.Client, m *message.Message, _ config.Config) {
	opCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	raw, err := client.WA.Download(opCtx, m.Media)
	if err != nil || len(raw) == 0 {
		_, _ = m.Reply(ctx, "Media belum bisa diambil, yaa.")
		return
	}

	// Extract audio from the video/GIF.
	audioData, err := extractOpusAudio(opCtx, raw)
	if err != nil {
		_, _ = m.Reply(ctx, fmt.Sprintf("Audionya belum bisa diekstrak: %s", err))
		return
	}

	// Build animated sticker from the same video.
	stickerData, err := sticker.BuildSticker(opCtx, raw, m.PushName, false, true)
	if err != nil {
		_, _ = m.Reply(ctx, fmt.Sprintf("Stikernya belum bisa dibuat: %s", err))
		return
	}

	if _, err := client.SendAudioSticker(opCtx, m.From, stickerData, audioData, true, m.ID); err != nil {
		_, _ = m.Reply(ctx, "Stiker audio belum bisa dikirim, yaa.")
		return
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:        "stickeraudio",
		As:          []string{"sa", "audiosticker", "stkaud"},
		Description: "Ubah video/GIF menjadi stiker GIF dengan audio",
		Tags:        "convert",
		IsPrefix:    true,
		IsMedia:     true,
		Exec:        stickerAudio,
	})
}
