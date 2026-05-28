package pkg

import (
	"context"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

func init() {
	commands.Register(&commands.Command{
		Name:     "delete",
		As:       []string{"delete", "del", "d"},
		Tags:     "owner",
		IsPrefix: true,
		IsOwner:  true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			if m.IsBot {
				return
			}

			ctxInfo := m.ContextInfo
			if ctxInfo == nil || ctxInfo.GetStanzaID() == "" {
				_, _ = m.Reply(ctx, "Balas pesan bot yang mau dihapus, yaa.")
				return
			}

			if ctxInfo.GetParticipant() != client.BotJID() {
				_, _ = m.Reply(ctx, "Balas pesan bot yang mau dihapus, yaa.")
				return
			}

			_, err := client.DeleteMessage(ctx, m.From, ctxInfo.GetStanzaID())
			if err != nil {
				_, _ = m.Reply(ctx, "Gagal menghapus pesan bot.")
			}
		},
	})
}
