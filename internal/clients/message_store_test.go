package clients

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

func TestRestoreViewOnceFlagWithCompleteQuoteAndMissingParticipant(t *testing.T) {
	db, err := sqlx.Connect("sqlite", filepath.Join(t.TempDir(), "messages.db"))
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
	bot, sender := types.NewJID("999", types.DefaultUserServer), types.NewJID("123", types.DefaultUserServer)
	c := &Client{WA: &whatsmeow.Client{Store: &store.Device{ID: &bot}}, Messages: s}
	raw := rvoSource("image")
	raw.ImageMessage.ViewOnce = nil
	// Some callers retain the event flag separately from the unwrapped protobuf.
	e := &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: sender, Sender: sender}, ID: "original", Timestamp: time.Now()}, Message: raw, RawMessage: raw, IsViewOnce: true}
	if !s.Capture(c.MessageStoreAccount(), e) {
		t.Fatal("capture rejected")
	}
	for _, participant := range []string{sender.String(), ""} {
		t.Run(participant, func(t *testing.T) {
			quote := &waE2E.ContextInfo{StanzaID: proto.String("original"), Participant: proto.String(participant), QuotedMessage: proto.Clone(raw).(*waE2E.Message)}
			command := &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: sender, Sender: sender}, ID: "rvo"}, Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(".rvo"), ContextInfo: quote}}}
			m := message.NewParser(nil, config.Config{Prefix: "."}).Parse(context.Background(), command)
			if m.IsQuotedViewOnce || m.Media == nil {
				t.Fatal("fixture must have complete media but no view-once flag")
			}
			if !c.RestoreQuotedMessage(context.Background(), m) || !m.IsQuotedViewOnce {
				t.Fatal("store flag not restored")
			}
			m.From = types.NewJID("other", types.DefaultUserServer)
			m.IsQuotedViewOnce = false
			if c.RestoreQuotedMessage(context.Background(), m) || m.IsQuotedViewOnce {
				t.Fatal("message restored into wrong chat")
			}
		})
	}
}
