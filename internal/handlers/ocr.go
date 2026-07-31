package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/httpclient"
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

			ocrURL := strings.TrimRight(cfg.BASEApiURL, "/") + "/api/ocr"

			req, err := http.NewRequestWithContext(opCtx, http.MethodPost, ocrURL, bytes.NewReader(raw))
			if err != nil {
				m.Reply(ctx, "Gagal membuat request ke OCR API.")
				return
			}

			req.Header.Set("Content-Type", "image/jpeg")

			hc := httpclient.New("", 60*time.Second)
			resp, err := hc.HTTP.Do(req)
			if err != nil {
				m.Reply(ctx, "Gagal menghubungi OCR API.")
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				m.Reply(ctx, fmt.Sprintf("OCR API error dengan status: %d", resp.StatusCode))
				return
			}

			var result struct {
				Success bool `json:"success"`
				Data    struct {
					Text string `json:"text"`
				} `json:"data"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				m.Reply(ctx, "Gagal membaca respons dari OCR API.")
				return
			}

			text := strings.TrimSpace(result.Data.Text)
			if text == "" {
				m.Reply(ctx, "Tidak ada teks yang terdeteksi di gambar tersebut.")
				return
			}

			m.Reply(ctx, text)
		},
	})
}
