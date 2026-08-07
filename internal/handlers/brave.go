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
		Name:        "brave",
		As:          []string{"bsearch"},
		Tags:        "search",
		Description: "Melakukan pencarian di web menggunakan Brave Search",
		IsPrefix:    true,
		ShowWait:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if cfg.BASEApiURL == "" {
				m.Reply(ctx, "Fitur ini belum dikonfigurasi (BASEAPI_URL kosong).")
				return
			}

			if m.Query == "" {
				m.Reply(ctx, "Masukkan kata kunci yang ingin dicari di Brave Search.")
				return
			}

			opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			apiClient := api.Shared(cfg.BASEApiURL, 30*time.Second)
			res, err := apiClient.SearchBrave(opCtx, m.Query)
			if err != nil {
				m.Reply(ctx, "Gagal menghubungi Brave Search API: "+err.Error())
				return
			}

			if len(res.Results) == 0 {
				m.Reply(ctx, "Tidak menemukan hasil untuk kata kunci tersebut.")
				return
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🦁 *Brave Search* : %s\n", m.Query))
			sb.WriteString(fmt.Sprintf("Waktu: %.2fs | Total: %s hasil\n\n", res.SearchTime, res.TotalResults))

			for i, r := range res.Results {
				if i >= 5 { // limit display to 5 results
					break
				}
				sb.WriteString(fmt.Sprintf("📄 *%s*\n", r.Title))
				sb.WriteString(fmt.Sprintf("🔗 %s\n", r.Link))
				if r.Snippet != "" {
					sb.WriteString(fmt.Sprintf("📝 %s\n", r.Snippet))
				}
				sb.WriteString("\n")
			}

			m.Reply(ctx, strings.TrimSpace(sb.String()))
		},
	})
}
