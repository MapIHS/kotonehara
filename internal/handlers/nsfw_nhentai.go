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

func nhentaiExt(t string) string {
	switch t {
	case "j":
		return "jpg"
	case "p":
		return "png"
	case "g":
		return "gif"
	default:
		return "jpg"
	}
}

func nhentaiGallery(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	code := strings.Fields(m.Query)[0]

	ap := api.Shared("https://nhentai.net", 15*time.Second)
	gallery, err := ap.NhentaiGallery(ctx, code)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengambil gallery: "+err.Error())
		return
	}

	ext := nhentaiExt(gallery.Images.Cover.T)
	coverURL := api.NhentaiCoverURL(gallery.MediaID, ext)

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
	ap := api.Shared("https://nhentai.net", 15*time.Second)
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
		txt += fmt.Sprintf("%d. %s | 📄 %v hal | Code: %v\n", i+1, res.Title.Pretty, res.NumPages, res.ID)
	}

	m.Reply(ctx, txt)
}
