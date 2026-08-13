package handlers

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
		Name:        "nhentai",
		As:          []string{"nh"},
		Tags:        "nsfw",
		Description: "Dapatkan info nhentai",
		IsPrefix:    true,
		IsQuery:     true,
		IsPrivate:   true,
		Exec:        nhentaiGallery,
	})
	commands.Register(&commands.Command{
		Name:        "nhsearch",
		As:          []string{"nhsearch"},
		Tags:        "nsfw",
		Description: "Cari komik nhentai",
		IsPrefix:    true,
		IsQuery:     true,
		IsPrivate:   true,
		Exec:        nhentaiSearch,
	})
}



func nhentaiGallery(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if cfg.BASEApiURL == "" {
		m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
		return
	}

	args := strings.Fields(m.Query)
	if len(args) == 0 {
		m.Reply(ctx, "Harap masukkan kode gallery (contoh: .nh 672279)")
		return
	}
	code := args[0]

	ap := api.Shared(cfg.BASEApiURL, 15*time.Second)
	gallery, err := ap.NhentaiGallery(ctx, code)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengambil gallery: "+err.Error())
		return
	}
	if gallery == nil || gallery.ID == 0 {
		m.Reply(ctx, "❌ Gallery tidak ditemukan atau API mengembalikan respon kosong.")
		return
	}

	coverURL := api.NhentaiCoverURL(gallery.Cover.Path)

	var tags, artists []string
	for _, tag := range gallery.Tags {
		if tag.Type == "tag" {
			tags = append(tags, tag.Name)
		} else if tag.Type == "artist" {
			artists = append(artists, tag.Name)
		}
	}

	caption := fmt.Sprintf(`📖 *%v*
🔢 Code: %v
📄 Pages: %v
❤️ Favorites: %v
🏷️ Tags: %s
🎨 Artists: %s`, gallery.Title.Pretty, gallery.ID, gallery.NumPages, gallery.NumFavorites, strings.Join(tags, ", "), strings.Join(artists, ", "))

	buff, err := client.FetchBytes(coverURL)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengunduh cover: "+err.Error())
		return
	}

	_, err = client.SendImage(ctx, m.From, buff, caption, m.ID)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengirim gambar: "+err.Error())
	}
}

func nhentaiSearch(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if cfg.BASEApiURL == "" {
		m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
		return
	}

	ap := api.Shared(cfg.BASEApiURL, 15*time.Second)
	results, err := ap.NhentaiSearch(ctx, m.Query)
	if err != nil {
		m.Reply(ctx, "❌ Gagal melakukan pencarian: "+err.Error())
		return
	}

	if len(results) == 0 {
		m.Reply(ctx, "❌ Tidak ada hasil yang ditemukan.")
		return
	}

	txt := fmt.Sprintf("🔍 Hasil pencarian: %s\n\n", m.Query)
	limit := 5
	if len(results) < limit {
		limit = len(results)
	}

	for i := 0; i < limit; i++ {
		res := results[i]
		txt += fmt.Sprintf("%d. %s | 📄 %v hal | Code: %v\n", i+1, res.EnglishTitle, res.NumPages, res.ID)
	}

	m.Reply(ctx, txt)
}
