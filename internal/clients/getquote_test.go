package clients

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

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

func quoteFixture(t *testing.T) (*Client, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Connect("sqlite", filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	s, err := msgstore.New(context.Background(), db, msgstore.Options{FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopQuoteStore(t, s) })
	bot := types.NewJID("999", types.DefaultUserServer)
	return &Client{WA: &whatsmeow.Client{Store: &store.Device{ID: &bot, LID: types.NewJID("777", types.HiddenUserServer)}}, Messages: s}, db
}
func stopQuoteStore(t *testing.T, s *msgstore.Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func quoteEvent(chat, sender types.JID, id string, raw *waE2E.Message) *events.Message {
	return &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: sender, IsGroup: chat.Server == types.GroupServer}, ID: id, Timestamp: time.Now()}, RawMessage: raw, Message: raw}
}

func replyContent(text, id string, sender types.JID, quoted *waE2E.Message) *waE2E.Message {
	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text), ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String(id), Participant: proto.String(sender.String()), QuotedMessage: quoted}}}
}

func quoteCommand(chat, parentSender types.JID, parentID string, preview *waE2E.Message) *message.Message {
	return &message.Message{From: chat, IsGroup: chat.Server == types.GroupServer, QuotedMsg: preview, ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String(parentID), Participant: proto.String(parentSender.String())}}
}

func TestGetQuoteRetrievesOriginalTextAndDeletedImageAfterRestart(t *testing.T) {
	for _, kind := range []string{"text", "image"} {
		t.Run(kind, func(t *testing.T) {
			c, db := quoteFixture(t)
			chat := types.NewJID("group", types.GroupServer)
			a := types.NewJID("111", types.DefaultUserServer)
			b := types.NewJID("222", types.DefaultUserServer)
			original := &waE2E.Message{Conversation: proto.String("Halo!")}
			if kind == "image" {
				original = rvoSource("image")
				original.ImageMessage.ViewOnce = nil
				original.ImageMessage.Caption = proto.String("Test")
			}
			c.Messages.Capture(c.MessageStoreAccount(), quoteEvent(chat, a, "original", original))
			parent := replyContent("Hai.", "original", a, &waE2E.Message{Conversation: proto.String("preview")})
			c.Messages.Capture(c.MessageStoreAccount(), quoteEvent(chat, b, "reply", parent))
			revoke := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum()}}
			if c.Messages.Capture(c.MessageStoreAccount(), quoteEvent(chat, a, "original", revoke)) {
				t.Fatal("revoke overwrote original")
			}
			stopQuoteStore(t, c.Messages)
			restarted, err := msgstore.New(context.Background(), db, msgstore.Options{CacheBytes: 1, FlushInterval: time.Hour})
			if err != nil {
				t.Fatal(err)
			}
			defer stopQuoteStore(t, restarted)
			c.Messages = restarted
			// WhatsApp's preview does not carry the parent's own reply metadata.
			m := quoteCommand(chat, b, "reply", &waE2E.Message{Conversation: proto.String("Hai.")})
			result, err := c.GetQuote(context.Background(), m)
			if err != nil || !proto.Equal(result, original) {
				t.Fatalf("getquote returned the parent/preview instead of original: %+v %v", result, err)
			}
			if kind == "text" {
				result.Conversation = proto.String("mutated")
			} else {
				result.ImageMessage.Caption = proto.String("mutated")
			}
			again, err := c.GetQuote(context.Background(), m)
			if err != nil || !proto.Equal(again, original) {
				t.Fatal("caller mutated stored original")
			}
		})
	}
}

