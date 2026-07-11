package commands

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

func CommandExec(ctx context.Context, c *clients.Client, m *message.Message, cfg config.Config) {
	if ctx == nil || ctx.Err() != nil {
		return
	}

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
		if shouldSendCooldownSticker(ck) {
			if data, err := loadSpamSticker(); err == nil && len(data) > 0 {
				_, _ = c.SendSticker(ctx, m.From, data, false, false, m.ID)
			}
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
		if data, err := loadOwnerSticker(); err == nil && len(data) > 0 {
			_, _ = c.SendSticker(ctx, m.From, data, false, false, m.ID)
		}
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
	if m.IsGroup && (cmd.IsAdmin || cmd.IsBotAdmin) {
		fillAdminStatus(ctx, c, m)
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

	runWithSpinner(ctx, userName, cmd.Name, func() {
		cmd.Exec(ctx, c, m, cfg)
	})
}

func runWithSpinner(ctx context.Context, userName, commandName string, run func()) {
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		i := 0
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		var ticker *time.Ticker
		var tick <-chan time.Time
		defer func() {
			if ticker != nil {
				ticker.Stop()
			}
		}()

		for {
			select {
			case <-done:
				fmt.Printf("\r\033[K\033[1;32m[✓]\033[0m \033[1;36m%s\033[0m use command \033[1;32m%s\033[0m\n", userName, commandName)
				return
			case <-ctx.Done():
				return
			case <-timer.C:
				fmt.Printf("\r\033[K\033[1;35m[%s]\033[0m \033[1;36m%s\033[0m use command \033[1;32m%s\033[0m", spinner[i%len(spinner)], userName, commandName)
				i++
				ticker = time.NewTicker(250 * time.Millisecond)
				tick = ticker.C
			case <-tick:
				fmt.Printf("\r\033[K\033[1;35m[%s]\033[0m \033[1;36m%s\033[0m use command \033[1;32m%s\033[0m", spinner[i%len(spinner)], userName, commandName)
				i++
			}
		}
	}()

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() { close(done) })
		<-finished
	}
	defer stop()
	run()
}

func fillAdminStatus(ctx context.Context, c *clients.Client, m *message.Message) {
	admins, err := c.GroupAdmins(ctx, m.From)
	if err != nil || len(admins) == 0 {
		return
	}

	sender := m.Sender.String()
	bot := c.BotJID()
	for _, admin := range admins {
		if admin == sender {
			m.IsAdmin = true
		}
		if admin == bot {
			m.IsBotAdmin = true
		}
		if m.IsAdmin && m.IsBotAdmin {
			return
		}
	}
}
