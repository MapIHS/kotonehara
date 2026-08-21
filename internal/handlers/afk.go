package handlers

import (
	"context"
	"log"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
)

const maxAFKReasonRunes = 300

func init() {
	commands.Register(&commands.Command{
		Name:        "afk",
		As:          []string{},
		Tags:        "tools",
		Description: "Set status AFK (Away From Keyboard)",
		IsPrefix:    true,
		SkipQuota:   true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			reason := strings.TrimSpace(m.Query)
			if reason == "" {
				reason = "Tanpa alasan"
			} else if utf8.RuneCountInString(reason) > maxAFKReasonRunes {
				_, _ = m.Reply(ctx, "Alasan AFK terlalu panjang. Maksimal 300 karakter.")
				return
			}

			for _, jid := range afkJIDs(ctx, client, m.Sender.String()) {
				if err := store.SetAFK(ctx, jid, reason); err != nil {
					log.Printf("set AFK: %v", err)
					_, _ = m.Reply(ctx, "Status AFK belum bisa disimpan.")
					return
				}
			}

			if m.ID != nil && m.ContextInfo != nil {
				m.ID.MentionedJID = collectMentions(reason, m.ContextInfo.GetMentionedJID())
			}

			_, _ = m.Reply(ctx, "💤 Kamu sekarang AFK.\n\nAlasan: "+reason+"\n\nStatus AFK akan hilang otomatis kalau kamu mengirim pesan lagi.")
		},
	})
}

var mentionRe = regexp.MustCompile(`@(\d+)`)

func collectMentions(text string, candidateJIDs []string) []string {
	matches := mentionRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	mentioned := map[string]bool{}
	for _, m := range matches {
		mentioned[m[1]] = true
	}

	var result []string
	for _, jid := range candidateJIDs {
		user := strings.Split(jid, "@")[0]
		if mentioned[user] {
			result = append(result, jid)
		}
	}
	return result
}