func TestGetQuoteFallbackAndIsolation(t *testing.T) {
	c, _ := quoteFixture(t)
	chat := types.NewJID("group", types.GroupServer)
	sender := types.NewJID("111", types.DefaultUserServer)
	original := &waE2E.Message{Conversation: proto.String("Halo!")}
	parent := replyContent("Hai.", "original", sender, original)
	m := quoteCommand(chat, sender, "reply", parent)
	// Complete nested quotes work without either original event in the store.
	result, err := c.GetQuote(context.Background(), m)
	if err != nil || !proto.Equal(result, original) {
		t.Fatalf("embedded fallback: %v", err)
	}
	c.Messages.Capture(c.MessageStoreAccount(), quoteEvent(chat, sender, "reply", parent))
	for _, tc := range []struct {
		name string
		cmd  *message.Message
		want error
	}{
		{"not a reply", &message.Message{}, sql.ErrNoRows},
		{"parent without quote", quoteCommand(chat, sender, "unknown", &waE2E.Message{Conversation: proto.String("Hai.")}), sql.ErrNoRows},
		{"other chat cannot retrieve parent", quoteCommand(types.NewJID("other", types.GroupServer), sender, "reply", &waE2E.Message{}), sql.ErrNoRows},
		{"wrong parent sender", quoteCommand(chat, types.NewJID("other", types.DefaultUserServer), "reply", &waE2E.Message{}), sql.ErrNoRows},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.GetQuote(context.Background(), tc.cmd); !errors.Is(err, tc.want) {
				t.Fatalf("lookup: %v", err)
			}
		})
	}
	foreign := proto.Clone(parent).(*waE2E.Message)
	foreign.ExtendedTextMessage.ContextInfo.RemoteJID = proto.String("other@g.us")
	if _, err := c.GetQuote(context.Background(), quoteCommand(chat, sender, "unknown", foreign)); !errors.Is(err, ErrQuoteDifferentChat) {
		t.Fatalf("cross-chat nested quote: %v", err)
	}
	c.Messages = nil
	if result, err := c.GetQuote(context.Background(), m); err != nil || !proto.Equal(result, original) {
		t.Fatalf("store disabled fallback: %v", err)
	}
	// RemoteJID may use the verified PN alternative of the current LID chat.
	lid := types.NewJID("12345", types.HiddenUserServer)
	pn := types.NewJID("628123", types.DefaultUserServer)
	dmParent := proto.Clone(parent).(*waE2E.Message)
	dmParent.ExtendedTextMessage.ContextInfo.RemoteJID = proto.String(pn.String())
	dm := quoteCommand(lid, sender, "unknown", dmParent)
	dm.Msg = &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: lid, Sender: lid, SenderAlt: pn}}}
	if result, err := c.GetQuote(context.Background(), dm); err != nil || !proto.Equal(result, original) {
		t.Fatalf("verified chat alias rejected: %v", err)
	}
}

type sentMessageStub struct {
	response whatsmeow.SendResponse
	err      error
}

func (s sentMessageStub) SendMessage(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	return s.response, s.err
}

func TestSentBotRepliesAreStoredOnlyAfterSuccessfulSend(t *testing.T) {
	c, _ := quoteFixture(t)
	chat := types.NewJID("group", types.GroupServer)
	sender := types.NewJID("111", types.DefaultUserServer)
	original := &waE2E.Message{Conversation: proto.String("Halo!")}
	parent := replyContent("Hai.", "original", sender, original)
	if _, err := c.sendStored(context.Background(), sentMessageStub{response: whatsmeow.SendResponse{ID: "sent"}}, chat, parent); err != nil {
		t.Fatal(err)
	}
	parent.ExtendedTextMessage.ContextInfo.QuotedMessage.Conversation = proto.String("mutated after send")
	m := quoteCommand(chat, c.WA.Store.GetJID(), "sent", &waE2E.Message{Conversation: proto.String("Hai.")})
	result, err := c.GetQuote(context.Background(), m)
	if err != nil || result.GetConversation() != "Halo!" {
		t.Fatalf("bot's reply not stored independently: %+v %v", result, err)
	}
	before := c.Messages.Stats().Accepted
	failed := errors.New("send failed")
	if _, err := c.sendStored(context.Background(), sentMessageStub{response: whatsmeow.SendResponse{ID: "failed"}, err: failed}, chat, parent); !errors.Is(err, failed) {
		t.Fatal("send error lost")
	}
	if c.Messages.Stats().Accepted != before {
		t.Fatal("failed send entered store")
	}
}

