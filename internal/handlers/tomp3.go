package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/mediaproc"
)

const maxMP3Bytes = 15 * 1024 * 1024

var errMP3TooLarge = errors.New("hasil MP3 terlalu besar")

func convertToMP3(ctx context.Context, data []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "tomp3")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input")
	output := filepath.Join(dir, "output.mp3")
	if err := os.WriteFile(input, data, 0600); err != nil {
		return nil, err
	}
	cmd := mediaproc.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-nostdin", "-i", input, "-map", "0:a:0", "-vn", "-c:a", "libmp3lame",
		"-b:a", "128k", "-fs", strconv.Itoa(maxMP3Bytes), output)
	if log, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("encode MP3: %w: %s", err, log)
	}
	info, err := os.Stat(output)
	if err != nil {
		return nil, err
	}
	if info.Size() >= maxMP3Bytes {
		return nil, errMP3TooLarge
	}
	if info.Size() == 0 {
		return nil, errors.New("hasil MP3 kosong")
	}
	return os.ReadFile(output)
}

func init() {
	commands.Register(&commands.Command{
		Name: "tomp3", Tags: "convert",
		Description: "Ubah video atau audio yang direply menjadi MP3",
		IsPrefix:    true, IsMedia: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, _ config.Config) {
			opCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()

			data, err := client.DownloadMedia(opCtx, m)
			if err != nil || len(data) == 0 {
				m.Reply(ctx, "Media belum bisa diambil, yaa.")
				return
			}
			audio, err := convertToMP3(opCtx, data)
			if errors.Is(err, errMP3TooLarge) {
				m.Reply(ctx, "Hasil MP3 terlalu besar. Coba pakai klip yang lebih pendek, yaa.")
				return
			}
			if err != nil {
				m.Reply(ctx, "Media belum bisa diubah jadi MP3. Pastikan ada audionya, yaa.")
				return
			}
			if _, err := client.SendAudio(opCtx, m.From, audio, false, m.ID); err != nil {
				m.Reply(ctx, "MP3-nya belum bisa dikirim, yaa.")
			}
		},
	})
}
