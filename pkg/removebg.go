package pkg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "removebg",
		As:          []string{"rbg", "rmbg", "hapusbg"},
		Tags:        "convert",
		Description: "Hapus background gambar",
		IsPrefix:    true,
		IsMedia:     true,
		ShowWait:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if cfg.RemoveBGURL == "" {
				m.Reply(ctx, "Fitur remove background belum dikonfigurasi.")
				return
			}

			quality := "fast"
			if q := strings.TrimSpace(m.Query); q != "" {
				quality = q
			}

			opCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()

			raw, err := client.WA.Download(opCtx, m.Media)
			if err != nil || len(raw) == 0 {
				m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
				return
			}

			result, err := api.RemoveBG(opCtx, client.HTTP, cfg.RemoveBGURL, raw, "image.png", quality)
			if err != nil {
				m.Reply(ctx, fmt.Sprintf("Gagal menghapus background: %s", err))
				return
			}

			resultURL := cfg.RemoveBGURL + result.ResultURL
			imgData, err := client.FetchBytes(resultURL)
			if err != nil {
				m.Reply(ctx, fmt.Sprintf("Gagal mengunduh hasil: %s", err))
				return
			}

			if _, err := client.SendImage(opCtx, m.From, imgData, "", m.ID); err != nil {
				m.Reply(ctx, "Gambar hasilnya belum bisa dikirim, yaa.")
			}
		},
	})
}
