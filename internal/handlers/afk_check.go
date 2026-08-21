package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/infra/store"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow/types"
)

var afkReplyCooldown = newAFKReplyLimiter(90 * time.Second)

func CheckAFK(ctx context.Context, c *clients.Client, m *message.Message, cfg config.Config) {
	if m == nil || m.Sender.IsEmpty() || m.IsBot {
		return
	}

	senderRaw := m.Sender.ToNonAD().String()
	if isAFKActivity(m) && !isAFKCommand(m.Body, cfg.Prefix) {
		if afk, cleared := findAndClearAFK(ctx, c, m.Sender); cleared {
			duration := formatDuration(time.Since(afk.Time))
			if m.ID != nil {
				m.ID.MentionedJID = dedup(append([]string{senderRaw}, extractMentionJIDs(afk.Reason)...))
			}
			_, _ = m.Reply(ctx, fmt.Sprintf("👋 Welcome back @%s! Status AFK kamu telah dihapus.\nKamu AFK selama %s.", jidUser(senderRaw), duration))
		}
	}

	if !m.IsGroup {
		return
	}

	mentionedAFKs, taggedJIDs := collectMentionedAFKs(ctx, c, m)
	if len(mentionedAFKs) == 0 {
		return
	}

	cooldownKey := m.From.String() + "|" + strings.Join(taggedJIDs, ",")
	if !afkReplyCooldown.Allow(cooldownKey) {
		return
	}

	if m.ID != nil {
		m.ID.MentionedJID = dedup(taggedJIDs)
	}
	_, _ = m.Reply(ctx, "Sstt, orangnya lagi nggak ada!\n\n"+strings.Join(mentionedAFKs, "\n"))
}

// isAFKActivity excludes protocol/placeholder events. Only a real user message
// with text or media should mark an AFK user as active again.
func isAFKActivity(m *message.Message) bool {
	return strings.TrimSpace(m.Body) != "" || m.Media != nil || m.IsQuotedSticker
}

func collectMentionedAFKs(ctx context.Context, c *clients.Client, m *message.Message) ([]string, []string) {
	var mentionedAFKs []string
	var taggedJIDs []string

	if m.ContextInfo == nil {
		return mentionedAFKs, taggedJIDs
	}

	for _, rawJid := range m.ContextInfo.GetMentionedJID() {
		line, jids, ok := afkMentionLine(ctx, c, rawJid)
		if ok {
			mentionedAFKs = append(mentionedAFKs, line)
			taggedJIDs = append(taggedJIDs, jids...)
		}
	}

	quotedRaw := m.ContextInfo.GetParticipant()
	if m.QuotedMsg != nil && quotedRaw != "" && !containsJID(taggedJIDs, quotedRaw) {
		line, jids, ok := afkMentionLine(ctx, c, quotedRaw)
		if ok {
			mentionedAFKs = append(mentionedAFKs, line)
			taggedJIDs = append(taggedJIDs, jids...)
		}
	}

	return mentionedAFKs, dedup(taggedJIDs)
}

func afkMentionLine(ctx context.Context, c *clients.Client, rawJid string) (string, []string, bool) {
	afk, ok := findAFK(ctx, c, rawJid)
	if !ok {
		return "", nil, false
	}
	duration := formatDuration(time.Since(afk.Time))
	jids := append([]string{rawJid}, extractMentionJIDs(afk.Reason)...)
	return fmt.Sprintf("• @%s sedang AFK: %s (sejak %s lalu)", jidUser(rawJid), afk.Reason, duration), jids, true
}

func extractMentionJIDs(text string) []string {
	matches := mentionRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	var jids []string
	for _, match := range matches {
		user := match[1]
		if len(user) >= 15 {
			jids = append(jids, user+"@lid")
		} else {
			jids = append(jids, user+"@s.whatsapp.net")
		}
	}
	return dedup(jids)
}

func dedup(s []string) []string {
	seen := make(map[string]bool, len(s))
	var result []string
	for _, v := range s {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	return result
}

func findAndClearAFK(ctx context.Context, c *clients.Client, sender types.JID) (store.AFKState, bool) {
	jids := afkJIDs(ctx, c, sender.String())
	afk, cleared, err := store.ClearAFK(ctx, jids...)
	if err != nil {
		log.Printf("clear AFK: %v", err)
		return store.AFKState{}, false
	}
	return afk, cleared
}

func findAFK(ctx context.Context, c *clients.Client, jidStr string) (store.AFKState, bool) {
	afk, ok, err := store.GetAFK(ctx, afkJIDs(ctx, c, jidStr)...)
	if err != nil {
		log.Printf("get AFK: %v", err)
		return store.AFKState{}, false
	}
	return afk, ok
}

func afkJIDs(ctx context.Context, c *clients.Client, jidStr string) []string {
	parsed := parseJID(jidStr)
	jids := []string{jidStr}
	if !parsed.IsEmpty() {
		nonAD := parsed.ToNonAD().String()
		jids = append(jids, nonAD)
		if phoneJID := c.SenderPhone(ctx, parsed); phoneJID != "" {
			jids = append(jids, phoneJID)
		}
	}
	return dedup(jids)
}

func isAFKCommand(body string, prefix string) bool {
	body = strings.TrimSpace(body)
	prefix = strings.TrimSpace(prefix)
	if body == "" || prefix == "" || !strings.HasPrefix(body, prefix) {
		return false
	}
	body = strings.TrimSpace(strings.TrimPrefix(body, prefix))
	fields := strings.Fields(body)
	return len(fields) > 0 && strings.EqualFold(fields[0], "afk")
}

func jidUser(jid string) string {
	return strings.Split(jid, "@")[0]
}

func parseJID(raw string) types.JID {
	jid, _ := types.ParseJID(raw)
	return jid
}

func containsJID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type afkReplyLimiter struct {
	mu       sync.Mutex
	ttl      time.Duration
	lastSent map[string]time.Time
}

func newAFKReplyLimiter(ttl time.Duration) *afkReplyLimiter {
	return &afkReplyLimiter{ttl: ttl, lastSent: map[string]time.Time{}}
}

func (l *afkReplyLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if last, ok := l.lastSent[key]; ok && now.Sub(last) < l.ttl {
		return false
	}
	l.lastSent[key] = now
	if len(l.lastSent) > 1024 {
		for k, t := range l.lastSent {
			if now.Sub(t) >= l.ttl {
				delete(l.lastSent, k)
			}
		}
	}
	return true
}
