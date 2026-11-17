package commands

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/msgstore"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

func TestStoredMediaRestoredBeforeCommandValidation(t *testing.T) {
	isolateCommands(t)
	db, err := sqlx.Connect("sqlite", filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	s, err := msgstore.New(context.Background(), db, msgstore.Options{FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()
	bot := types.NewJID("999", types.DefaultUserServer)
	c := &clients.Client{WA: &whatsmeow.Client{Store: &store.Device{ID: &bot}}, Messages: s}
	sender := types.NewJID("123", types.DefaultUserServer)
	group := types.NewJID("456", types.GroupServer)
	raw := &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{DirectPath: proto.String("/complete"), MediaKey: []byte("key"), FileEncSHA256: []byte("hash")}}}}
	original := (&events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: group, Sender: sender, IsGroup: true}, ID: "original", Timestamp: time.Now()}, RawMessage: raw}).UnwrapRaw()
	if !s.Capture(c.MessageStoreAccount(), original) {
		t.Fatal("capture rejected")
	}
	ran := 0
	Register(&Command{Name: "probe", IsPrefix: true, IsMedia: true, SkipQuota: true, Exec: func(_ context.Context, _ *clients.Client, m *message.Message, _ config.Config) {
		ran++
		if m.Media.GetDirectPath() != "/complete" || !m.IsQuotedImage || !m.IsQuotedViewOnce {
			t.Fatal("restored metadata/flags missing")
		}
	}})
	cfg := config.Config{Prefix: "."}
	for _, tc := range []struct {
		chat    types.JID
		allowed bool
	}{{group, true}, {types.NewJID("other", types.GroupServer), false}} {
		contextInfo := &waE2E.ContextInfo{StanzaID: proto.String("original"), Participant: proto.String(sender.String()), QuotedMessage: &waE2E.Message{}}
		evt := &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: tc.chat, Sender: sender, IsGroup: true}, ID: "command"}, Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(".probe"), ContextInfo: contextInfo}}}
		m := message.NewParser(nil, cfg).Parse(context.Background(), evt)
		m.Reply = func(context.Context, string, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			return whatsmeow.SendResponse{}, nil
		}
		before := ran
		CommandExec(context.Background(), c, m, cfg)
		if (ran > before) != tc.allowed {
			t.Fatalf("chat isolation failed: %s", tc.chat)
		}
	}
	if s.Stats().DBHits != 0 {
		t.Fatal("recent media needed database read")
	}
}
