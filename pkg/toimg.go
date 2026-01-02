package pkg

import (
	"bytes"
	"context"
	"image"
	"image/png"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "toimg",
		Tags:        "convert",
		Description: "Change Media to Image",
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
			if _, err := client.SendImage(ctx, m.From, buf.Bytes(), "", m.ID); err != nil {
				m.Reply(ctx, "Gambarnya belum bisa dikirim, yaa.")
				return
			}
		},
	})
}
