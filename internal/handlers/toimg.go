package handlers

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	exec "github.com/MapIHS/kotonehara/internal/service/mediaproc"
	_ "golang.org/x/image/webp"
)

func mediaToPNG(ctx context.Context, data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	// Go's WebP decoder does not support the animated WebP container.
	if len(data) < 30 || string(data[:4]) != "RIFF" || string(data[8:16]) != "WEBPVP8X" || data[20]&0x02 == 0 {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "toimg")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "sticker.webp")
	output := filepath.Join(dir, "frame.png")
	if err := os.WriteFile(input, data, 0600); err != nil {
		return nil, err
	}
	binary := "magick"
	if _, err := exec.LookPath(binary); err != nil {
		binary = "convert"
	}
	if err := exec.CommandContext(ctx, binary, input+"[0]", "-coalesce", output).Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(output)
}

func init() {
	commands.Register(&commands.Command{
		Name:        "toimg",
		As:          []string{"toimage"},
		Tags:        "convert",
		Description: "Change Media to Image",
		IsPrefix:    true,
		IsMedia:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			data, err := client.WA.Download(opCtx, m.Media)
			if err != nil || len(data) == 0 {
				m.Reply(ctx, "Media belum bisa diambil, yaa.")
				return
			}
			pngData, err := mediaToPNG(opCtx, data)
			if err != nil {
				if st := stickerMessageFromCommand(m); len(st.GetPngThumbnail()) > 0 {
					pngData, err = mediaToPNG(opCtx, st.GetPngThumbnail())
				}
			}
			if err != nil {
				m.Reply(ctx, "Media belum bisa diubah jadi gambar, yaa.")
				return
			}

			if _, err := client.SendImage(opCtx, m.From, pngData, getCaption(m), m.ID); err != nil {
				m.Reply(ctx, "Gambarnya belum bisa dikirim, yaa.")
				return
			}
		},
	})
}

func getCaption(m *message.Message) string {
	if m == nil || m.QuotedMsg == nil {
		return ""
	}
	if caption := m.QuotedMsg.GetImageMessage().GetCaption(); caption != "" {
		return strings.TrimSpace(caption)
	}
	if caption := m.QuotedMsg.GetVideoMessage().GetCaption(); caption != "" {
		return strings.TrimSpace(caption)
	}
	return ""
}
