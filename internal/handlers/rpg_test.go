package handlers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/identity"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/rpg"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	_ "modernc.org/sqlite"
)

func TestRPGPrivateLinkAndSharedGacha(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "rpg.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	s, err := rpg.New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	sender := types.NewJID("111", types.HiddenUserServer)
	group := types.NewJID("12345", types.GroupServer)
	replies := []string{}
	privateText := ""
	var privateTo types.JID
	m := &message.Message{From: group, Sender: sender, Identity: identity.New(sender, types.EmptyJID), PushName: "Ihsan", IsGroup: true, Query: "mulai", StanzaID: "message-1", Reply: func(_ context.Context, body string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		replies = append(replies, body)
		return whatsmeow.SendResponse{}, nil
	}}
	send := func(to types.JID, body string) error { privateTo = to; privateText = body; return nil }
	handleRPG(ctx, m, s, "https://game.example.com", send)
	if privateTo != sender || !strings.Contains(privateText, "/rpg/#ticket=") {
		t.Fatal("missing private link")
	}
	for _, body := range replies {
		if strings.Contains(body, "ticket=") {
			t.Fatal("login leaked to group")
		}
	}
	p, err := s.EnsurePlayer(ctx, []string{sender.String()}, "Ihsan")
	if err != nil {
		t.Fatal(err)
	}
	m.Query = "gacha 10"
	m.StanzaID = "message-draw"
	handleRPG(ctx, m, s, "https://game.example.com", send)
	first, err := s.Snapshot(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Profile.Shards != 0 {
		t.Fatal("bot did not spend same wallet")
	}
	handleRPG(ctx, m, s, "https://game.example.com", send)
	second, err := s.Snapshot(ctx, p.ID)
	if err != nil || second.Profile.Revision != first.Profile.Revision {
		t.Fatal("replayed WhatsApp command drew twice", err)
	}
	m.Query = "profil"
	handleRPG(ctx, m, s, "https://game.example.com", send)
	if !strings.Contains(replies[len(replies)-1], "Embun: 0") {
		t.Fatal("profile not synchronized")
	}
}
