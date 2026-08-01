package handlers

import (
	"context"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "afk",
		As:          []string{},
		Tags:        "tools",
		Description: "Set status AFK (Away From Keyboard)",
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			reason := strings.TrimSpace(m.Query)
			if reason == "" {
				reason = "Tanpa alasan"
			}
			senderJid := m.Sender.ToNonAD().String()
			store.SetAFK(senderJid, reason)
			m.Reply(ctx, "💤 Kamu sekarang AFK.\n\nAlasan: "+reason+"\n\nStatus AFK akan hilang otomatis kalau kamu mengirim pesan lagi.")
		},
	})
}
