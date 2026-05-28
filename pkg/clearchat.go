package pkg

import (
	"context"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

func parseDeleteMediaOption(q string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(q)) {
	case "", "chat", "nomedia", "no-media":
		return false, true
	case "media", "withmedia", "all", "total":
		return true, true
	default:
		return false, false
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:     "clearchat",
		Tags:     "owner",
		IsPrefix: true,
		IsOwner:  true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.IsBot {
				return
			}

			deleteMedia, ok := parseDeleteMediaOption(m.Query)
			if !ok {
				_, _ = m.Reply(ctx, "Format: clearchat [media]")
				return
			}

			_, _ = m.Reply(ctx, "Sedang membersihkan chat ini, yaa.")
			if err := client.DeleteChat(ctx, m.From, deleteMedia); err != nil {
				_, _ = m.Reply(ctx, "Gagal membersihkan chat: "+err.Error())
			}
		},
	})
}
