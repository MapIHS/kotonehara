package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "ocr",
		As:          []string{"baca", "teks"},
		Tags:        "tools",
		Description: "Membaca teks dari gambar (Optical Character Recognition)",
		IsPrefix:    true,
		IsMedia:     true,
		ShowWait:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
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

			if _, err := exec.LookPath("tesseract"); err != nil {
				m.Reply(ctx, "Fitur OCR belum tersedia di server (tesseract tidak ditemukan).")
				return
			}

			// tesseract reads from stdin when input is "-" or "stdin" (varies by version, "stdin" works in v4+)
			cmd := exec.CommandContext(opCtx, "tesseract", "stdin", "stdout", "-l", "ind+eng")
			cmd.Stdin = bytes.NewReader(raw)
			
			var out bytes.Buffer
			cmd.Stdout = &out

			if err := cmd.Run(); err != nil {
				m.Reply(ctx, fmt.Sprintf("Gagal mengekstrak teks dari gambar: %s", err))
				return
			}

			result := strings.TrimSpace(out.String())
			if result == "" {
				m.Reply(ctx, "Tidak ada teks yang terdeteksi di gambar tersebut.")
				return
			}

			m.Reply(ctx, result)
		},
	})
}
