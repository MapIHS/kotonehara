package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
)

// CheckAFK is called on every incoming message to handle AFK auto-replies and clearing AFK status.
func CheckAFK(ctx context.Context, c *clients.Client, m *message.Message) {
	if m == nil || m.Sender.IsEmpty() {
		return
	}

	senderJid := m.Sender.ToNonAD().String()

	// 1. If sender is AFK, remove their AFK status
	if !strings.HasPrefix(strings.TrimSpace(m.Body), ".afk") && !strings.HasPrefix(strings.TrimSpace(m.Body), "afk") {
		if afk, cleared := store.ClearAFK(senderJid); cleared {
			duration := formatDuration(time.Since(afk.Time))
			if m.ID != nil {
				m.ID.MentionedJID = append(m.ID.MentionedJID, senderJid)
			}
			m.Reply(ctx, fmt.Sprintf("👋 Welcome back @%s! Status AFK kamu telah dihapus.\nKamu AFK selama %s.", strings.Split(senderJid, "@")[0], duration))
		}
	}

	// 2. Check if the message mentions any AFK users
	var mentionedAFKs []string
	var taggedJIDs []string
	
	if m.ContextInfo != nil {
		// Check explicitly mentioned JIDs
		for _, jid := range m.ContextInfo.GetMentionedJID() {
			if afk, ok := store.GetAFK(jid); ok {
				duration := formatDuration(time.Since(afk.Time))
				mentionedAFKs = append(mentionedAFKs, fmt.Sprintf("• @%s sedang AFK: %s (sejak %s lalu)", strings.Split(jid, "@")[0], afk.Reason, duration))
				taggedJIDs = append(taggedJIDs, jid)
			}
		}

		// Check quoted message participant
		if m.QuotedMsg != nil {
			quotedJid := m.ContextInfo.GetParticipant()
			if quotedJid != "" {
				if afk, ok := store.GetAFK(quotedJid); ok {
					alreadyMentioned := false
					for _, jid := range m.ContextInfo.GetMentionedJID() {
						if jid == quotedJid {
							alreadyMentioned = true
							break
						}
					}
					if !alreadyMentioned {
						duration := formatDuration(time.Since(afk.Time))
						mentionedAFKs = append(mentionedAFKs, fmt.Sprintf("• @%s sedang AFK: %s (sejak %s lalu)", strings.Split(quotedJid, "@")[0], afk.Reason, duration))
						taggedJIDs = append(taggedJIDs, quotedJid)
					}
				}
			}
		}
	}

	if len(mentionedAFKs) > 0 {
		replyText := "Sstt, orangnya lagi nggak ada!\n\n" + strings.Join(mentionedAFKs, "\n")
		if m.ID != nil {
			m.ID.MentionedJID = taggedJIDs
		}
		m.Reply(ctx, replyText)
	}
}
