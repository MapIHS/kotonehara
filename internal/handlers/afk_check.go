package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow/types"
)

// CheckAFK is called on every incoming message to handle AFK auto-replies and clearing AFK status.
func CheckAFK(ctx context.Context, c *clients.Client, m *message.Message) {
	if m == nil || m.Sender.IsEmpty() {
		return
	}

	// Resolve sender to phone-number JID (handles LID → PN mapping).
	senderJid := c.SenderPhone(ctx, m.Sender)

	// 1. If sender is AFK, remove their AFK status
	if !strings.HasPrefix(strings.TrimSpace(m.Body), ".afk") && !strings.HasPrefix(strings.TrimSpace(m.Body), "afk") {
		if afk, cleared := store.ClearAFK(senderJid); cleared {
			duration := formatDuration(time.Since(afk.Time))
			phone := jidToPhone(senderJid)
			if m.ID != nil {
				m.ID.MentionedJID = append(m.ID.MentionedJID, senderJid)
			}
			m.Reply(ctx, fmt.Sprintf("👋 Welcome back @%s! Status AFK kamu telah dihapus.\nKamu AFK selama %s.", phone, duration))
		}
	}

	// 2. Check if the message mentions any AFK users
	var mentionedAFKs []string
	var taggedJIDs []string

	if m.ContextInfo != nil {
		// Check explicitly mentioned JIDs
		for _, jid := range m.ContextInfo.GetMentionedJID() {
			// Resolve mentioned JID (might be LID) to phone-number JID
			resolvedJid := c.SenderPhone(ctx, jidFromString(jid))
			if afk, ok := store.GetAFK(resolvedJid); ok {
				duration := formatDuration(time.Since(afk.Time))
				phone := jidToPhone(resolvedJid)
				mentionedAFKs = append(mentionedAFKs, fmt.Sprintf("• @%s sedang AFK: %s (sejak %s lalu)", phone, afk.Reason, duration))
				taggedJIDs = append(taggedJIDs, resolvedJid)
			}
		}

		// Check quoted message participant
		if m.QuotedMsg != nil {
			quotedRaw := m.ContextInfo.GetParticipant()
			if quotedRaw != "" {
				resolvedQuoted := c.SenderPhone(ctx, jidFromString(quotedRaw))
				if afk, ok := store.GetAFK(resolvedQuoted); ok {
					alreadyMentioned := false
					for _, jid := range taggedJIDs {
						if jid == resolvedQuoted {
							alreadyMentioned = true
							break
						}
					}
					if !alreadyMentioned {
						duration := formatDuration(time.Since(afk.Time))
						phone := jidToPhone(resolvedQuoted)
						mentionedAFKs = append(mentionedAFKs, fmt.Sprintf("• @%s sedang AFK: %s (sejak %s lalu)", phone, afk.Reason, duration))
						taggedJIDs = append(taggedJIDs, resolvedQuoted)
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

// jidToPhone extracts the phone number part from a JID string.
// "628xxx@s.whatsapp.net" → "628xxx"
func jidToPhone(jid string) string {
	return strings.Split(jid, "@")[0]
}

// jidFromString parses a raw JID string into types.JID for resolution.
func jidFromString(raw string) types.JID {
	jid, _ := types.ParseJID(raw)
	return jid
}
