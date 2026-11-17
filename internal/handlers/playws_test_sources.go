package handlers

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
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

// cardVariant is one candidate way to package the playws card. On some
// WhatsApp builds every network request from the card is blocked (WS_POLICY);
// trusted_sources formats alone made no difference, so the HTML document shape
// is compared too: a bare fragment versus a complete HTML document.
type cardVariant struct {
	Label   string
	Sources func(ws *url.URL) []string
	HTML    func(fragment, title string, ws *url.URL) string
}

// fullHTMLDocument wraps the card fragment in a complete document. csp, when
// set, is added as a meta Content-Security-Policy.
func fullHTMLDocument(fragment, title, csp string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="id"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	if csp != "" {
		b.WriteString(`<meta http-equiv="Content-Security-Policy" content="`)
		b.WriteString(html.EscapeString(csp))
		b.WriteString(`">`)
	}
	b.WriteString(`<title>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</title></head><body>`)
	b.WriteString(fragment)
	b.WriteString(`</body></html>`)
	return b.String()
}

var (
	playerURLField = regexp.MustCompile(`,?"playerUrl":"[^"]*"`)
	httpWord       = regexp.MustCompile(`(?i)https?`)
)

// withoutHTTPS removes every "http"/"https" occurrence from the card: the
// https player link in the embedded track data, the diagnostic HTTPS probe,
// and JavaScript comments. The WebSocket itself is wss:// and unchanged. It
// returns false when the card no longer has the expected shape.
func withoutHTTPS(card string) (string, bool) {
	card = playerURLField.ReplaceAllString(card, "")
	start := strings.Index(card, "function probe(){")
	end := strings.Index(card, "\n  function failWithProbe")
	if start >= 0 && end > start {
		card = card[:start] + "function probe(){return Promise.resolve('nonaktif');}" + card[end:]
	}
	card = strings.ReplaceAll(card, "info.https", "info.cek")
	lines := strings.Split(card, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			kept = append(kept, line)
		}
	}
	card = strings.Join(kept, "\n")
	return card, !httpWord.MatchString(card)
}

func cardVariants() []cardVariant {
	host := func(ws *url.URL) []string { return []string{ws.Hostname()} }
	fragment := func(f, _ string, _ *url.URL) string { return f }
	noHTTPS := func(f, _ string, _ *url.URL) string {
		if clean, ok := withoutHTTPS(f); ok {
			return clean
		}
		return ""
	}
	noHTTPSDocument := func(f, title string, ws *url.URL) string {
		if clean := noHTTPS(f, title, ws); clean != "" {
			return fullHTMLDocument(clean, title, "")
		}
		return ""
	}
	return []cardVariant{
		{"1 seperti sekarang", host, fragment},
		{"2 tanpa https (potongan HTML)", host, noHTTPS},
		{"3 tanpa https (dokumen lengkap)", host, noHTTPSDocument},
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:        "playwstest",
		Description: "Kirim kartu playws dalam beberapa bentuk HTML untuk diuji di HP",
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
	_, _ = m.Reply(ctx, "🧪 Menyiapkan satu sesi player untuk diuji dalam beberapa bentuk kartu…")
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
	for _, variant := range cardVariants() {
		sources := variant.Sources(ws)
		payload := variant.HTML(session.HTML, session.Title, ws)
		if payload == "" {
			_, _ = m.Reply(ctx, "Varian "+variant.Label+" dilewati: bentuk kartu dari Hararest tidak dikenali.")
			continue
		}
		label := fmt.Sprintf("Varian %s\ntrusted_sources = %q\nTekan Putar pada kartu di bawah, lalu catat kode yang muncul.", variant.Label, sources)
		_, _ = m.Reply(ctx, label)
		out, err := clients.BuildRichHTML(payload, sources...)
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
	_, _ = m.Reply(ctx, "Selesai. Kirim screenshot hasil tiap varian. Tekan Putar satu per satu.")
}
