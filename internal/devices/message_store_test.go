package devices

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
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

func TestStoreCapturesEvenWhenCommandQueueIsFull(t *testing.T) {
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
	d := New(nil, config.Config{}, context.Background())
	d.Messages = s
	d.maxQueued = 0
	defer d.Shutdown(context.Background())
	bot := types.NewJID("999", types.DefaultUserServer)
	sender := types.NewJID("123", types.DefaultUserServer)
	raw := &waE2E.Message{Conversation: proto.String("ordinary chat")}
	e := &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: sender, Sender: sender}, ID: "first", Timestamp: time.Now()}, RawMessage: raw, Message: raw}
	d.registerEventHandler(&whatsmeow.Client{Store: &store.Device{ID: &bot}}, nil)(e)
	fromMe := false
	if _, err := s.Get(context.Background(), msgstore.Key{Account: bot.String(), Chat: sender, Sender: sender, ID: e.Info.ID, FromMe: &fromMe}); err != nil {
		t.Fatalf("command overload lost stored message: %v", err)
	}
}
