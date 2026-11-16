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
		Name:        "menu",
		Description: "Tampilkan daftar command. Pakai .menu --d untuk melihat semua deskripsi",
		Tags:        "main",
		IsPrefix:    true,
		SkipQuota:   true,
		OwnHelpFlag: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			withDescription := commands.IsHelpFlag(m.Query)
			text := fmt.Sprintf("Hello %s, Berikut List Command Yang Tersedia\n", m.PushName)
			if !withDescription {
				text += fmt.Sprintf("_Ketik %smenu %s untuk melihat deskripsi, atau <command> %s untuk satu command (contoh: %ss %s)_\n", cfg.Prefix, commands.HelpFlag, commands.HelpFlag, cfg.Prefix, commands.HelpFlag)
			}
			text += "\n"
			nsfwEnabled := false
			if m.IsGroup {
				nsfwEnabled = store.IsNSFWEnabled(m.From.ToNonAD().String())
			}
			text += commands.BuildMenuText(cfg.Prefix, m.IsGroup, nsfwEnabled, withDescription)
			_, _ = m.Reply(ctx, text)
		},
	})
}
