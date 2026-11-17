package commands

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
)

// quotaCheck is set from cmd/bot/main.go to avoid circular imports.
// Returns (allowed, blockMessage, error).
var quotaCheck func(ctx context.Context, jid string, aliases []string) (bool, string, error)

// SetQuotaCheck sets the quota check function for command execution.
func SetQuotaCheck(fn func(ctx context.Context, jid string, aliases []string) (bool, string, error)) {
	quotaCheck = fn
}

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

	// A command typed with the wrong prefix style is not an invocation, so it
	// must not start a cooldown or trigger the spam sticker.
	if cmd.IsPrefix && msgPrefix == "" {
		return
	}
	if !cmd.IsPrefix && msgPrefix != "" {
		return
	}

	cooldownKeys := make([]string, 0, 4)
	for _, alias := range m.Identity.AliasStrings() {
		cooldownKeys = append(cooldownKeys, alias+"|"+cmd.Name)
	}
	if len(cooldownKeys) == 0 {
		cooldownKeys = append(cooldownKeys, m.Sender.ToNonAD().String()+"|"+cmd.Name)
	}
	if !allowCooldownAliases(cooldownKeys) {
		if shouldSendCooldownSticker(cooldownKeys[0]) {
			if data, err := loadSpamSticker(); err == nil && len(data) > 0 {
				_, _ = c.SendSticker(ctx, m.From, data, false, false, m.ID)
			}
		}
		return
	}

	// "<command> --d" shows the description instead of running the command.
	// It costs no quota and skips media/query validation, but still respects
	// chat visibility so hidden NSFW commands are not advertised.
	if IsHelpFlag(m.Query) && !cmd.OwnHelpFlag {
		nsfwEnabled := m.IsGroup && cmd.IsPrivate && store.IsNSFWEnabled(m.From.ToNonAD().String())
		if visibleIn(cmd, m.IsGroup, nsfwEnabled) {
			_, _ = m.Reply(ctx, BuildCommandHelp(cmd, cfgPrefix))
			return
		}
	}

	if cmd.IsOwner && !m.IsOwner {
		if data, err := loadOwnerSticker(); err == nil && len(data) > 0 {
			_, _ = c.SendSticker(ctx, m.From, data, false, false, m.ID)
		}
		return
	}
	if cmd.IsMedia && c != nil {
		c.RestoreQuotedMedia(ctx, m)
	}
	if cmd.IsMedia && m.Media == nil {
		_, _ = m.Reply(ctx, "Media-nya dibutuhkan, yaa.")
		return
	}
	if cmd.IsQuery {
		m.Query = quotedQuery(cmd, m)
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
		if strings.Contains(strings.ToLower(cmd.Tags), "nsfw") {
			if !store.IsNSFWEnabled(m.From.ToNonAD().String()) {
				_, _ = m.Reply(ctx, "Fitur NSFW dinonaktifkan di grup ini.\n\n_Admin dapat menyalakannya dengan perintah .nsfw_")
				return
			}
		} else {
			_, _ = m.Reply(ctx, "Hanya untuk chat pribadi, yaa.")
			return
		}
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

	// Quota is charged only after every precondition passes, so a rejected
	// invocation (missing media/query, wrong chat, not admin) costs nothing.
	// Exempt: SkipQuota commands, owner commands, and the bot account itself.
	if !cmd.SkipQuota && !cmd.IsOwner && !m.IsBot && quotaCheck != nil {
		allowed, blockMsg, err := quotaCheck(ctx, m.Identity.StateJID(), m.Identity.AliasStrings())
		if err != nil {
			// The checker already applied its fail-open/fail-closed policy;
			// an error here means the command must not run.
			log.Printf("quota check error: %v", err)
			_, _ = m.Reply(ctx, "Sistem kuota sedang bermasalah. Coba lagi nanti, yaa.")
			return
		}
		if !allowed {
			_, _ = m.Reply(ctx, blockMsg)
			return
		}
	}

	if cmd.After != nil {
		cmd.After(ctx, c, m)
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

	senderAliases := make(map[string]struct{})
	for _, alias := range m.Identity.AliasStrings() {
		senderAliases[alias] = struct{}{}
	}
	botIdentity := c.BotIdentity()
	for _, admin := range admins {
		if _, ok := senderAliases[admin]; ok {
			m.IsAdmin = true
		}
		if botIdentity.MatchesString(admin) {
			m.IsBotAdmin = true
		}
		if m.IsAdmin && m.IsBotAdmin {
			return
		}
	}
}

// qualityToken matches a bare quality option such as "720", "1080p" or "4k".
var qualityToken = regexp.MustCompile(`(?i)^(\d{3,4}p?|\d{1,2}k)$`)

// quotedQuery lets downloaders take the link from the replied-to message, so
// replying ".ig" to a chat containing a link works like ".ig <link>", and
// replying ".ytv 720" works like ".ytv <link> 720". A typed query that is
// anything other than quality options is returned unchanged.
func quotedQuery(cmd *Command, m *message.Message) string {
	if !strings.EqualFold(strings.TrimSpace(cmd.Tags), "downloader") {
		return m.Query
	}
	options := strings.Fields(m.Query)
	for _, option := range options {
		if !qualityToken.MatchString(option) {
			return m.Query
		}
	}
	link := message.FirstURL(m.QuotedText())
	if link == "" {
		return m.Query
	}
	return strings.Join(append([]string{link}, options...), " ")
}
