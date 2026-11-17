package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type rvoClientStub struct {
	restore        bool
	lookups, sends int
	err            error
}

func (c *rvoClientStub) RestoreQuotedMessage(_ context.Context, m *message.Message) bool {
	c.lookups++
	if c.restore {
		message.SetQuotedMessage(m, &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}}})
	}
	return c.restore
}
func (c *rvoClientStub) SendViewOnceAsNormal(_ context.Context, to types.JID, m *waE2E.Message, quote *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	c.sends++
	return whatsmeow.SendResponse{}, c.err
}

func TestRVOReplyValidationStoreFallbackAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		reply, quotedVO, restore bool
		err                      error
		sends                    int
		response                 string
	}{
		{name: "requires reply", response: "Balas"},
		{name: "ordinary media rejected", reply: true, response: "belum ditemukan"},
		{name: "original restored", reply: true, restore: true, sends: 1},
		{name: "complete quote without store", reply: true, quotedVO: true, sends: 1},
		{name: "size error", reply: true, restore: true, err: clients.ErrViewOnceTooLarge, sends: 1, response: "64 MiB"},
		{name: "download error", reply: true, restore: true, err: whatsmeow.ErrMediaDownloadFailedWith404, sends: 1, response: "tidak tersedia"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &rvoClientStub{restore: tc.restore, err: tc.err}
			var replies []string
			m := &message.Message{QuotedMsg: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, IsQuotedViewOnce: tc.quotedVO, Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				replies = append(replies, text)
				return whatsmeow.SendResponse{}, nil
			}}
			if tc.reply {
				m.ContextInfo = &waE2E.ContextInfo{StanzaID: proto.String("original")}
			}
			readViewOnce(context.Background(), c, m)
			if c.sends != tc.sends {
				t.Fatalf("sends=%d, want %d", c.sends, tc.sends)
			}
			if !tc.reply && c.lookups != 0 {
				t.Fatal("store lookup without reply")
			}
			if tc.response != "" {
				if len(replies) != 1 || !strings.Contains(replies[0], tc.response) {
					t.Fatalf("response: %v", replies)
				}
			} else if len(replies) != 0 {
				t.Fatalf("unexpected failure response: %v", replies)
			}
		})
	}
}
