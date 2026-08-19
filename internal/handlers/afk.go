package handlers

import (
	"context"
	"regexp"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
)

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
			}

			rawJid := m.Sender.ToNonAD().String()
			store.SetAFK(rawJid, reason)

			phoneJid := client.SenderPhone(ctx, m.Sender)
			if phoneJid != "" && phoneJid != rawJid {
				store.SetAFK(phoneJid, reason)
			}

			// Forward mentions from the original message so @tags in the reason render properly.
			if m.ID != nil && m.ContextInfo != nil {
				m.ID.MentionedJID = collectMentions(reason, m.ContextInfo.GetMentionedJID())
			}

			m.Reply(ctx, "💤 Kamu sekarang AFK.\n\nAlasan: "+reason+"\n\nStatus AFK akan hilang otomatis kalau kamu mengirim pesan lagi.")
		},
	})
}

// mentionRe matches @<digits> patterns in message text (LID or phone mentions).
var mentionRe = regexp.MustCompile(`@(\d+)`)

// collectMentions returns the subset of candidateJIDs whose user part appears
// as @<user> in the text.  This ensures MentionedJID only contains JIDs that
// actually have a corresponding @tag in the text body.
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
