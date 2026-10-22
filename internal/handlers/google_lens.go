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

func formatGoogleLensResult(result *api.GoogleLensResult, mode string) string {
	if mode == "ai" {
		answer := strings.TrimSpace(result.AIResponse)
		if answer == "" {
			return "Google Lens tidak menemukan ringkasan AI untuk gambar ini."
		}
		if len([]rune(answer)) > 3500 {
			answer = string([]rune(answer)[:3500]) + "…"
		}
		return "🔎 *Google Lens AI*\n\n" + answer
	}
	if len(result.VisualMatches) == 0 {
		return "Google Lens belum menemukan hasil visual untuk gambar ini."
	}
	var b strings.Builder
	b.WriteString("🔎 *Hasil Google Lens*\n")
	for i, match := range result.VisualMatches {
		if i == 5 {
			break
		}
		title := strings.TrimSpace(match.Title)
		if title == "" {
			title = "Buka sumber hasil"
		}
		url := strings.TrimSpace(match.URL)
		if url == "" {
			continue
		}
		fmt.Fprintf(&b, "\n%d. %s\n%s\n", i+1, title, url)
	}
	if b.String() == "🔎 *Hasil Google Lens*\n" {
		return "Google Lens menemukan hasil, tetapi tidak ada tautan yang bisa ditampilkan."
	}
	if page := strings.TrimSpace(result.LensPage); page != "" {
		fmt.Fprintf(&b, "\nLihat semua: %s", page)
	}
	return strings.TrimSpace(b.String())
}

func handleGoogleLens(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config, mode string) {
	if strings.TrimSpace(cfg.GoogleLensWorkerToken) == "" {
		m.Reply(ctx, "Google Lens belum dikonfigurasi (GOOGLE_LENS_WORKER_TOKEN kosong).")
		return
	}
	if (cfg.CloudflareAccessClientID == "") != (cfg.CloudflareAccessClientSecret == "") {
		m.Reply(ctx, "Konfigurasi Cloudflare Access Google Lens belum lengkap.")
		return
	}
	if m.Media == nil || m.IsVideo || m.IsQuotedVideo || m.IsGif || m.IsQuotedGif || (!m.IsImage && !m.IsQuotedImage) {
		m.Reply(ctx, "Kirim atau balas gambar (JPEG, PNG, atau WebP) dengan perintah ini untuk mencari sumbernya.")
		return
	}
	opCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	raw, err := client.WA.Download(opCtx, m.Media)
	if err != nil || len(raw) == 0 {
		m.Reply(ctx, "Gambarnya belum bisa diunduh.")
		return
	}
	result, err := api.New("", 100*time.Second).GoogleLens(opCtx, cfg.GoogleLensWorkerURL, cfg.GoogleLensWorkerToken,
		cfg.CloudflareAccessClientID, cfg.CloudflareAccessClientSecret, mode, raw)
	if err != nil {
		m.Reply(ctx, "Pencarian Google Lens gagal: "+err.Error())
		return
	}
	m.Reply(ctx, formatGoogleLensResult(result, mode))
}

func init() {
	commands.Register(&commands.Command{
		Name: "lens", As: []string{"googlelens", "lensvisual"}, Tags: "search", Description: "Cari sumber dan gambar serupa dengan Google Lens",
		IsPrefix: true, IsMedia: true, ShowWait: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			handleGoogleLens(ctx, client, m, cfg, "visual")
		},
	})
	commands.Register(&commands.Command{
		Name: "lensai", As: []string{"googlelensai"}, Tags: "search", Description: "Tanyakan gambar ke Google Lens AI",
		IsPrefix: true, IsMedia: true, ShowWait: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			handleGoogleLens(ctx, client, m, cfg, "ai")
		},
	})
}
