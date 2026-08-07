package handlers

import (
	"context"
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
		Name:        "ocr",
		As:          []string{"baca", "teks"},
		Tags:        "tools",
		Description: "Membaca teks dari gambar menggunakan Hararest API",
		IsPrefix:    true,
		IsMedia:     true,
		ShowWait:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if cfg.BASEApiURL == "" {
				m.Reply(ctx, "Fitur OCR belum dikonfigurasi (BASEAPI_URL kosong).")
				return
			}

			if m.Media == nil || m.IsVideo || m.IsQuotedVideo || m.IsGif || m.IsQuotedGif {
				m.Reply(ctx, "Kirim atau balas gambar (bukan video/dokumen) dengan perintah ini untuk mengekstrak teksnya.")
				return
			}

			opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			raw, err := client.WA.Download(opCtx, m.Media)
			if err != nil || len(raw) == 0 {
				m.Reply(ctx, "Gambarnya belum bisa diunduh.")
				return
			}

			apiClient := api.Shared(cfg.BASEApiURL, 60*time.Second)
			text, err := apiClient.ExtractOCR(opCtx, raw)
			if err != nil {
				m.Reply(ctx, "Gagal menghubungi OCR API: "+err.Error())
				return
			}

			text = strings.TrimSpace(text)
			if text == "" {
				m.Reply(ctx, "Tidak ada teks yang terdeteksi di gambar tersebut.")
				return
			}

			m.Reply(ctx, text)
		},
	})
}
