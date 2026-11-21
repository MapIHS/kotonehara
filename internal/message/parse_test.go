package message

import (
	"context"
	"testing"

	"github.com/MapIHS/kotonehara/internal/identity"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestOwnerMatchingDoesNotCrossPNAndLIDNamespaces(t *testing.T) {
	parser := NewParser(nil, config.Config{Owners: []string{"628123456789"}})
	event := textEvent(
		types.NewJID("628123456789", types.HiddenUserServer),
		types.EmptyJID,
	)

	if parsed := parser.Parse(context.Background(), event); parsed.IsOwner {
		t.Fatal("LID with the same user component as the owner PN must not be owner")
	}
}

func TestOwnerMatchingUsesVerifiedSenderAlternate(t *testing.T) {
	pn := types.NewJID("628123456789", types.DefaultUserServer)
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	parser := NewParser(nil, config.Config{Owners: []string{pn.User}})
	parser.ResolveIdentity = func(ctx context.Context, primary, alternate types.JID) (identity.Identity, error) {
		return identity.New(primary, alternate), nil
	}

	if parsed := parser.Parse(context.Background(), textEvent(lid, pn)); !parsed.IsOwner {
		t.Fatal("owner PN supplied as SenderAlt must match the LID sender")
	}
}

func TestOwnerMatchingNormalizesDeviceJID(t *testing.T) {
	owner := types.NewJID("628123456789", types.DefaultUserServer)
	deviceSender := owner
	deviceSender.Device = 4
	parser := NewParser(nil, config.Config{Owners: []string{owner.User}})

	if parsed := parser.Parse(context.Background(), textEvent(deviceSender, types.EmptyJID)); !parsed.IsOwner {
		t.Fatal("device-qualified owner PN must match its normalized PN")
	}
}

func textEvent(sender, senderAlt types.JID) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{MessageSource: types.MessageSource{
			Chat:      sender.ToNonAD(),
			Sender:    sender,
			SenderAlt: senderAlt,
		}},
		Message: &waE2E.Message{Conversation: proto.String("hello")},
	}
}

func TestQuotedViewOnceWrapperIsUnwrappedWithoutStore(t *testing.T) {
	e := textEvent(types.NewJID("123", types.DefaultUserServer), types.EmptyJID)
	e.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(".toimg"), ContextInfo: &waE2E.ContextInfo{QuotedMessage: &waE2E.Message{ViewOnceMessageV2Extension: &waE2E.FutureProofMessage{Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{DirectPath: proto.String("/quoted")}}}}}}}
	m := NewParser(nil, config.Config{Prefix: "."}).Parse(context.Background(), e)
	if m.Media == nil || m.Media.GetDirectPath() != "/quoted" || !m.IsQuotedImage || !m.IsQuotedViewOnce {
		t.Fatal("wrapped quote lost")
	}
}

func TestQuotedAlbumParentExposesAlbumIDWithoutMedia(t *testing.T) {
	e := textEvent(types.NewJID("628123456789", types.DefaultUserServer), types.EmptyJID)
	e.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String(".s"),
		ContextInfo: &waE2E.ContextInfo{
			StanzaID:      proto.String("ALBUM1"),
			QuotedMessage: &waE2E.Message{AlbumMessage: &waE2E.AlbumMessage{ExpectedImageCount: proto.Uint32(2)}},
		},
	}}
	m := NewParser(nil, config.Config{Prefix: "."}).Parse(context.Background(), e)
	if m.QuotedAlbumID != "ALBUM1" || m.Media != nil {
		t.Fatalf("album id=%q media=%v", m.QuotedAlbumID, m.Media)
	}

	e.Message.ExtendedTextMessage.ContextInfo.QuotedMessage = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}
	if m := NewParser(nil, config.Config{Prefix: "."}).Parse(context.Background(), e); m.QuotedAlbumID != "" {
		t.Fatalf("plain image must not set album id: %q", m.QuotedAlbumID)
	}
}
