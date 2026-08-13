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
		Name:        "nsfwgif",
		As:          []string{"hgif"},
		Tags:        "nsfw",
		Description: "Kirim nsfw gif",
		IsPrefix:    true,
		IsQuery:     true,
		IsPrivate:   true,
		Exec:        nsfwGif,
	})
}

func nsfwGif(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	categories := []string{"anal", "blowjob", "cum", "fuck", "solo", "yuri", "yaoi"}
	cat := strings.ToLower(strings.TrimSpace(m.Query))

	valid := false
	for _, c := range categories {
		if c == cat {
			valid = true
			break
		}
	}

	if !valid {
		m.Reply(ctx, fmt.Sprintf("❌ Kategori tidak valid. Kategori yang tersedia: %s", strings.Join(categories, ", ")))
		return
	}

	if cfg.BASEApiURL == "" {
		m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
		return
	}

	ap := api.Shared(cfg.BASEApiURL, 15*time.Second)
	url, err := ap.PurrBot(ctx, cat)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengambil gif: "+err.Error())
		return
	}

	buff, err := client.FetchBytes(url)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengunduh gif: "+err.Error())
		return
	}

	_, err = client.SendVideo(ctx, m.From, buff, true, "", m.ID)
	if err != nil {
		m.Reply(ctx, "❌ Gagal mengirim gif: "+err.Error())
	}
}
