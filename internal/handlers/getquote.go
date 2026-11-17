package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

type getQuoteClient interface {
	GetQuote(context.Context, *message.Message) (*waE2E.Message, error)
	SendForwarded(context.Context, types.JID, *waE2E.Message) (whatsmeow.SendResponse, error)
}

func getQuoteCmd(ctx context.Context, c getQuoteClient, m *message.Message) {
	if m.ContextInfo.GetStanzaID() == "" {
		m.Reply(ctx, "Balas pesan yang mengutip pesan lain, yaa.")
		return
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	quoted, err := c.GetQuote(opCtx, m)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("getquote lookup: %v", err)
		}
		m.Reply(ctx, "Pesan yang dikutip tidak ditemukan, yaa.")
		return
	}
	if _, err := c.SendForwarded(opCtx, m.From, quoted); err != nil {
		log.Printf("getquote forward: %v", err)
		if errors.Is(err, clients.ErrForwardMediaUnavailable) {
			m.Reply(ctx, "Media belum bisa diambil, yaa.")
		} else {
			m.Reply(ctx, "Pesannya belum bisa dikirim, yaa.")
		}
	}
}

func init() {
	commands.Register(&commands.Command{Name: "getquote", As: []string{"gq"}, Tags: "tools", Description: "Ambil pesan yang dikutip oleh pesan yang dibalas", IsPrefix: true, Exec: func(ctx context.Context, c *clients.Client, m *message.Message, _ config.Config) {
		getQuoteCmd(ctx, c, m)
	}})
}
