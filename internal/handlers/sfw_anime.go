package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "swaifu",
		As:          []string{"sfwaifu"},
		Tags:        "anime",
		Description: "Gambar waifu (SFW)",
		IsPrefix:    true,
		Exec:        sfwAnime("waifu"),
	})
	commands.Register(&commands.Command{
		Name:        "smaid",
		As:          []string{"sfwmaid"},
		Tags:        "anime",
		Description: "Gambar maid (SFW)",
		IsPrefix:    true,
		Exec:        sfwAnime("maid"),
	})
	commands.Register(&commands.Command{
		Name:        "suniform",
		As:          []string{"sfwuniform"},
		Tags:        "anime",
		Description: "Gambar uniform (SFW)",
		IsPrefix:    true,
		Exec:        sfwAnime("uniform"),
	})
	commands.Register(&commands.Command{
		Name:        "sselfie",
		As:          []string{"sfwselfie"},
		Tags:        "anime",
		Description: "Gambar selfie anime (SFW)",
		IsPrefix:    true,
		Exec:        sfwAnime("selfies"),
	})
}

func sfwAnime(tag string) func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	return func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
		if cfg.BASEApiURL == "" {
			m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
			return
		}

		ap := api.Shared(cfg.BASEApiURL, 15*time.Second)

		img, err := ap.WaifuIm(ctx, tag, false)
		if err != nil {
			m.Reply(ctx, "❌ Gagal mengambil gambar: "+err.Error())
			return
		}

		buff, err := client.FetchBytes(img.URL)
		if err != nil {
			m.Reply(ctx, "❌ Gagal mengunduh gambar: "+err.Error())
			return
		}

		caption := fmt.Sprintf("🎨 %s\nSource: %s", tag, img.Source)
		_, err = client.SendImage(ctx, m.From, buff, caption, m.ID)
		if err != nil {
			m.Reply(ctx, "❌ Gagal mengirim gambar: "+err.Error())
		}
	}
}
