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

	senderRaw := m.Sender.ToNonAD().String()

	// 1. If sender is AFK, remove their AFK status
	if !strings.HasPrefix(strings.TrimSpace(m.Body), ".afk") && !strings.HasPrefix(strings.TrimSpace(m.Body), "afk") {
		if afk, cleared := findAndClearAFK(ctx, c, m.Sender); cleared {
			duration := formatDuration(time.Since(afk.Time))
			if m.ID != nil {
				m.ID.MentionedJID = []string{senderRaw}
			}
			m.Reply(ctx, fmt.Sprintf("👋 Welcome back @%s! Status AFK kamu telah dihapus.\nKamu AFK selama %s.", lidUser(senderRaw), duration))
		}
	}

	// 2. Check if the message mentions any AFK users
	var mentionedAFKs []string
	var taggedJIDs []string

	if m.ContextInfo != nil {
		for _, rawJid := range m.ContextInfo.GetMentionedJID() {
			if afk, ok := findAFK(ctx, c, rawJid); ok {
				duration := formatDuration(time.Since(afk.Time))
				mentionedAFKs = append(mentionedAFKs, fmt.Sprintf("• @%s sedang AFK: %s (sejak %s lalu)", lidUser(rawJid), afk.Reason, duration))
				taggedJIDs = append(taggedJIDs, rawJid)
			}
		}

		if m.QuotedMsg != nil {
			quotedRaw := m.ContextInfo.GetParticipant()
			if quotedRaw != "" {
				if afk, ok := findAFK(ctx, c, quotedRaw); ok {
					alreadyMentioned := false
					for _, jid := range taggedJIDs {
						if jid == quotedRaw {
							alreadyMentioned = true
							break
						}
					}
					if !alreadyMentioned {
						duration := formatDuration(time.Since(afk.Time))
						mentionedAFKs = append(mentionedAFKs, fmt.Sprintf("• @%s sedang AFK: %s (sejak %s lalu)", lidUser(quotedRaw), afk.Reason, duration))
						taggedJIDs = append(taggedJIDs, quotedRaw)
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

func findAndClearAFK(ctx context.Context, c *clients.Client, sender types.JID) (store.AFKState, bool) {
	rawJID := sender.ToNonAD().String()
	if afk, cleared := store.ClearAFK(rawJID); cleared {
		// Also clean phone JID if it exists
		phoneJID := c.SenderPhone(ctx, sender)
		if phoneJID != "" && phoneJID != rawJID {
			store.ClearAFK(phoneJID)
		}
		return afk, true
	}

	phoneJID := c.SenderPhone(ctx, sender)
	if phoneJID != "" && phoneJID != rawJID {
		if afk, cleared := store.ClearAFK(phoneJID); cleared {
			return afk, true
		}
	}
	return store.AFKState{}, false
}

func findAFK(ctx context.Context, c *clients.Client, jidStr string) (store.AFKState, bool) {
	if afk, ok := store.GetAFK(jidStr); ok {
		return afk, true
	}
	parsed := parseJID(jidStr)
	if !parsed.IsEmpty() {
		phoneJID := c.SenderPhone(ctx, parsed)
		if phoneJID != "" && phoneJID != jidStr {
			if afk, ok := store.GetAFK(phoneJID); ok {
				return afk, true
			}
		}
	}
	return store.AFKState{}, false
}

// lidUser extracts the user ID part from a JID string for use in @mention text.
// "170743069466624@lid" -> "170743069466624"
// "628xxx@s.whatsapp.net" -> "628xxx"
// In WhatsApp, the @text string MUST match the user part of the JID in MentionedJID
// so the WhatsApp mobile client renders it as a blue clickable mention tag.
func lidUser(jid string) string {
	return strings.Split(jid, "@")[0]
}

// parseJID parses a raw JID string into types.JID.
func parseJID(raw string) types.JID {
	jid, _ := types.ParseJID(raw)
	return jid
}
