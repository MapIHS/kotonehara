package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/quota"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "quota",
		As:          []string{"kuota", "limit"},
		Tags:        "main",
		Description: "Cek sisa kuota harian",
		IsPrefix:    true,
		SkipQuota:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			qc := quota.Global()
			if qc == nil {
				m.Reply(ctx, "Sistem kuota belum aktif.")
				return
			}

			info, err := qc.GetUsageInfo(ctx, m.Sender.String())
			if err != nil {
				m.Reply(ctx, "Gagal mengambil info kuota.")
				return
			}

			if info.IsPremium {
				m.Reply(ctx, "⭐ *Status Kuota Kamu*\n─────────────────\n🏷️ Tier: *Premium* ✨\n📈 Kuota: *Unlimited*\n\nTerima kasih atas dukunganmu! 🙏")
				return
			}

			used := info.UsedCount
			max := info.MaxLimit
			remaining := max - used
			if remaining < 0 {
				remaining = 0
			}

			// Build progress bar
			barLen := 10
			filled := 0
			if max > 0 {
				filled = (used * barLen) / max
				if filled > barLen {
					filled = barLen
				}
			}
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)
			pct := 0
			if max > 0 {
				pct = (used * 100) / max
				if pct > 100 {
					pct = 100
				}
			}

			warning := ""
			if remaining == 1 {
				warning = "\n\n⚠️ *Sisa 1x lagi!*"
			} else if remaining == 0 {
				warning = "\n\n🚫 *Kuota habis!*"
			}

			txt := fmt.Sprintf(
				"📊 *Status Kuota Kamu*\n"+
					"─────────────────\n"+
					"🏷️ Tier: *Free*\n"+
					"📈 Terpakai: *%d / %d*\n"+
					"[%s] %d%%\n"+
					"⏳ Reset: Besok 00:00 WIB%s\n\n"+
					"⭐ Upgrade ke *Premium* untuk akses unlimited!\n"+
					"Ketik *.donasi* untuk info lebih lanjut.",
				used, max, bar, pct, warning,
			)

			m.Reply(ctx, txt)
		},
	})
}
