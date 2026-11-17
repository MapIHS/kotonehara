package handlers

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type getQuoteStub struct {
	lookups, sends     int
	lookupErr, sendErr error
	to                 types.JID
}

func (c *getQuoteStub) GetQuote(context.Context, *message.Message) (*waE2E.Message, error) {
	c.lookups++
	return &waE2E.Message{Conversation: proto.String("Halo!")}, c.lookupErr
}
func (c *getQuoteStub) SendForwarded(_ context.Context, to types.JID, _ *waE2E.Message) (whatsmeow.SendResponse, error) {
	c.sends++
	c.to = to
	return whatsmeow.SendResponse{}, c.sendErr
}

func TestGetQuoteHandlerValidationAndForward(t *testing.T) {
	for _, tc := range []struct {
		name               string
		reply              bool
		lookupErr, sendErr error
		want               string
		sends              int
	}{
		{name: "requires reply", want: "Balas pesan"},
		{name: "original found", reply: true, sends: 1},
		{name: "not found", reply: true, lookupErr: sql.ErrNoRows, want: "tidak ditemukan"},
		{name: "database failure", reply: true, lookupErr: errors.New("db unavailable"), want: "tidak ditemukan"},
		{name: "missing media", reply: true, sendErr: clients.ErrForwardMediaUnavailable, sends: 1, want: "Media belum bisa diambil, yaa."},
		{name: "send failure", reply: true, sendErr: errors.New("send failed"), sends: 1, want: "belum bisa dikirim"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &getQuoteStub{lookupErr: tc.lookupErr, sendErr: tc.sendErr}
			var replies []string
			m := &message.Message{From: types.NewJID("group", types.GroupServer), Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				replies = append(replies, text)
				return whatsmeow.SendResponse{}, nil
			}}
			if tc.reply {
				m.ContextInfo = &waE2E.ContextInfo{StanzaID: proto.String("parent")}
			}
			getQuoteCmd(context.Background(), c, m)
			if c.sends != tc.sends {
				t.Fatalf("sends %d want %d", c.sends, tc.sends)
			}
			if !tc.reply && c.lookups != 0 {
				t.Fatal("looked up store without reply")
			}
			if c.sends > 0 && c.to != m.From {
				t.Fatal("forward left current chat")
			}
			if tc.want != "" {
				if len(replies) != 1 || !strings.Contains(replies[0], tc.want) {
					t.Fatalf("reply: %v", replies)
				}
			} else if len(replies) > 0 {
				t.Fatalf("unexpected response: %v", replies)
			}
		})
	}
}
