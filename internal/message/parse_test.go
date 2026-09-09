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
