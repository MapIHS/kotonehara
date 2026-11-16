package commands

import (
	"context"
	"errors"
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
	m.Body, m.IsBot = ".paid", true
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 2 || checks != 1 {
		t.Fatal("bot account was not exempt from quota")
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

func TestQuotaNotChargedWhenPreconditionsFail(t *testing.T) {
	isolateCommands(t)
	checks, ran := 0, 0
	SetQuotaCheck(func(context.Context, string, []string) (bool, string, error) {
		checks++
		return true, "", nil
	})
	Register(&Command{Name: "needmedia", IsPrefix: true, IsMedia: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	Register(&Command{Name: "needquery", IsPrefix: true, IsQuery: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	Register(&Command{Name: "grouponly", IsPrefix: true, IsGroup: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	Register(&Command{Name: "ok", IsPrefix: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	cfg := config.Config{Prefix: "."}
	reply := func(context.Context, string, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		return whatsmeow.SendResponse{}, nil
	}
	for _, body := range []string{".needmedia", ".needquery", ".grouponly"} {
		CommandExec(context.Background(), nil, &message.Message{Body: body, Reply: reply}, cfg)
	}
	if checks != 0 || ran != 0 {
		t.Fatalf("rejected invocations charged quota: checks=%d ran=%d", checks, ran)
	}
	CommandExec(context.Background(), nil, &message.Message{Body: ".ok", Reply: reply}, cfg)
	if checks != 1 || ran != 1 {
		t.Fatalf("valid invocation: checks=%d ran=%d", checks, ran)
	}
}

func TestQuotaErrorBlocksCommand(t *testing.T) {
	isolateCommands(t)
	ran := 0
	SetQuotaCheck(func(context.Context, string, []string) (bool, string, error) {
		return false, "", errors.New("db down")
	})
	Register(&Command{Name: "paid", IsPrefix: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	reply := ""
	m := &message.Message{Body: ".paid", Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		reply = text
		return whatsmeow.SendResponse{}, nil
	}}
	CommandExec(context.Background(), nil, m, config.Config{Prefix: "."})
	if ran != 0 || reply == "" {
		t.Fatal("command ran despite quota error")
	}
}

func TestWrongPrefixDoesNotStartCooldown(t *testing.T) {
	isolateCommands(t)
	SetCooldown(time.Hour)
	ran := 0
	Register(&Command{Name: "probe", IsPrefix: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	cfg := config.Config{Prefix: "."}
	sender := types.NewJID("628123", types.DefaultUserServer)
	reply := func(context.Context, string, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		return whatsmeow.SendResponse{}, nil
	}
	// Unprefixed "probe" is ordinary chat text, not an invocation.
	CommandExec(context.Background(), nil, &message.Message{Body: "probe", Sender: sender, Reply: reply}, cfg)
	CommandExec(context.Background(), nil, &message.Message{Body: ".probe", Sender: sender, Reply: reply}, cfg)
	if ran != 1 {
		t.Fatal("unprefixed text started a cooldown and blocked the real command")
	}
}

func TestHelpFlagShowsDescriptionWithoutRunning(t *testing.T) {
	isolateCommands(t)
	checks, ran := 0, 0
	SetQuotaCheck(func(context.Context, string, []string) (bool, string, error) {
		checks++
		return true, "", nil
	})
	Register(&Command{Name: "sticker", As: []string{"s"}, Tags: "convert", Description: "Ubah gambar jadi stiker", IsPrefix: true, IsMedia: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	Register(&Command{Name: "menu", IsPrefix: true, OwnHelpFlag: true, Exec: func(_ context.Context, _ *clients.Client, m *message.Message, _ config.Config) {
		if IsHelpFlag(m.Query) {
			ran += 10
		}
	}})
	cfg := config.Config{Prefix: "."}
	reply := ""
	m := &message.Message{Body: ".s --D", Query: "--D", Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		reply = text
		return whatsmeow.SendResponse{}, nil
	}}
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 0 || checks != 0 {
		t.Fatalf("--d ran the command or charged quota: ran=%d checks=%d", ran, checks)
	}
	for _, want := range []string{"*.sticker*", "Ubah gambar jadi stiker", "Alias: .s", "butuh media"} {
		if !strings.Contains(reply, want) {
			t.Errorf("help %q missing %q", reply, want)
		}
	}
	// A command that owns the flag receives it instead of the generic reply.
	m.Body, m.Query = ".menu --d", "--d"
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 10 {
		t.Fatal("OwnHelpFlag command did not receive --d")
	}
	// --d inside a longer query is ordinary input.
	Register(&Command{Name: "echo", IsPrefix: true, Exec: func(context.Context, *clients.Client, *message.Message, config.Config) { ran++ }})
	m.Body, m.Query = ".echo pakai --d", "pakai --d"
	CommandExec(context.Background(), nil, m, cfg)
	if ran != 11 {
		t.Fatal("--d inside a query was intercepted")
	}
}

func TestMenuDescriptionsOnlyWithFlag(t *testing.T) {
	isolateCommands(t)
	Register(&Command{Name: "sticker", As: []string{"s"}, Tags: "convert", Description: "Ubah gambar jadi stiker", IsPrefix: true})
	Register(&Command{Name: "waifu", Tags: "nsfw", Description: "rahasia", IsPrefix: true, IsPrivate: true})
	plain := BuildMenuText(".", false, false, false)
	if strings.Contains(plain, "Ubah gambar") || !strings.Contains(plain, ".sticker (alias: .s)") {
		t.Fatalf("plain menu: %q", plain)
	}
	if detailed := BuildMenuText(".", false, false, true); !strings.Contains(detailed, "Ubah gambar jadi stiker") {
		t.Fatalf("detailed menu lacks description: %q", detailed)
	}
	if group := BuildMenuText(".", true, false, true); strings.Contains(group, "waifu") {
		t.Fatal("private NSFW command shown in group with NSFW disabled")
	}
}
