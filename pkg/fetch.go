package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
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

type fetchResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

func fetchHTTPClient(c *clients.Client) *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func safeFetchHTTPClient(base *http.Client, resolver fetchResolver) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}

	client := *base
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if baseTransport, ok := transport.(*http.Transport); ok {
		tr := baseTransport.Clone()
		originalDialContext := tr.DialContext
		if originalDialContext == nil {
			originalDialContext = (&net.Dialer{}).DialContext
		}
		tr.Proxy = nil
		tr.DialContext = pinnedFetchDialContext(resolver, originalDialContext)
		client.Transport = tr
	}

	previousCheckRedirect := base.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := validateFetchURL(req.Context(), req.URL, resolver); err != nil {
			return fmt.Errorf("redirect ditolak: %w", err)
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("terlalu banyak redirect")
		}
		return nil
	}
	return &client
}

func pinnedFetchDialContext(resolver fetchResolver, dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("alamat tujuan tidak valid: %w", err)
		}
		addrs, err := resolveFetchHost(ctx, host, resolver)
		if err != nil {
			return nil, err
		}

		var lastErr error
		for _, addr := range addrs {
			conn, dialErr := dial(ctx, network, net.JoinHostPort(addr.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, fmt.Errorf("gagal terhubung ke tujuan tervalidasi: %w", lastErr)
	}
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

func validateFetchURL(ctx context.Context, u *url.URL, resolver fetchResolver) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("URL tidak valid")
	}
	_, err := resolveFetchHost(ctx, u.Hostname(), resolver)
	return err
}

func resolveFetchHost(ctx context.Context, rawHost string, resolver fetchResolver) ([]netip.Addr, error) {
	host := strings.ToLower(strings.TrimSuffix(rawHost, "."))
	if host == "" {
		return nil, fmt.Errorf("host kosong")
	}
	if isBlockedFetchHostname(host) {
		return nil, fmt.Errorf("host tidak diizinkan")
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		if isBlockedFetchIP(addr) {
			return nil, fmt.Errorf("alamat IP tidak diizinkan")
		}
		return []netip.Addr{addr.Unmap()}, nil
	}
	if resolver == nil {
		return nil, fmt.Errorf("resolver tidak tersedia")
	}

	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("host tidak dapat di-resolve: %w", err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host tidak memiliki alamat IP")
	}
	for i, addr := range addrs {
		addr = addr.Unmap()
		if isBlockedFetchIP(addr) {
			return nil, fmt.Errorf("host mengarah ke alamat IP yang tidak diizinkan")
		}
		addrs[i] = addr
	}
	return addrs, nil
}

func isBlockedFetchHostname(host string) bool {
	blocked := []string{
		"localhost",
		"metadata",
		"metadata.internal",
		"metadata.google.internal",
		"metadata.goog",
		"metadata.azure.internal",
		"metadata.aws.internal",
		"instance-data",
		"instance-data.ec2.internal",
	}
	for _, suffix := range blocked {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func isBlockedFetchIP(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}

	blockedPrefixes := [...]netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"), // shared address space, including Tailscale
		netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
		netip.MustParsePrefix("198.18.0.0/15"), // benchmark networks
		netip.MustParsePrefix("240.0.0.0/4"),   // reserved IPv4
		netip.MustParsePrefix("2001:db8::/32"), // documentation IPv6
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}

	// Alibaba Cloud exposes instance metadata on this otherwise public-looking IP.
	return addr == netip.MustParseAddr("100.100.100.200")
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

	if err := validateFetchURL(fetchCtx, u, net.DefaultResolver); err != nil {
		m.Reply(ctx, "Tujuan URL tidak diizinkan atau tidak dapat diverifikasi, yaa.")
		return
	}
	httpClient := safeFetchHTTPClient(fetchHTTPClient(client), net.DefaultResolver)

	if req, err := newFetchRequest(fetchCtx, http.MethodHead, u); err == nil {
		if resp, err := httpClient.Do(req); err == nil && resp != nil {
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

	resp, err := httpClient.Do(req)
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
