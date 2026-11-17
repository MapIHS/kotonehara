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
	Alt     bool
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

// cardVariant.Alt marks variants whose player session is created through the
// --base origin (e.g. the raw Heroku URL) instead of BASEAPI_URL.
func cardVariants() []cardVariant {
	host := func(ws *url.URL) []string { return []string{ws.Hostname()} }
	fragment := func(f, _ string, _ *url.URL) string { return f }
	noHTTPS := func(f, _ string, _ *url.URL) string {
		if clean, ok := withoutHTTPS(f); ok {
			return clean
		}
		return ""
	}
	return []cardVariant{
		{Label: "1 BASEAPI_URL, seperti sekarang", Sources: host, HTML: fragment},
		{Label: "2 --base, seperti sekarang", Sources: host, HTML: fragment, Alt: true},
		{Label: "3 --base, tanpa https", Sources: host, HTML: noHTTPS, Alt: true},
	}
}

// parsePlayWSTestQuery splits "--base <origin> <query>" (flag optional, any
// position). The origin must be a bare https origin.
func parsePlayWSTestQuery(raw string) (base, query string, err error) {
	fields := strings.Fields(raw)
	rest := fields[:0:0]
	for i := 0; i < len(fields); i++ {
		value, isFlag := "", false
		switch {
		case fields[i] == "--base":
			if i+1 >= len(fields) {
				return "", "", fmt.Errorf("--base butuh URL")
			}
			value, isFlag = fields[i+1], true
			i++
		case strings.HasPrefix(fields[i], "--base="):
			value, isFlag = strings.TrimPrefix(fields[i], "--base="), true
		}
		if !isFlag {
			rest = append(rest, fields[i])
			continue
		}
		u, perr := url.Parse(value)
		if perr != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
			return "", "", fmt.Errorf("--base harus origin https tanpa path, contoh https://contoh.herokuapp.com")
		}
		base = "https://" + u.Host
	}
	return base, strings.Join(rest, " "), nil
}

func init() {
	commands.Register(&commands.Command{
		Name:        "playwstest",
		Description: "Uji kartu playws di HP. Opsional: --base <origin https> untuk membuat sesi lewat alamat lain",
		Tags:        "owner",
		IsPrefix:    true,
		IsOwner:     true,
		IsQuery:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			base, query, err := parsePlayWSTestQuery(m.Query)
			if err != nil {
				_, _ = m.Reply(ctx, err.Error())
				return
			}
			createAlt := (func(context.Context, string) (*api.PlayerSession, error))(nil)
			if base != "" {
				createAlt = api.Shared(base, 10*time.Minute).CreatePlayer
			}
			handlePlayWSSourceTest(ctx, m, query, api.Shared(cfg.BASEApiURL, 10*time.Minute).CreatePlayer, createAlt, client.WA.SendMessage)
		},
	})
}

func handlePlayWSSourceTest(ctx context.Context, m *message.Message, query string,
	create, createAlt func(context.Context, string) (*api.PlayerSession, error),
	send func(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error),
) {
	if strings.TrimSpace(query) == "" {
		_, _ = m.Reply(ctx, "Tulis judul lagu. Contoh: .playwstest --base https://contoh.herokuapp.com faded")
		return
	}
	_, _ = m.Reply(ctx, "🧪 Menyiapkan sesi player untuk diuji dalam beberapa bentuk kartu…")
	type prepared struct {
		session *api.PlayerSession
		ws      *url.URL
		err     error
	}
	cache := map[bool]*prepared{}
	get := func(alt bool) *prepared {
		if p, ok := cache[alt]; ok {
			return p
		}
		p := &prepared{}
		fn := create
		if alt {
			fn = createAlt
		}
		if fn == nil {
			p.err = fmt.Errorf("dilewati: tambahkan --base <origin https> untuk varian ini")
		} else if p.session, p.err = fn(ctx, query); p.err == nil {
			if p.ws, p.err = url.Parse(p.session.WSURL); p.err == nil && p.ws.Hostname() == "" {
				p.err = fmt.Errorf("alamat player tidak valid")
			}
		}
		cache[alt] = p
		return p
	}
	for _, variant := range cardVariants() {
		p := get(variant.Alt)
		if p.err != nil {
			_, _ = m.Reply(ctx, "Varian "+variant.Label+": "+p.err.Error())
			continue
		}
		sources := variant.Sources(p.ws)
		payload := variant.HTML(p.session.HTML, p.session.Title, p.ws)
		if payload == "" {
			_, _ = m.Reply(ctx, "Varian "+variant.Label+" dilewati: bentuk kartu dari Hararest tidak dikenali.")
			continue
		}
		label := fmt.Sprintf("Varian %s\nws = %s\ntrusted_sources = %q\nTekan Putar pada kartu di bawah, lalu catat kode yang muncul.", variant.Label, p.ws.Scheme+"://"+p.ws.Host, sources)
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
