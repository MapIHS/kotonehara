package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

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
		if data, err := loadSpamSticker(); err == nil && len(data) > 0 {
			_, _ = c.SendSticker(ctx, m.From, data, false, false, m.ID)
		}
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

	userName := m.PushName
	if userName == "" {
		userName = m.Sender.User
	}

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		i := 0
		for {
			select {
			case <-done:
				fmt.Printf("\r\033[K\033[1;32m[✓]\033[0m \033[1;36m%s\033[0m use command \033[1;32m%s\033[0m\n", userName, cmd.Name)
				close(finished)
				return
			default:
				fmt.Printf("\r\033[K\033[1;35m[%s]\033[0m \033[1;36m%s\033[0m use command \033[1;32m%s\033[0m", spinner[i%len(spinner)], userName, cmd.Name)
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	cmd.Exec(ctx, c, m, cfg)
	close(done)
	<-finished
}
