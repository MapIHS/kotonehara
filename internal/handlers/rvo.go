package handlers

import (
	"context"
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

type rvoClient interface {
	RestoreQuotedMessage(context.Context, *message.Message) bool
	SendViewOnceAsNormal(context.Context, types.JID, *waE2E.Message, *waE2E.ContextInfo) (whatsmeow.SendResponse, error)
}

func readViewOnce(ctx context.Context, client rvoClient, m *message.Message) {
	if m.ContextInfo.GetStanzaID() == "" {
		m.Reply(ctx, "Balas gambar, video, atau voice note sekali lihat dengan .rvo, yaa.")
		return
	}
	opCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	client.RestoreQuotedMessage(opCtx, m)
	if !m.IsQuotedViewOnce || m.QuotedMsg == nil {
		m.Reply(ctx, "Media belum bisa diambil, yaa.")
		return
	}
	if _, err := client.SendViewOnceAsNormal(opCtx, m.From, m.QuotedMsg, m.ID); err != nil {
		log.Printf("rvo: %v", err)
		switch {
		case errors.Is(err, clients.ErrViewOnceTooLarge):
			m.Reply(ctx, "Media sekali lihat maksimal 64 MiB, yaa.")
		case errors.Is(err, clients.ErrViewOnceUnsupported):
			m.Reply(ctx, "RVO mendukung gambar, video, dan voice note sekali lihat, yaa.")
		default:
			m.Reply(ctx, "Media belum bisa diambil, yaa.")
		}
	}
}

func init() {
	commands.Register(&commands.Command{Name: "rvo", As: []string{"readviewonce"}, Tags: "tools", Description: "Balas gambar, video, atau voice note sekali lihat untuk mengirimnya sebagai media biasa", IsPrefix: true,
		Exec: func(ctx context.Context, c *clients.Client, m *message.Message, _ config.Config) {
			readViewOnce(ctx, c, m)
		},
	})
}
