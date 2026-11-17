package handlers

import (
	"context"
	"fmt"
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

// trustedSourceVariant is one candidate format for the rich HTML card's
// trusted_sources field. The field is not documented; the playws card fails
// with WS_POLICY on some WhatsApp builds, so these are compared on a device.
type trustedSourceVariant struct {
	Label   string
	Sources func(ws *url.URL) []string
}

func trustedSourceVariants() []trustedSourceVariant {
	httpsOrigin := func(ws *url.URL) string {
		scheme := "https"
		if ws.Scheme == "ws" {
			scheme = "http"
		}
		return scheme + "://" + ws.Host
	}
	parentDomain := func(ws *url.URL) string {
		parts := strings.Split(ws.Hostname(), ".")
		if len(parts) <= 2 {
			return ws.Hostname()
		}
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return []trustedSourceVariant{
		{"A host (sekarang)", func(ws *url.URL) []string { return []string{ws.Hostname()} }},
		{"B origin https", func(ws *url.URL) []string { return []string{httpsOrigin(ws)} }},
		{"C origin wss", func(ws *url.URL) []string { return []string{ws.Scheme + "://" + ws.Host} }},
		{"D https + wss + host", func(ws *url.URL) []string {
			return []string{httpsOrigin(ws), ws.Scheme + "://" + ws.Host, ws.Hostname()}
		}},
		{"E domain induk", func(ws *url.URL) []string { return []string{parentDomain(ws)} }},
		{"F kosong", func(*url.URL) []string { return nil }},
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:        "playwstest",
		Description: "Kirim kartu playws dengan beberapa format trusted_sources untuk diuji di HP",
		Tags:        "owner",
		IsPrefix:    true,
		IsOwner:     true,
		IsQuery:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			ap := api.Shared(cfg.BASEApiURL, 10*time.Minute)
			handlePlayWSSourceTest(ctx, m, ap.CreatePlayer, client.WA.SendMessage)
		},
	})
}

func handlePlayWSSourceTest(ctx context.Context, m *message.Message,
	create func(context.Context, string) (*api.PlayerSession, error),
	send func(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error),
) {
	_, _ = m.Reply(ctx, "🧪 Menyiapkan satu sesi player untuk diuji dengan beberapa format trusted_sources…")
	session, err := create(ctx, strings.TrimSpace(m.Query))
	if err != nil {
		_, _ = m.Reply(ctx, "Gagal menyiapkan player: "+err.Error())
		return
	}
	ws, err := url.Parse(session.WSURL)
	if err != nil || ws.Hostname() == "" {
		_, _ = m.Reply(ctx, "Alamat player tidak valid.")
		return
	}
	for _, variant := range trustedSourceVariants() {
		sources := variant.Sources(ws)
		label := fmt.Sprintf("Varian %s\ntrusted_sources = %q\nTekan Putar pada kartu di bawah, lalu catat kode yang muncul.", variant.Label, sources)
		_, _ = m.Reply(ctx, label)
		out, err := clients.BuildRichHTML(session.HTML, sources...)
		if err == nil {
			_, err = send(ctx, m.From, out)
		}
		if err != nil {
			_, _ = m.Reply(ctx, "Varian "+variant.Label+" gagal dikirim: "+err.Error())
		}
		// Keep labels and cards in order on the receiving phone.
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
	_, _ = m.Reply(ctx, "Selesai. Kirim screenshot hasil tiap varian. Satu sesi melayani maksimal 4 koneksi bersamaan, jadi tekan Putar satu per satu.")
}
