package handlers

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	_ "golang.org/x/image/webp"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "toimg",
		As:          []string{"toimage"},
		Tags:        "convert",
		Description: "Change Media to Image",
		IsPrefix:    true,
		IsMedia:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			data, err := client.DownloadMedia(ctx, m)
			if err != nil || len(data) == 0 {
				m.Reply(ctx, "Media belum bisa diambil, yaa.")
				return
			}
			mime := http.DetectContentType(data)
			if mime == "image/jpeg" || mime == "image/png" || mime == "image/webp" {
				if _, err := client.SendImage(ctx, m.From, data, getCaption(m), m.ID); err != nil {
					m.Reply(ctx, "Gambarnya belum bisa dikirim, yaa.")
				}
				return
			}
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				m.Reply(ctx, "Media belum bisa diproses, yaa.")
				return
			}

			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				m.Reply(ctx, "Gambarnya belum bisa dibuat, yaa.")
				return
			}

			if _, err := client.SendImage(ctx, m.From, buf.Bytes(), getCaption(m), m.ID); err != nil {
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
