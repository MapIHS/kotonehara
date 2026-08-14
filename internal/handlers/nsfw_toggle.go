package handlers

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "nsfw",
		Tags:        "group",
		Description: "Toggle on/off fitur NSFW di grup ini (Admin Only)",
		IsPrefix:    true,
		IsGroup:     true,
		IsAdmin:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			jid := m.From.ToNonAD().String()
			newStatus, err := store.ToggleNSFW(jid)
			if err != nil {
				_, _ = m.Reply(ctx, "❌ Gagal mengubah pengaturan NSFW: "+err.Error())
				return
			}

			if newStatus {
				_, _ = m.Reply(ctx, "✅ Fitur NSFW sekarang *AKTIF* di grup ini.\n_Silakan ketik .menu untuk melihat daftar command._")
			} else {
				_, _ = m.Reply(ctx, "✅ Fitur NSFW sekarang *NONAKTIF* di grup ini.")
			}
		},
	})
}
