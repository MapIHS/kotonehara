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
		Name:        "waifu",
		Description: "Gambar acak waifu (NSFW)",
		Tags:        "nsfw",
		IsPrefix:    true,
		IsPrivate:   true,
		Disable:     true,
		Exec:        nsfwAnime("waifu"),
	})
	commands.Register(&commands.Command{
		Name:        "neko",
		Description: "Gambar acak neko (NSFW)",
		As:          []string{"nekonsfw"},
		Tags:        "nsfw",
		IsPrefix:    true,
		IsPrivate:   true,
		Disable:     true,
		Exec:        nsfwAnime("neko"),
	})

	commands.Register(&commands.Command{
		Name:        "hentai",
		Description: "Gambar acak hentai (NSFW)",
		Tags:        "nsfw",
		IsPrefix:    true,
		IsPrivate:   true,
		Disable:     true,
		Exec:        nsfwAnime("hentai"),
	})
	commands.Register(&commands.Command{
		Name:        "oppai",
		Description: "Gambar acak oppai (NSFW)",
		Tags:        "nsfw",
		IsPrefix:    true,
		IsPrivate:   true,
		Disable:     true,
		Exec:        nsfwAnime("oppai"),
	})
	commands.Register(&commands.Command{
		Name:        "ero",
		Description: "Gambar acak ero/ecchi (NSFW)",
		As:          []string{"ecchi"},
		Tags:        "nsfw",
		IsPrefix:    true,
		IsPrivate:   true,
		Disable:     true,
		Exec:        nsfwAnime("ero"),
	})
	commands.Register(&commands.Command{
		Name:        "maid",
		Description: "Gambar acak maid (NSFW)",
		Tags:        "nsfw",
		IsPrefix:    true,
		IsPrivate:   true,
		Disable:     true,
		Exec:        nsfwAnime("maid"),
	})
}

func nsfwAnime(tag string) func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	return func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
		if cfg.BASEApiURL == "" {
			m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
			return
		}

		ap := api.Shared(cfg.BASEApiURL, 90*time.Second)

		img, err := ap.WaifuIm(ctx, tag, true)
		if err != nil {
			m.Reply(ctx, "Gagal mengambil gambar: "+err.Error())
			return
		}

		buff, err := client.FetchBytes(img.URL)
		if err != nil {
			m.Reply(ctx, "Gagal mengunduh gambar: "+err.Error())
			return
		}

		caption := fmt.Sprintf("Source: %s", img.Source)
		_, err = client.SendImage(ctx, m.From, buff, caption, m.ID)
		if err != nil {
			m.Reply(ctx, "Gagal mengirim gambar: "+err.Error())
		}
	}
}