func TestForwardedMessagePreservesContentAndDropsReplyRouting(t *testing.T) {
	for _, kind := range []string{"text", "image", "video", "audio", "document", "sticker"} {
		t.Run(kind, func(t *testing.T) {
			var source *waE2E.Message
			switch kind {
			case "text":
				source = replyContent("Halo!", "previous", types.NewJID("111", types.DefaultUserServer), &waE2E.Message{Conversation: proto.String("old")})
			case "document":
				m := rvoSource("image").ImageMessage
				source = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{DirectPath: m.DirectPath, MediaKey: m.MediaKey, FileSHA256: m.FileSHA256, FileEncSHA256: m.FileEncSHA256, FileName: proto.String("file.pdf"), Caption: proto.String("Test")}}
			case "sticker":
				m := rvoSource("image").ImageMessage
				source = &waE2E.Message{StickerMessage: &waE2E.StickerMessage{DirectPath: m.DirectPath, MediaKey: m.MediaKey, FileSHA256: m.FileSHA256, FileEncSHA256: m.FileEncSHA256, IsAnimated: proto.Bool(true)}}
			default:
				source = rvoSource(kind)
				if source.ImageMessage != nil {
					source.ImageMessage.ViewOnce = nil
				}
				if source.VideoMessage != nil {
					source.VideoMessage.ViewOnce = nil
				}
				if source.AudioMessage != nil {
					source.AudioMessage.ViewOnce = nil
				}
			}
			source.ProtocolMessage = &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum()}
			source.MessageContextInfo = &waE2E.MessageContextInfo{MessageSecret: []byte("do not replay")}
			before := proto.Clone(source)
			out, err := forwardedMessage(source)
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(source, before) {
				t.Fatal("forward mutated source")
			}
			if out.ProtocolMessage != nil || out.MessageContextInfo != nil {
				t.Fatal("protocol/secret replayed")
			}
			ci := message.ExtractContextInfo(out)
			if !ci.GetIsForwarded() || ci.GetForwardingScore() != 1 || ci.GetStanzaID() != "" || ci.QuotedMessage != nil {
				t.Fatal("incorrect forwarding context")
			}
			switch kind {
			case "text":
				if out.ExtendedTextMessage.GetText() != "Halo!" {
					t.Fatal("text lost")
				}
			case "image":
				if out.ImageMessage.GetCaption() != "caption" {
					t.Fatal("caption lost")
				}
			case "video":
				if out.VideoMessage.GetSeconds() != 5 {
					t.Fatal("video metadata lost")
				}
			case "audio":
				if !out.AudioMessage.GetPTT() {
					t.Fatal("voice note lost")
				}
			case "document":
				if out.DocumentMessage.GetFileName() != "file.pdf" {
					t.Fatal("filename lost")
				}
			case "sticker":
				if !out.StickerMessage.GetIsAnimated() {
					t.Fatal("animated flag lost")
				}
			}
		})
	}
	if _, err := forwardedMessage(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}); !errors.Is(err, ErrForwardMediaUnavailable) {
		t.Fatal("placeholder media forwarded")
	}
	if _, err := forwardedMessage(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}); !errors.Is(err, ErrForwardUnsupported) {
		t.Fatal("protocol-only message forwarded")
	}
	if out, err := forwardedMessage(&waE2E.Message{Conversation: proto.String("Halo!")}); err != nil || out.GetExtendedTextMessage().GetText() != "Halo!" {
		t.Fatal("conversation was not forwarded")
	}
	viewOnce := &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: rvoSource("image")}}
	if out, err := forwardedMessage(viewOnce); err != nil || !message.IsViewOnceMessage(out) {
		t.Fatal("view-once flag was silently removed")
	}
}
