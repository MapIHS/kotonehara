package commands

import (
	"context"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/message"
)

func CommandExec(ctx context.Context, c *clients.Client, m *message.Message) {
	s := strings.TrimSpace(m.Command)
	if s == "" {
		return
	}

	prefix := ""
	switch s[0] {
	case '.':
		prefix = s[:1]
	}

	name := s
	if prefix != "" {
		name = strings.TrimSpace(strings.TrimPrefix(name, prefix))
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

	// prefix rules
	if cmd.IsPrefix && prefix == "" {
		return
	}
	if !cmd.IsPrefix && prefix != "" {
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

	cmd.Exec(ctx, c, m)
}
