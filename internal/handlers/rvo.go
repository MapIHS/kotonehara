package handlers

import (
	"context"
	"log"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

func init() {
	commands.Register(&commands.Command{
		Name:        "rvo",
		As:          []string{"readviewonce"},
		Tags:        "tools",
		Description: "Balas media sekali lihat untuk mengirim ulang sebagai media biasa",
		IsPrefix:    true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, _ config.Config) {
			if m.ContextInfo.GetStanzaID() == "" || !m.IsQuotedViewOnce {
				_, _ = m.Reply(ctx, "Balas media sekali lihat dengan .rvo, yaa.")
				return
			}
			if _, err := client.SendCachedViewOnceAsNormal(ctx, m.From, m.From, m.ContextInfo.GetStanzaID(), m.ID); err != nil {
				log.Printf("rvo failed: quote_is_view_once=%t cache_error=%T", m.IsQuotedViewOnce, err)
				_, _ = m.Reply(ctx, "Media belum bisa diambil, yaa.")
			}
		},
	})
}
