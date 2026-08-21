package handlers

import (
	"context"
	"fmt"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
)

func init() {
	commands.Register(&commands.Command{
		Name:      "menu",
		Tags:      "main",
		IsPrefix:  true,
		SkipQuota: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			text := fmt.Sprintf("Hello %s, Berikut List Command Yang Tersedia\n\n", m.PushName)
			nsfwEnabled := false
			if m.IsGroup {
				nsfwEnabled = store.IsNSFWEnabled(m.From.ToNonAD().String())
			}
			text += commands.BuildMenuText(cfg.Prefix, m.IsGroup, nsfwEnabled)
			_, _ = m.Reply(ctx, text)
		},
	})
}
