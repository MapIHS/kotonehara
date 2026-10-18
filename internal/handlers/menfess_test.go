package handlers

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/identity"
	"github.com/MapIHS/kotonehara/internal/menfess"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	_ "modernc.org/sqlite"
)

type menfessSent struct {
	to, text string
	info     *waE2E.ContextInfo
}
type menfessFakeClient struct {
	sent     []menfessSent
	fail     bool
	resolved identity.Identity
}

func (f *menfessFakeClient) ResolveIdentity(_ context.Context, a, b types.JID) (identity.Identity, error) {
	if !f.resolved.PN.IsEmpty() {
		return f.resolved, nil
	}
	return identity.New(a, b), nil
}
func (f *menfessFakeClient) BotIdentity() identity.Identity {
	return identity.New(types.NewJID("628000000000", types.DefaultUserServer), types.EmptyJID)
}
func (f *menfessFakeClient) SendText(_ context.Context, to types.JID, text string, info *waE2E.ContextInfo, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	if f.fail {
		return whatsmeow.SendResponse{}, errors.New("fixture delivery failed")
	}
	f.sent = append(f.sent, menfessSent{to.String(), text, info})
	return whatsmeow.SendResponse{}, nil
}

func menfessTestSetup(t *testing.T) (*menfess.Service, *menfessFakeClient) {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s, err := menfess.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	previous := menfess.Default()
	menfess.SetDefault(s)
	t.Cleanup(func() { menfess.SetDefault(previous); db.Close() })
	return s, &menfessFakeClient{}
}

func menfessTestMessage(jid, query string) (*message.Message, *[]string) {
	replies := []string{}
	sender, _ := types.ParseJID(jid)
	m := &message.Message{Sender: sender, From: sender, Query: query, ID: &waE2E.ContextInfo{Participant: &jid}, Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		replies = append(replies, text)
		return whatsmeow.SendResponse{}, nil
	}}
	return m, &replies
}

func TestMenfessCommandsConsentAndPrivacy(t *testing.T) {
	s, client := menfessTestSetup(t)
	ctx := context.Background()
	sender, recipient := "628123456789@s.whatsapp.net", "628987654321@s.whatsapp.net"
	m, replies := menfessTestMessage(sender, "628987654321 | Bintang | Halo rahasia")
	handleMenfess(ctx, client, m, "!", "menfess")
	if len(client.sent) != 1 || client.sent[0].to != recipient || !strings.Contains(client.sent[0].text, "!terimamenfess") || strings.Contains(client.sent[0].text, "Halo rahasia") {
		t.Fatalf("bad invitation: %+v %v", client.sent, *replies)
	}
	if strings.Contains(client.sent[0].text, "628123456789") || client.sent[0].info != nil {
		t.Fatal("invitation leaked sender identity or quote metadata")
	}
	var code string
	for _, line := range strings.Split(client.sent[0].text, "\n") {
		if strings.HasPrefix(line, "Kode: ") {
			code = strings.TrimPrefix(line, "Kode: ")
		}
	}
	if code == "" {
		t.Fatal("invitation missing code")
	}
	client.sent = nil
	m, _ = menfessTestMessage(recipient, code)
	handleMenfess(ctx, client, m, "!", "terimamenfess")
	if len(client.sent) != 2 || client.sent[0].to != recipient || !strings.Contains(client.sent[0].text, "Halo rahasia") || client.sent[1].to != sender {
		t.Fatalf("accept deliveries: %+v", client.sent)
	}
	for _, sent := range client.sent {
		if sent.info != nil || strings.Contains(sent.text, "628123456789") || strings.Contains(sent.text, "628987654321") {
			t.Fatal("accept leaked metadata or number")
		}
	}
	client.sent = nil
	m, replies = menfessTestMessage(recipient, code+" | Halo kembali")
	handleMenfess(ctx, client, m, "!", "balas")
	if len(client.sent) != 1 || client.sent[0].to != sender || !strings.Contains(client.sent[0].text, "Halo kembali") || client.sent[0].info != nil {
		t.Fatalf("bad relay: %+v %v", client.sent, *replies)
	}
	if _, err := s.Reply(ctx, "628111111111@s.whatsapp.net", code, "intruder"); !errors.Is(err, menfess.ErrSession) {
		t.Fatalf("outsider accessed session: %v", err)
	}
	client.sent = nil
	m, _ = menfessTestMessage(recipient, "")
	handleMenfess(ctx, client, m, "!", "blokmenfess")
	if _, err := s.Reply(ctx, sender, code, "blocked"); !errors.Is(err, menfess.ErrSession) {
		t.Fatalf("block kept session open: %v", err)
	}
}

func TestMenfessFailedInvitationClosesSession(t *testing.T) {
	s, client := menfessTestSetup(t)
	client.fail = true
	sender := "628123456789@s.whatsapp.net"
	m, replies := menfessTestMessage(sender, "628987654321 | Bintang | Hallo")
	handleMenfess(context.Background(), client, m, ".", "menfess")
	if !strings.Contains(strings.Join(*replies, "\n"), "gagal dikirim") {
		t.Fatalf("missing delivery failure: %v", *replies)
	}
	// If the failed invitation remained live, the next invite would return ErrBusy.
	if _, err := s.Invite(context.Background(), sender, "628222222222@s.whatsapp.net", "Anon", "Retry"); err != nil {
		t.Fatalf("failed invitation stuck: %v", err)
	}
}

func TestMenfessPrivateOnlyAndIdentityMapping(t *testing.T) {
	_, client := menfessTestSetup(t)
	m, replies := menfessTestMessage("628123456789@s.whatsapp.net", "628987654321 | Anon | Hello")
	m.IsGroup = true
	handleMenfess(context.Background(), client, m, ".", "menfess")
	if len(client.sent) != 0 || !strings.Contains(strings.Join(*replies, "\n"), "chat pribadi") {
		t.Fatal("group invitation was allowed")
	}
	m.IsGroup = false
	m.Sender = types.NewJID("12345", types.HiddenUserServer)
	client.resolved = identity.New(m.Sender, types.NewJID("628123456789", types.DefaultUserServer))
	handleMenfess(context.Background(), client, m, ".", "menfess")
	if len(client.sent) != 1 {
		t.Fatalf("LID identity did not resolve: %+v %v", client.sent, *replies)
	}
	for _, cmd := range commands.Commands() {
		if cmd.Tags != "menfess" {
			continue
		}
		if !cmd.IsPrivate || !cmd.IsPrefix {
			t.Errorf("command missing restrictions: %s", cmd.Name)
		}
		if (cmd.Name == "stopmenfess" || cmd.Name == "blokmenfess" || cmd.Name == "terimamenfess") && !cmd.SkipQuota {
			t.Errorf("control command quota locked: %s", cmd.Name)
		}
	}
}

func TestMenfessInputParser(t *testing.T) {
	for _, input := range []string{"628123456789", "group@g.us | Anon | hello", "abc628123456789 | Anon | hello", "628123456789 | Anon | "} {
		if _, _, _, err := parseMenfessInvite(input); err == nil {
			t.Errorf("accepted invalid input %q", input)
		}
	}
	target, alias, text, err := parseMenfessInvite("+628123456789 | Anon | hello | world")
	if err != nil || target.User != "628123456789" || alias != "Anon" || text != "hello | world" {
		t.Fatalf("bad parsed invitation: %s %q %q %v", target, alias, text, err)
	}
}
