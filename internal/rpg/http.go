package rpg

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type HTTPConfig struct {
	PublicURL     string
	GatewaySecret string
	ListenAddr    string
}

func (c HTTPConfig) Validate() error {
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("RPG_PUBLIC_URL must be an HTTP(S) origin without path/query/credentials")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && allowHTTPOrigin(u.Hostname())) {
		return fmt.Errorf("RPG_PUBLIC_URL requires HTTPS (HTTP allowed only on loopback or a Tailscale IP)")
	}
	if len(c.GatewaySecret) < 32 || strings.ContainsAny(c.GatewaySecret, "\r\n\t ") {
		return fmt.Errorf("RPG_GATEWAY_SECRET must contain at least 32 characters without spaces")
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("RPG_LISTEN_ADDR is required")
	}
	return nil
}

// HTTP on a configured Tailscale address relies on the tailnet transport.
// Do not extend this exception to public hosts or arbitrary private networks.
func allowHTTPOrigin(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return ip.IsLoopback() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip) ||
		netip.MustParsePrefix("fd7a:115c:a1e0::/48").Contains(ip)
}

var tokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type HTTPHandler struct {
	service    *Service
	cfg        HTTPConfig
	mux        *http.ServeMux
	cookieName string
	secure     bool
}

func NewHTTPHandler(s *Service, c HTTPConfig) (http.Handler, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	origin, _ := url.Parse(c.PublicURL)
	origin.Scheme = strings.ToLower(origin.Scheme)
	origin.Host = strings.ToLower(origin.Host)
	if (origin.Scheme == "https" && origin.Port() == "443") || (origin.Scheme == "http" && origin.Port() == "80") {
		host := origin.Hostname()
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		origin.Host = host
	}
	origin.Path = ""
	c.PublicURL = origin.String()
	h := &HTTPHandler{service: s, cfg: c, mux: http.NewServeMux(), cookieName: "__Secure-hara_rpg", secure: strings.HasPrefix(c.PublicURL, "https://")}
	if !h.secure {
		h.cookieName = "hara_rpg_local"
	}
	h.mux.HandleFunc("POST /rpg/api/session/exchange", h.exchange)
	h.mux.HandleFunc("POST /rpg/api/session/logout", h.authorized(h.logout))
	h.mux.HandleFunc("GET /rpg/api/profile", h.authorized(func(w http.ResponseWriter, r *http.Request, player, session string) error {
		snapshot, err := s.Snapshot(r.Context(), player)
		if err != nil {
			return err
		}
		writeJSON(w, 200, map[string]any{"profile": snapshot.Profile, "battle": snapshot.Battle, "csrf": h.csrf(session)})
		return nil
	}))
	h.mux.HandleFunc("GET /rpg/api/catalog", h.authorized(func(w http.ResponseWriter, r *http.Request, _, _ string) error {
		writeJSON(w, 200, s.Catalog())
		return nil
	}))
	h.mux.HandleFunc("POST /rpg/api/battles", h.authorized(func(w http.ResponseWriter, r *http.Request, player, _ string) error {
		var a struct {
			RequestID string `json:"request_id"`
			Stage     int    `json:"stage"`
		}
		if err := decode(w, r, &a); err != nil {
			return err
		}
		out, err := s.StartBattle(r.Context(), player, a.RequestID, a.Stage)
		return respond(w, out, err)
	}))
	h.mux.HandleFunc("POST /rpg/api/battles/{id}/actions", h.authorized(func(w http.ResponseWriter, r *http.Request, player, _ string) error {
		var a Action
		if err := decode(w, r, &a); err != nil {
			return err
		}
		if !tokenPattern.MatchString(r.PathValue("id")) {
			return fail(404, "battle_missing", "Battle tidak ditemukan.")
		}
		out, err := s.Act(r.Context(), player, r.PathValue("id"), a)
		return respond(w, out, err)
	}))
	h.mux.HandleFunc("POST /rpg/api/battles/{id}/retreat", h.authorized(func(w http.ResponseWriter, r *http.Request, player, _ string) error {
		var a struct {
			RequestID string `json:"request_id"`
			Revision  int64  `json:"expected_revision"`
		}
		if err := decode(w, r, &a); err != nil {
			return err
		}
		if !tokenPattern.MatchString(r.PathValue("id")) {
			return fail(404, "battle_missing", "Battle tidak ditemukan.")
		}
		out, err := s.Retreat(r.Context(), player, r.PathValue("id"), a.RequestID, a.Revision)
		return respond(w, out, err)
	}))
	h.mux.HandleFunc("PUT /rpg/api/party", h.authorized(func(w http.ResponseWriter, r *http.Request, player, _ string) error {
		var a PartyRequest
		if err := decode(w, r, &a); err != nil {
			return err
		}
		out, err := s.SetParty(r.Context(), player, a)
		return respond(w, out, err)
	}))
	h.mux.HandleFunc("POST /rpg/api/gacha/pulls", h.authorized(func(w http.ResponseWriter, r *http.Request, player, _ string) error {
		var a struct {
			RequestID string `json:"request_id"`
			Banner    string `json:"banner_id"`
			Count     int    `json:"count"`
		}
		if err := decode(w, r, &a); err != nil {
			return err
		}
		out, err := s.Summon(r.Context(), player, a.RequestID, a.Banner, a.Count)
		return respond(w, out, err)
	}))
	h.mux.HandleFunc("GET /rpg/api/gacha/history", h.authorized(func(w http.ResponseWriter, r *http.Request, player, _ string) error {
		out, err := s.History(r.Context(), player)
		if err != nil {
			return err
		}
		writeJSON(w, 200, map[string]any{"history": out})
		return nil
	}))
	return h, nil
}
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-RPG-Gateway")), []byte(h.cfg.GatewaySecret)) != 1 {
		writeError(w, fail(401, "gateway", "Gateway tidak diizinkan."))
		return
	}
	if r.Method != "GET" && r.Header.Get("Origin") != h.cfg.PublicURL {
		writeError(w, fail(403, "origin", "Origin tidak diizinkan."))
		return
	}
	h.mux.ServeHTTP(w, r)
}
func (h *HTTPHandler) csrf(session string) string {
	mac := hmac.New(sha256.New, []byte(h.cfg.GatewaySecret))
	mac.Write([]byte("rpg-csrf:" + session))
	return hex.EncodeToString(mac.Sum(nil))
}
func (h *HTTPHandler) authorized(fn func(http.ResponseWriter, *http.Request, string, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(h.cookieName)
		if err != nil {
			writeError(w, fail(401, "session_required", "Masuk melalui link .rpg lanjut dari bot."))
			return
		}
		player, err := h.service.SessionPlayer(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, err)
			return
		}
		if r.Method != "GET" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(h.csrf(cookie.Value))) != 1 {
			writeError(w, fail(403, "csrf", "Token sesi tidak cocok. Muat ulang halaman."))
			return
		}
		if err = fn(w, r, player, cookie.Value); err != nil {
			writeError(w, err)
		}
	}
}
func (h *HTTPHandler) setCookie(w http.ResponseWriter, session string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: h.cookieName, Value: session, Path: "/rpg/api", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func (h *HTTPHandler) exchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := decode(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	session, err := h.service.ExchangeTicket(r.Context(), body.Ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	h.setCookie(w, session, 7*24*60*60)
	writeJSON(w, 200, map[string]string{"csrf": h.csrf(session)})
}
func (h *HTTPHandler) logout(w http.ResponseWriter, r *http.Request, _, session string) error {
	if err := h.service.Logout(r.Context(), session); err != nil {
		return err
	}
	h.setCookie(w, "", -1)
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fail(415, "content_type", "Gunakan application/json.")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return fail(400, "invalid_body", "Body JSON tidak valid.")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fail(400, "invalid_body", "Body harus berisi satu objek JSON.")
	}
	return nil
}
func respond(w http.ResponseWriter, out Snapshot, err error) error {
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	var e *Error
	if !errors.As(err, &e) {
		log.Printf("RPG request failed: %v", err)
		if strings.Contains(err.Error(), "locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
			e = &Error{503, "busy", "Database sedang sibuk. Coba lagi dengan request yang sama."}
		} else {
			e = &Error{500, "internal", "Permainan belum bisa diproses. Coba lagi sebentar."}
		}
	}
	writeJSON(w, e.Status, map[string]any{"error": map[string]string{"code": e.Code, "message": e.Message}})
}

// StartHTTP binds synchronously, so an invalid port fails before WhatsApp login.
func StartHTTP(s *Service, c HTTPConfig) (*http.Server, error) {
	h, err := NewHTTPHandler(s, c)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", c.ListenAddr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("RPG HTTP stopped: %v", err)
		}
	}()
	return server, nil
}
func ShutdownHTTP(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
