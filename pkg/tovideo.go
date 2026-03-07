package pkg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
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

	isGif := false

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inFile, "-c:v", "libopenh264", "-pix_fmt", "yuv420p", outFile)
	if err := cmd.Run(); err != nil {
		cmd = exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inFile, "-pix_fmt", "yuv420p", outFile)
		if err := cmd.Run(); err != nil {
			isGif = true
			gifFile := filepath.Join(dir, "temp.gif")
			magickCmd := exec.CommandContext(ctx, "magick", inFile, gifFile)
			if err := magickCmd.Run(); err != nil {
				convertCmd := exec.CommandContext(ctx, "convert", inFile, gifFile)
				if err := convertCmd.Run(); err != nil {
					return nil, false, err
				}
			}

			gifToMp4 := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", gifFile, "-c:v", "libopenh264", "-pix_fmt", "yuv420p", outFile)
			if err := gifToMp4.Run(); err != nil {
				gifToMp4Fallback := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", gifFile, "-pix_fmt", "yuv420p", outFile)
				if err := gifToMp4Fallback.Run(); err != nil {
					return nil, false, err
				}
			}
		}
	}

	res, err := os.ReadFile(outFile)
	return res, isGif, err
}

func init() {
	commands.Register(&commands.Command{
		Name:        "tovideo",
		Tags:        "convert",
		Description: "Change Media to Video",
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.Media == nil {
				m.Reply(ctx, "Balas Medianya dulu, yaa.")
				return
			}

			data, err := client.WA.Download(ctx, m.Media)
			if err != nil || len(data) == 0 {
				m.Reply(ctx, "Media belum bisa diambil, yaa.")
				return
			}

			vidData, isGif, err := convertToVideo(ctx, data)
			if err != nil {
				m.Reply(ctx, "Video belum bisa diproses, yaa.")
				return
			}
			if _, err := client.SendVideo(ctx, m.From, vidData, isGif, "", m.ID); err != nil {
				m.Reply(ctx, "Videonya belum bisa dikirim, yaa.")
				return
			}
		},
	})
}
