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
		Name:     "toimg",
		Tags:     "convert",
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if !m.IsQuotedSticker && m.Media == nil {
				m.Reply(ctx, "Balas stikernya dulu, yaa.")
				return
			}
			data, err := client.WA.Download(ctx, m.Media)
			if err != nil || len(data) == 0 {
				m.Reply(ctx, "Stikernya belum bisa diambil, yaa.")
				return
			}
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				m.Reply(ctx, "Stikernya belum bisa diproses, yaa.")
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
