package pkg

import (
	"context"
	"fmt"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/message"
)

func init() {
	commands.Register(&commands.Command{
		Name:     "menu",
		Tags:     "main",
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message) {
			pfx := "."
			text := fmt.Sprintf("Hello %s, Berikut List Command Yang Tersedia\n\n", m.PushName)
			text += commands.BuildMenuText(pfx)
			_, _ = m.Reply(ctx, text)
		},
	})
}
