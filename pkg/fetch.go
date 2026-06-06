package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
)

const (
	maxFetchBytes    int64 = 100 * 1024 * 1024
	maxTextReplySize       = 65536
	fetchTimeout           = 45 * time.Second
)

func fetchHTTPClient(c *clients.Client) *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func newFetchRequest(ctx context.Context, method string, u *url.URL) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; kotonehara/1.0; +https://github.com/MapIHS/kotonehara)")
	req.Header.Set("Accept", "*/*")
	return req, nil
}

func isTextLike(ct string) bool {
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		mediaType = ct
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))

	return strings.HasPrefix(mediaType, "text/") ||
		strings.Contains(mediaType, "json") ||
		strings.Contains(mediaType, "xml") ||
		mediaType == "application/javascript" ||
		mediaType == "application/x-www-form-urlencoded"
}

func mediaTypeFromContentType(ct string) string {
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(ct))
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func filenameFromURL(u *url.URL, ct string) string {
	base := path.Base(u.Path)
	if base == "." || base == "/" || base == "" {
		base = "file"
	}
	base = sanitizeFilename(base)

	mediaType := mediaTypeFromContentType(ct)
	if !strings.Contains(path.Base(base), ".") {
		if exts, _ := mime.ExtensionsByType(mediaType); len(exts) > 0 {
			base += exts[0]
		}
	}
	return base
}

func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			return '_'
		case r < 32:
			return -1
		default:
			return r
		}
	}, strings.TrimSpace(name))

	if name == "" {
		return "file"
	}
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func parseFetchURL(raw string) (*url.URL, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("scheme tidak didukung")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("host kosong")
	}
	return u, nil
}

func fetchCmd(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	_ = cfg

	u, err := parseFetchURL(m.Query)
	if err != nil {
		m.Reply(ctx, "Awali URL dengan http:// atau https://, yaa.")
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	if req, err := newFetchRequest(fetchCtx, http.MethodHead, u); err == nil {
		if resp, err := fetchHTTPClient(client).Do(req); err == nil && resp != nil {
			if resp.StatusCode < http.StatusBadRequest && tooLarge(resp.Header.Get("Content-Length")) {
				_ = resp.Body.Close()
				m.Reply(ctx, "Ukuran konten terlalu besar, yaa.")
				return
			}
			_ = resp.Body.Close()
		}
	}

	req, err := newFetchRequest(fetchCtx, http.MethodGet, u)
	if err != nil {
		m.Reply(ctx, "URL tidak valid, yaa.")
		return
	}

	resp, err := fetchHTTPClient(client).Do(req)
	if err != nil || resp == nil {
		m.Reply(ctx, "Gagal mengambil konten, yaa.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		m.Reply(ctx, fmt.Sprintf("Gagal mengambil konten (HTTP %d), yaa.", resp.StatusCode))
		return
	}
	if tooLarge(resp.Header.Get("Content-Length")) {
		m.Reply(ctx, "Ukuran konten terlalu besar, yaa.")
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil || len(body) == 0 {
		m.Reply(ctx, "Konten kosong atau gagal dibaca, yaa.")
		return
	}
	if int64(len(body)) > maxFetchBytes {
		m.Reply(ctx, "Ukuran konten terlalu besar, yaa.")
		return
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(body)
	}

	if !isTextLike(ct) {
		sendFetchedMedia(ctx, client, m, u, ct, body)
		return
	}

	m.Reply(ctx, textFetchReply(body))
}

func tooLarge(contentLength string) bool {
	if contentLength == "" {
		return false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(contentLength), 10, 64)
	return err == nil && n > maxFetchBytes
}

func sendFetchedMedia(ctx context.Context, client *clients.Client, m *message.Message, u *url.URL, ct string, body []byte) {
	mediaType := mediaTypeFromContentType(ct)

	switch {
	case strings.HasPrefix(mediaType, "image/"):
		if _, err := client.SendImage(ctx, m.From, body, "", m.ID); err != nil {
			m.Reply(ctx, "Gambar belum bisa dikirim, yaa.")
		}
	case strings.HasPrefix(mediaType, "video/"):
		if _, err := client.SendVideo(ctx, m.From, body, false, "", m.ID); err != nil {
			m.Reply(ctx, "Video belum bisa dikirim, yaa.")
		}
	case strings.HasPrefix(mediaType, "audio/"):
		if _, err := client.SendAudio(ctx, m.From, body, false, m.ID); err != nil {
			name := filenameFromURL(u, ct)
			if _, docErr := client.SendDocument(ctx, m.From, body, name, "", m.ID); docErr != nil {
				m.Reply(ctx, "Audio belum bisa dikirim, yaa.")
			}
		}
	default:
		name := filenameFromURL(u, ct)
		if _, err := client.SendDocument(ctx, m.From, body, name, "", m.ID); err != nil {
			m.Reply(ctx, "Berkas belum bisa dikirim, yaa.")
		}
	}
}

func textFetchReply(body []byte) string {
	txt := string(body)

	var any interface{}
	if json.Unmarshal(body, &any) == nil {
		if pretty, err := json.MarshalIndent(any, "", "  "); err == nil {
			txt = string(pretty)
		}
	}

	if len(txt) > maxTextReplySize {
		suffix := "\n\n..."
		txt = txt[:maxTextReplySize-len(suffix)] + suffix
	}
	return txt
}

func init() {
	commands.Register(&commands.Command{
		Name:        "fetch",
		As:          []string{"fetch", "f"},
		Tags:        "tools",
		Description: "Ambil konten dari URL http/https",
		IsQuery:     true,
		IsPrefix:    true,
		Exec:        fetchCmd,
	})
}
