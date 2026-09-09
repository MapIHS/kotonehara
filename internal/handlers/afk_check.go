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
		if afk, cleared := findAndClearAFK(ctx, c, m.Sender, m.SenderAlt); cleared {
			duration := formatDuration(time.Since(afk.Time))
			if m.ID != nil {
				m.ID.MentionedJID = []string{senderRaw}
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
	seenIdentities := make(map[string]struct{})

	if m.ContextInfo == nil {
		return mentionedAFKs, taggedJIDs
	}

	for _, rawJid := range m.ContextInfo.GetMentionedJID() {
		line, jids, identityKey, ok := afkMentionLine(ctx, c, rawJid)
		if ok {
			if _, seen := seenIdentities[identityKey]; seen {
				continue
			}
			seenIdentities[identityKey] = struct{}{}
			mentionedAFKs = append(mentionedAFKs, line)
			taggedJIDs = append(taggedJIDs, jids...)
		}
	}

	quotedRaw := m.ContextInfo.GetParticipant()
	if m.QuotedMsg != nil && quotedRaw != "" && !containsJID(taggedJIDs, quotedRaw) {
		line, jids, identityKey, ok := afkMentionLine(ctx, c, quotedRaw)
		if ok {
			if _, seen := seenIdentities[identityKey]; seen {
				return mentionedAFKs, dedup(taggedJIDs)
			}
			mentionedAFKs = append(mentionedAFKs, line)
			taggedJIDs = append(taggedJIDs, jids...)
		}
	}

	return mentionedAFKs, dedup(taggedJIDs)
}

func afkMentionLine(ctx context.Context, c *clients.Client, rawJid string) (string, []string, string, bool) {
	parsed, err := types.ParseJID(rawJid)
	if err != nil || parsed.IsEmpty() {
		return "", nil, "", false
	}
	id, err := c.ResolveIdentity(ctx, parsed, types.EmptyJID)
	if err != nil {
		log.Printf("resolve mentioned AFK identity %s: %v", rawJid, err)
	}
	afk, ok := findAFK(ctx, c, rawJid)
	if !ok {
		return "", nil, "", false
	}
	duration := formatDuration(time.Since(afk.Time))
	identityKey := id.Key
	if identityKey == "" {
		identityKey = parsed.ToNonAD().String()
	}
	return fmt.Sprintf("• @%s sedang AFK: %s (sejak %s lalu)", jidUser(rawJid), afk.Reason, duration), []string{rawJid}, identityKey, true
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

func findAndClearAFK(ctx context.Context, c *clients.Client, sender, senderAlt types.JID) (store.AFKState, bool) {
	jids := afkJIDs(ctx, c, sender, senderAlt)
	afk, cleared, err := store.ClearAFK(ctx, jids...)
	if err != nil {
		log.Printf("clear AFK: %v", err)
		return store.AFKState{}, false
	}
	return afk, cleared
}

func findAFK(ctx context.Context, c *clients.Client, jidStr string) (store.AFKState, bool) {
	parsed := parseJID(jidStr)
	afk, ok, err := store.GetAFK(ctx, afkJIDs(ctx, c, parsed, types.EmptyJID)...)
	if err != nil {
		log.Printf("get AFK: %v", err)
		return store.AFKState{}, false
	}
	return afk, ok
}

func afkJIDs(ctx context.Context, c *clients.Client, primary, alternate types.JID) []string {
	id, err := c.ResolveIdentity(ctx, primary, alternate)
	if err != nil {
		log.Printf("resolve AFK identity %s: %v", primary, err)
	}
	jids := id.AliasStrings()
	stateJID := id.StateJID()
	if stateJID != "" {
		jids = append([]string{stateJID}, jids...)
	}
	if raw := primary.String(); raw != "" {
		jids = append(jids, raw)
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
