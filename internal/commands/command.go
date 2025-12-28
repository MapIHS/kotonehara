package commands

import (
	"context"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

func CommandExec(ctx context.Context, c *clients.Client, m *message.Message, cfg config.Config) {
	s := strings.TrimSpace(m.Body)
	if s == "" {
		return
	}

	cfgPrefix := strings.TrimSpace(cfg.Prefix)
	msgPrefix := ""

	if cfgPrefix != "" && strings.HasPrefix(s, cfgPrefix) {
		msgPrefix = cfgPrefix
	}

	name := s
	if msgPrefix != "" {
		name = strings.TrimSpace(strings.TrimPrefix(name, msgPrefix))
	}

	parts := strings.Fields(name)
	if len(parts) == 0 {
		return
	}
	key := strings.ToLower(parts[0])

	cmd, ok := lookup(key)
	if !ok {
		return
	}

	ck := m.Sender.String() + "|" + cmd.Name
	if !allowCooldown(ck) {
		_, _ = m.Reply(ctx, "Tolong beri jeda sebentar, yaa.")
		return
	}

	if cmd.IsPrefix && msgPrefix == "" {
		return
	}
	if !cmd.IsPrefix && msgPrefix != "" {
		return
	}

	if cmd.After != nil {
		cmd.After(ctx, c, m)
	}

	if cmd.IsOwner && !m.IsOwner {
		return
	}
	if cmd.IsMedia && m.Media == nil {
		_, _ = m.Reply(ctx, "Media-nya dibutuhkan, yaa.")
		return
	}
	if cmd.IsQuery && m.Query == "" {
		_, _ = m.Reply(ctx, "Kueri-nya dibutuhkan, yaa.")
		return
	}
	if cmd.IsGroup && !m.IsGroup {
		_, _ = m.Reply(ctx, "Hanya untuk grup, yaa.")
		return
	}
	if cmd.IsPrivate && m.IsGroup {
		_, _ = m.Reply(ctx, "Hanya untuk chat pribadi, yaa.")
		return
	}
	if m.IsGroup && cmd.IsAdmin && !m.IsAdmin {
		_, _ = m.Reply(ctx, "Perintah ini untuk admin grup, yaa.")
		return
	}
	if m.IsGroup && cmd.IsBotAdmin && !m.IsBotAdmin {
		_, _ = m.Reply(ctx, "Tolong jadikan bot sebagai admin dulu, yaa.")
		return
	}
	if cmd.ShowWait {
		_, _ = m.Reply(ctx, "Tunggu sebentar, yaa.")
	}

	cmd.Exec(ctx, c, m, cfg)
}
