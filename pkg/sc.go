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
		Name:     "sc",
		As:       []string{"sourcecode"},
		Tags:     "main",
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			client.SendText(ctx, m.From, "https://github.com/MapIHS/kotonehara\n\nPake ajh, jangan lupa kasih stars", m.ID)
		},
	})
}
