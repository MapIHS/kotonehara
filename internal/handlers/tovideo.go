package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	exec "github.com/MapIHS/kotonehara/internal/service/mediaproc"
)

func convertToVideo(ctx context.Context, data []byte) ([]byte, bool, error) {
	dir, err := os.MkdirTemp("", "tovideo")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(dir)

	inFile := filepath.Join(dir, "input")
	outFile := filepath.Join(dir, "output.mp4")

	if err := os.WriteFile(inFile, data, 0600); err != nil {
		return nil, false, err
	}

	// FFmpeg can decode only the first frame of some animated WebP files.
	// Check the VP8X animation flag before trying the video encoder.
	animatedWebP := len(data) >= 30 && string(data[:4]) == "RIFF" &&
		string(data[8:16]) == "WEBPVP8X" && data[20]&0x02 != 0
	encode := func(input string) error {
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", input,
			"-map", "0:v:0", "-map", "0:a?", "-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2",
			"-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart"}
		for _, encoder := range []string{"libx264", "libopenh264"} {
			out, err := exec.CommandContext(ctx, "ffmpeg", append(args, "-c:v", encoder, outFile)...).CombinedOutput()
			if err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if encoder == "libopenh264" {
				return fmt.Errorf("encode MP4: %w: %s", err, out)
			}
		}
		return nil
	}
	if animatedWebP {
		gifFile := filepath.Join(dir, "frames.gif")
		if err := exec.CommandContext(ctx, "magick", inFile, "-coalesce", gifFile).Run(); err != nil {
			if err := exec.CommandContext(ctx, "convert", inFile, "-coalesce", gifFile).Run(); err != nil {
				return nil, false, fmt.Errorf("decode animated WebP: %w", err)
			}
		}
		inFile = gifFile
	}
	if err := encode(inFile); err != nil {
		return nil, false, err
	}

	res, err := os.ReadFile(outFile)
	return res, animatedWebP, err
}

func init() {
	commands.Register(&commands.Command{
		Name:        "tovideo",
		As:          []string{"tomp4"},
		Tags:        "convert",
		Description: "Change Media to Video",
		IsPrefix:    true,
		IsMedia:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			opCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			data, err := client.WA.Download(opCtx, m.Media)
			if err != nil || len(data) == 0 {
				m.Reply(ctx, "Media belum bisa diambil, yaa.")
				return
			}

			vidData, isGif, err := convertToVideo(opCtx, data)
			if err != nil {
				m.Reply(ctx, "Video belum bisa diproses, yaa.")
				return
			}
			if _, err := client.SendVideo(opCtx, m.From, vidData, isGif, getCaption(m), m.ID); err != nil {
				m.Reply(ctx, "Videonya belum bisa dikirim, yaa.")
				return
			}
		},
	})
}
