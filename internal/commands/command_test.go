package commands

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/identity"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func isolateCommands(t *testing.T) {
	t.Helper()
	previous, previousQuota := reg, quotaCheck
	reg = &registry{byKey: map[string]*Command{}, byTag: map[string][]*Command{}}
	quotaCheck = nil
	SetCooldown(0)
	t.Cleanup(func() { reg, quotaCheck = previous, previousQuota; SetCooldown(3 * time.Second) })
}

func TestDispatchPrefixAliasAndValidation(t *testing.T) {
	isolateCommands(t)
	ran := 0
	Register(&Command{Name: "probe", As: []string{"alias"}, Tags: "test", IsPrefix: true, IsQuery: true, IsMedia: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	Register(&Command{Name: "disabled", Disable: true})
	cfg := config.Config{Prefix: "."}
	for _, tc := range []struct {
		body string
		want bool
	}{{".ALIAS hello", true}, {" .probe hi ", true}, {"probe hi", false}, {".disabled", false}, {".missing", false}, {"", false}} {
		if got := CanHandle(tc.body, cfg); got != tc.want {
			t.Errorf("CanHandle(%q) = %t", tc.body, got)
		}
	}
	var reply string
	m := &message.Message{Body: ".alias hello", Query: "hello", Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		reply = text
		return whatsmeow.SendResponse{}, nil
	}}
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 0 || !strings.Contains(reply, "Media") {
		t.Fatal("media validation did not block execution")
	}
	// A second command tests successful dispatch and query validation without a WA connection.
	Register(&Command{Name: "text", As: []string{"txt"}, IsPrefix: true, IsQuery: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	m.Body, m.Query = ".TXT hello", "hello"
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 1 {
		t.Fatal("alias was not dispatched")
	}
	m.Query = ""
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 1 || !strings.Contains(reply, "Kueri") {
		t.Fatal("empty query was executed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	CommandExec(ctx, nil, m, cfg)
	if ran != 1 {
		t.Fatal("canceled command executed")
	}
}

func TestQuotaUsesIdentityAliasesAndSkipQuota(t *testing.T) {
	isolateCommands(t)
	pn, lid := types.NewJID("628123", types.DefaultUserServer), types.NewJID("777", types.HiddenUserServer)
	ran, checks := 0, 0
	SetQuotaCheck(func(_ context.Context, jid string, aliases []string) (bool, string, error) {
		checks++
		if jid != pn.String() || len(aliases) != 2 {
			t.Errorf("identity lost: %s %v", jid, aliases)
		}
		return false, "limit reached", nil
	})
	Register(&Command{Name: "paid", IsPrefix: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	Register(&Command{Name: "free", IsPrefix: true, SkipQuota: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	reply := ""
	m := &message.Message{Body: ".paid", Identity: identity.New(lid, pn), Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		reply = text
		return whatsmeow.SendResponse{}, nil
	}}
	cfg := config.Config{Prefix: "."}
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 0 || checks != 1 || reply != "limit reached" {
		t.Fatal("quota did not block")
	}
	m.Body = ".free"
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 1 || checks != 1 {
		t.Fatal("SkipQuota not respected")
	}
}

func TestCooldownSurvivesIdentitySwitchAndResets(t *testing.T) {
	isolateCommands(t)
	SetCooldown(time.Hour)
	if !allowCooldownAliases([]string{"lid:777|cmd", "pn:628123|cmd"}) {
		t.Fatal("initial request blocked")
	}
	if allowCooldownAliases([]string{"pn:628123|cmd"}) {
		t.Fatal("switching identity bypassed cooldown")
	}
	if !allowCooldownAliases([]string{"pn:628123|other"}) {
		t.Fatal("unrelated command blocked")
	}
	SetCooldown(time.Hour)
	if !allowCooldownAliases([]string{"pn:628123|cmd"}) {
		t.Fatal("old cooldown survived lifecycle reset")
	}
}

func TestDispatchChatRestrictions(t *testing.T) {
	isolateCommands(t)
	ran := 0
	Register(&Command{Name: "group", IsPrefix: true, IsGroup: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	Register(&Command{Name: "private", IsPrefix: true, IsPrivate: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	cfg := config.Config{Prefix: "."}
	for _, tc := range []struct {
		body    string
		group   bool
		allowed bool
	}{
		{".group", false, false}, {".private", true, false}, {".group", true, true}, {".private", false, true},
	} {
		before := ran
		m := &message.Message{Body: tc.body, IsGroup: tc.group, Reply: func(context.Context, string, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			return whatsmeow.SendResponse{}, nil
		}}
		CommandExec(context.Background(), nil, m, cfg)
		if (ran > before) != tc.allowed {
			t.Errorf("incorrect chat restriction: %+v", tc)
		}
	}
}
