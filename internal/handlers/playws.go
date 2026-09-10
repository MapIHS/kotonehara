package handlers

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	commands.Register(&commands.Command{
		Name: "playws", As: []string{"playhtml"}, Tags: "downloader", IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			ap := api.Shared(cfg.BASEApiURL, 10*time.Minute)
			handlePlayWS(ctx, m, ap.CreatePlayer, client.WA.SendMessage)
		},
	})
}

func handlePlayWS(ctx context.Context, m *message.Message,
	create func(context.Context, string) (*api.PlayerSession, error),
	send func(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error),
) {
	query := strings.TrimSpace(m.Query)
	if query == "" {
		_, _ = m.Reply(ctx, "Tulis judul lagu atau link YouTube setelah playws.\nContoh: playws Coldplay Yellow")
		return
	}
	_, _ = m.Reply(ctx, "🎵 Mencari dan menyiapkan player…")
	session, err := create(ctx, query)
	if err != nil {
		_, _ = m.Reply(ctx, "Gagal menyiapkan player: "+err.Error())
		return
	}
	ws, err := url.Parse(session.WSURL)
	if err != nil || ws.Hostname() == "" {
		_, _ = m.Reply(ctx, "Alamat player tidak valid.")
		return
	}
	out, err := clients.BuildRichHTML(session.HTML, ws.Hostname())
	if err == nil {
		_, err = send(ctx, m.From, out)
	}
	if err != nil {
		_, _ = m.Reply(ctx, "Kartu player gagal dikirim: "+err.Error())
		return
	}
}
