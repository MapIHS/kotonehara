package pkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

const (
	ownerExecTimeout      = 60 * time.Second
	ownerExecMaxReplySize = 65536
	ownerExecSessionTTL   = 6 * time.Hour
)

type ownerExecSession struct {
	cwd       string
	updatedAt time.Time
}

var ownerExecSessions = struct {
	sync.RWMutex
	byKey map[string]ownerExecSession
}{
	byKey: map[string]ownerExecSession{},
}

func ownerExec(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	_ = cfg

	query := strings.TrimSpace(m.Query)
	if query == "" {
		m.Reply(ctx, "Command-nya dibutuhkan, yaa.")
		return
	}

	baseDir := ownerExecBaseDir(m)
	token := ownerExecToken(client)
	pwdMarker := "__KOTONEHARA_EXEC_PWD_" + token + "__="

	execCtx, cancel := context.WithTimeout(ctx, ownerExecTimeout)
	defer cancel()

	out, err := runOwnerExecCommand(execCtx, baseDir, query, pwdMarker)
	reply, shellDir := splitOwnerExecPWD(out, pwdMarker)
	reply = strings.TrimSpace(reply)
	nextDir := ownerExecNextDir(baseDir, query, shellDir)

	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		m.Reply(ctx, "Command timeout, yaa.")
		return
	}
	if err != nil {
		if reply == "" {
			reply = fmt.Sprintf("%v", err)
		} else {
			reply = fmt.Sprintf("%s\n\n%v", reply, err)
		}
	}
	if reply == "" {
		reply = "Selesai tanpa output."
	}

	reply = ownerExecReply(reply, nextDir, token)
	storeOwnerExecSession(token, nextDir)
	if sent, sendErr := m.Reply(ctx, reply); sendErr == nil && sent.ID != "" {
		storeOwnerExecSession(string(sent.ID), nextDir)
	}
}

func runOwnerExecCommand(ctx context.Context, cwd, query, pwdMarker string) ([]byte, error) {
	script := fmt.Sprintf("trap '__kotonehara_status=$?; printf \"\\n%s%%s\\n\" \"$PWD\"; exit $__kotonehara_status' EXIT\n%s", pwdMarker, query)
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Dir = cwd
	return cmd.CombinedOutput()
}

func ownerExecBaseDir(m *message.Message) string {
	if key := ownerExecQuotedMessageID(m); key != "" {
		if cwd, ok := lookupOwnerExecSession(key); ok {
			return cwd
		}
	}
	if key := ownerExecQuotedToken(m); key != "" {
		if cwd, ok := lookupOwnerExecSession(key); ok {
			return cwd
		}
	}
	return defaultOwnerExecDir()
}

func ownerExecQuotedMessageID(m *message.Message) string {
	if m == nil || m.ContextInfo == nil {
		return ""
	}
	return strings.TrimSpace(m.ContextInfo.GetStanzaID())
}

func ownerExecQuotedToken(m *message.Message) string {
	if m == nil || m.QuotedMsg == nil {
		return ""
	}

	text := m.QuotedMsg.GetExtendedTextMessage().GetText()
	if text == "" {
		text = m.QuotedMsg.GetConversation()
	}

	const prefix = "[exec:"
	idx := strings.LastIndex(text, prefix)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(prefix):]
	end := strings.Index(rest, "]")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func ownerExecToken(client *clients.Client) string {
	if client != nil && client.WA != nil {
		return string(client.WA.GenerateMessageID())
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func splitOwnerExecPWD(out []byte, marker string) (string, string) {
	text := string(out)
	idx := strings.LastIndex(text, marker)
	if idx < 0 {
		return text, ""
	}

	before := strings.TrimRight(text[:idx], "\r\n")
	after := text[idx+len(marker):]
	if lineEnd := strings.IndexByte(after, '\n'); lineEnd >= 0 {
		after = after[:lineEnd]
	}
	return before, strings.TrimSpace(after)
}

func ownerExecNextDir(baseDir, query, shellDir string) string {
	nextDir := baseDir
	if dir, ok := cleanOwnerExecDir(shellDir); ok {
		nextDir = dir
	}

	if sameOwnerExecDir(nextDir, baseDir) {
		if dir, ok := inferOwnerExecListDir(baseDir, query); ok {
			nextDir = dir
		}
	}
	return nextDir
}

func inferOwnerExecListDir(baseDir, query string) (string, bool) {
	fields := strings.Fields(query)
	if len(fields) < 2 {
		return "", false
	}

	switch filepath.Base(fields[0]) {
	case "ls", "dir", "ll":
	default:
		return "", false
	}

	candidate := ""
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") {
			continue
		}
		if candidate != "" {
			return "", false
		}
		candidate = field
	}
	if candidate == "" {
		return "", false
	}

	return cleanOwnerExecDir(resolveOwnerExecPath(baseDir, candidate))
}

func resolveOwnerExecPath(baseDir, target string) string {
	if target == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(target, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(target, "~/"))
		}
	}
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(baseDir, target)
}

func cleanOwnerExecDir(dir string) (string, bool) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", false
	}
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", false
		}
		dir = abs
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return filepath.Clean(dir), true
}

func sameOwnerExecDir(a, b string) bool {
	cleanA, okA := cleanOwnerExecDir(a)
	cleanB, okB := cleanOwnerExecDir(b)
	return okA && okB && cleanA == cleanB
}

func defaultOwnerExecDir() string {
	if wd, err := os.Getwd(); err == nil {
		if dir, ok := cleanOwnerExecDir(wd); ok {
			return dir
		}
	}
	return "/"
}

func ownerExecReply(output, cwd, token string) string {
	footer := fmt.Sprintf("\n\n[exec:%s]\nwd: %s", token, cwd)
	limit := ownerExecMaxReplySize - len(footer)
	if limit < 0 {
		return footer
	}
	if len(output) > limit {
		if limit <= 5 {
			return footer
		}
		output = output[:limit-5] + "\n\n..."
	}
	return output + footer
}

func lookupOwnerExecSession(key string) (string, bool) {
	pruneOwnerExecSessions()

	ownerExecSessions.RLock()
	defer ownerExecSessions.RUnlock()

	s, ok := ownerExecSessions.byKey[key]
	if !ok {
		return "", false
	}
	return s.cwd, true
}

func storeOwnerExecSession(key, cwd string) {
	key = strings.TrimSpace(key)
	cwd, ok := cleanOwnerExecDir(cwd)
	if key == "" || !ok {
		return
	}

	ownerExecSessions.Lock()
	defer ownerExecSessions.Unlock()

	ownerExecSessions.byKey[key] = ownerExecSession{
		cwd:       cwd,
		updatedAt: time.Now(),
	}
}

func pruneOwnerExecSessions() {
	ownerExecSessions.Lock()
	defer ownerExecSessions.Unlock()

	cutoff := time.Now().Add(-ownerExecSessionTTL)
	for key, session := range ownerExecSessions.byKey {
		if session.updatedAt.Before(cutoff) {
			delete(ownerExecSessions.byKey, key)
		}
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:     "$",
		As:       []string{"$"},
		Tags:     "owner",
		IsPrefix: false,
		IsOwner:  true,
		IsQuery:  true,
		Exec:     ownerExec,
	})
}
