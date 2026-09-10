package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func TestPlayWSHandler(t *testing.T) {
	for _, tc := range []struct {
		name, query            string
		createErr, sendErr     error
		wantCreates, wantSends int
		wantReply              string
	}{
		{"missing query", "", nil, nil, 0, 0, "judul lagu"},
		{"success", "  title  ", nil, nil, 1, 1, "Mencari dan menyiapkan player"},
		{"prepare fails", "title", errors.New("busy"), nil, 1, 0, "Gagal menyiapkan player: busy"},
		{"send fails", "title", nil, errors.New("offline"), 1, 1, "Kartu player gagal dikirim: offline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			chat := types.NewJID("123", types.GroupServer)
			var replies []string
			m := &message.Message{From: chat, Query: tc.query}
			m.Reply = func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				replies = append(replies, text)
				return whatsmeow.SendResponse{}, nil
			}
			creates, sends := 0, 0
			handlePlayWS(ctx, m, func(gotCtx context.Context, query string) (*api.PlayerSession, error) {
				creates++
				if gotCtx != ctx || query != "title" {
					t.Fatal("query/context changed")
				}
				return &api.PlayerSession{Title: "Title", HTML: "<p>player</p>", WSURL: "wss://api.example.com/ws/player/session", PlayerURL: "https://api.example.com/player/session"}, tc.createErr
			}, func(gotCtx context.Context, to types.JID, msg *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				sends++
				if gotCtx != ctx || to != chat {
					t.Fatal("wrong destination/context")
				}
				var data map[string]any
				if err := json.Unmarshal(msg.GetBotForwardedMessage().GetMessage().GetRichResponseMessage().GetUnifiedResponse().GetData(), &data); err != nil {
					t.Fatal(err)
				}
				primitive := data["sections"].([]any)[0].(map[string]any)["view_model"].(map[string]any)["primitive"].(map[string]any)
				if primitive["payload"] != "<p>player</p>" || primitive["trusted_sources"].([]any)[0] != "api.example.com" {
					t.Fatal("wrong player or source host")
				}
				return whatsmeow.SendResponse{}, tc.sendErr
			})
			if creates != tc.wantCreates || sends != tc.wantSends {
				t.Fatalf("creates=%d sends=%d", creates, sends)
			}
			if len(replies) == 0 || !strings.Contains(replies[len(replies)-1], tc.wantReply) {
				t.Fatalf("replies=%v", replies)
			}
		})
	}
}
