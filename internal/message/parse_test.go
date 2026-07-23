package message

import (
	"testing"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestExtractQuoteContextDoesNotCopyMentions(t *testing.T) {
	sender := types.NewJID("628123456789", types.DefaultUserServer)
	message := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Sender: sender},
			ID:            "source-message-id",
		},
		Message: &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String("pesan dengan mention"),
				ContextInfo: &waE2E.ContextInfo{
					MentionedJID: []string{"628111111111@s.whatsapp.net"},
				},
			},
		},
	}

	contextInfo := extractQuoteContext(message)
	if got := contextInfo.GetMentionedJID(); len(got) != 0 {
		t.Fatalf("MentionedJID = %v, want empty", got)
	}
	if got := contextInfo.GetStanzaID(); got != message.Info.ID {
		t.Fatalf("StanzaID = %q, want %q", got, message.Info.ID)
	}
	if got := contextInfo.GetParticipant(); got != sender.String() {
		t.Fatalf("Participant = %q, want %q", got, sender.String())
	}
	if contextInfo.GetQuotedMessage() != message.Message {
		t.Fatal("QuotedMessage was not preserved")
	}
}
