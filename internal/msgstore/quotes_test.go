package msgstore

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func quotedEvent(id string, complete bool) *events.Message {
	e := event(id)
	image := &waE2E.ImageMessage{ViewOnce: proto.Bool(true)}
	if complete {
		image.DirectPath = proto.String("/complete")
		image.MediaKey = bytes.Repeat([]byte{1}, 32)
		image.FileSHA256 = bytes.Repeat([]byte{2}, 32)
		image.FileEncSHA256 = bytes.Repeat([]byte{3}, 32)
	}
	e.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("reply"), ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("missing-original"), Participant: proto.String(e.Info.Sender.String()), QuotedMessage: &waE2E.Message{ImageMessage: image}}}}
	e.RawMessage = e.Message
	return e
}

func targetKey(e *events.Message) Key { k := key(e); k.ID = "missing-original"; return k }

func TestQuotedMediaFallbackBeforeCommitAfterRestartAndChatIsolation(t *testing.T) {
	db, s := fixture(t, Options{CacheEntries: 1})
	e := quotedEvent("observation", true)
	if !s.Capture("bot", e) {
		t.Fatal("quote rejected")
	}
	s.Capture("bot", event("evict-cache"))
	k := targetKey(e)
	if _, err := s.Get(context.Background(), k); err != sql.ErrNoRows {
		t.Fatalf("quote must not masquerade as an original: %v", err)
	}
	m, err := s.GetQuotedMedia(context.Background(), k)
	if err != nil || !m.IsViewOnce || m.Raw.GetImageMessage().GetDirectPath() != "/complete" {
		t.Fatalf("pending quote: %+v %v", m, err)
	}
	m.Raw.ImageMessage.MediaKey[0] = 9
	m, err = s.GetQuotedMedia(context.Background(), k)
	if err != nil || m.Raw.GetImageMessage().GetMediaKey()[0] != 1 {
		t.Fatal("caller mutated quote snapshot")
	}
	shutdown(t, s)
	restarted, err := New(context.Background(), db, Options{CacheBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown(t, restarted)
	if _, err := restarted.GetQuotedMedia(context.Background(), k); err != nil {
		t.Fatalf("persisted quote: %v", err)
	}
	for _, bad := range []Key{
		{Account: "other-bot", Chat: k.Chat, Sender: k.Sender, ID: k.ID},
		{Account: k.Account, Chat: types.NewJID("other", types.HiddenUserServer), Sender: k.Sender, ID: k.ID},
		{Account: k.Account, Chat: k.Chat, Sender: types.NewJID("other", types.HiddenUserServer), ID: k.ID},
		{Account: k.Account, Chat: k.Chat, ID: k.ID},
	} {
		if _, err := restarted.GetQuotedMedia(context.Background(), bad); err != sql.ErrNoRows {
			t.Fatalf("scope bypass: %v", err)
		}
	}
	// Genuine originals remain preferred and are never overwritten by quotes.
	original := event(k.ID)
	original.Message = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{DirectPath: proto.String("/original")}}
	original.RawMessage = original.Message
	original.IsViewOnce = true
	restarted.Capture("bot", original)
	m, err = restarted.Get(context.Background(), k)
	if err != nil || m.Raw.GetImageMessage().GetDirectPath() != "/original" {
		t.Fatalf("original precedence: %v", err)
	}
}

func TestQuoteUpgradeBackfillsExistingRepliesAndPrunesReferences(t *testing.T) {
	db, s := fixture(t, Options{CacheBytes: 1})
	s.Capture("bot", quotedEvent("incomplete", false))
	e := quotedEvent("complete", true)
	s.Capture("bot", e)
	shutdown(t, s)
	// Emulate the old schema: replies exist, but no quote reference table yet.
	if _, err := db.Exec("DROP TABLE bot_message_quotes"); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(context.Background(), db, Options{CacheBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown(t, restarted)
	if _, err := restarted.GetQuotedMedia(context.Background(), targetKey(e)); err != nil {
		t.Fatalf("backfill missed existing complete quote: %v", err)
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM bot_message_quotes"); err != nil || count != 1 {
		t.Fatalf("indexed incomplete quote: %d %v", count, err)
	}
	if _, err := db.Exec("UPDATE bot_messages SET expires_at=0"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&count, "SELECT count(*) FROM bot_message_quotes"); err != nil || count != 0 {
		t.Fatalf("orphan quote references: %d %v", count, err)
	}
	if _, err := restarted.GetQuotedMedia(context.Background(), targetKey(e)); err != sql.ErrNoRows {
		t.Fatalf("expired quote: %v", err)
	}
}

func TestQuoteIndexFiltersAndNormalizesSender(t *testing.T) {
	_, s := fixture(t, Options{})
	for _, kind := range []string{"ordinary", "missing metadata", "remote chat", "missing participant"} {
		e := quotedEvent(kind, true)
		ci := e.Message.GetExtendedTextMessage().GetContextInfo()
		switch kind {
		case "ordinary":
			ci.QuotedMessage.ImageMessage.ViewOnce = nil
		case "missing metadata":
			ci.QuotedMessage.ImageMessage.MediaKey = nil
		case "remote chat":
			ci.RemoteJID = proto.String("other@g.us")
		case "missing participant":
			ci.Participant = nil
		}
		s.Capture("bot", e)
		if _, err := s.GetQuotedMedia(context.Background(), targetKey(e)); err != sql.ErrNoRows {
			t.Fatalf("invalid quote %s indexed: %v", kind, err)
		}
	}
	e := quotedEvent("device-sender", true)
	sender := e.Info.Sender
	sender.Device = 4
	e.Message.ExtendedTextMessage.ContextInfo.Participant = proto.String(sender.String())
	s.Capture("bot", e)
	if _, err := s.GetQuotedMedia(context.Background(), targetKey(e)); err != nil {
		t.Fatalf("device JID: %v", err)
	}
	shutdown(t, s)
	// All rows still have the regular finite retention of their observing event.
	if st := s.Stats(); st.PendingEntries != 0 || st.Written != 5 || st.LastWriteDuration > 5*time.Second {
		t.Fatalf("quote persistence: %+v", st)
	}
}
