package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"go.mau.fi/whatsmeow"
)

const webSessionCookieName = "kotonehara_session"

//go:embed public/*
var webAssets embed.FS

type botStatus struct {
	mu        sync.RWMutex
	StartedAt time.Time `json:"started_at"`
	Stage     string    `json:"stage"`
	Connected bool      `json:"connected"`
	LoggedIn  bool      `json:"logged_in"`
	JID       string    `json:"jid,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	QRCode    string    `json:"qr_code,omitempty"`
}

type webAuth struct {
	username   string
	password   string
	sessionTTL time.Duration

	mu       sync.RWMutex
	sessions map[string]time.Time
}

type sessionResponse struct {
	AuthEnabled   bool   `json:"auth_enabled"`
	Authenticated bool   `json:"authenticated"`
	ExpiresAt     string `json:"expires_at,omitempty"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func newBotStatus() *botStatus {
	return &botStatus{
		StartedAt: time.Now(),
		Stage:     "starting",
	}
}

func newWebAuth(cfg config.Config) *webAuth {
	return &webAuth{
		username:   cfg.WebUsername,
		password:   cfg.WebPassword,
		sessionTTL: cfg.WebSessionTTL,
		sessions:   make(map[string]time.Time),
	}
}

func (a *webAuth) enabled() bool {
	return a.username != "" && a.password != ""
}

func (a *webAuth) authenticate(username, password string) bool {
	if !a.enabled() {
		return true
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(a.username)) != 1 {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) != 1 {
		return false
	}
	return true
}

func (a *webAuth) createSession() (string, time.Time, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(a.sessionTTL)
	a.mu.Lock()
	a.sessions[token] = expiresAt
	a.mu.Unlock()
	return token, expiresAt, nil
}

func (a *webAuth) revokeSession(token string) {
	if token == "" {
		return
	}
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func (a *webAuth) session(r *http.Request) (time.Time, bool) {
	if !a.enabled() {
		return time.Time{}, true
	}
	cookie, err := r.Cookie(webSessionCookieName)
	if err != nil {
		return time.Time{}, false
	}

	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()

	expiresAt, ok := a.sessions[cookie.Value]
	if !ok {
		return time.Time{}, false
	}
	if now.After(expiresAt) {
		delete(a.sessions, cookie.Value)
		return time.Time{}, false
	}
	return expiresAt, true
}

func (a *webAuth) isAuthenticated(r *http.Request) bool {
	_, ok := a.session(r)
	return ok
}

func (a *webAuth) setSessionCookie(w http.ResponseWriter, token string, secure bool, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     webSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
}

func (a *webAuth) clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     webSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func (s *botStatus) setStage(stage string) {
	s.mu.Lock()
	s.Stage = stage
	s.mu.Unlock()
}

func (s *botStatus) setQR(code string) {
	s.mu.Lock()
	s.QRCode = code
	s.mu.Unlock()
}

func (s *botStatus) clearQR() {
	s.mu.Lock()
	s.QRCode = ""
	s.mu.Unlock()
}

func (s *botStatus) setError(err error) {
	s.mu.Lock()
	if err != nil {
		s.LastError = err.Error()
	}
	s.mu.Unlock()
}

func (s *botStatus) updateClient(client *whatsmeow.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client == nil {
		s.Connected = false
		s.LoggedIn = false
		s.JID = ""
		return
	}
	s.Connected = client.IsConnected()
	s.LoggedIn = client.IsLoggedIn()
	s.JID = ""
	if client.Store != nil && client.Store.ID != nil {
		s.JID = client.Store.ID.String()
	}
}

func (s *botStatus) snapshot() botStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return botStatus{
		StartedAt: s.StartedAt,
		Stage:     s.Stage,
		Connected: s.Connected,
		LoggedIn:  s.LoggedIn,
		JID:       s.JID,
		LastError: s.LastError,
		QRCode:    s.QRCode,
	}
}

func startWebServer(ctx context.Context, status *botStatus, cfg config.Config) {
	port := os.Getenv("PORT")
	if port == "" {
		return
	}

	dist, err := fs.Sub(webAssets, "public")
	if err != nil {
		status.setError(err)
		log.Printf("web assets error: %v", err)
		return
	}

	auth := newWebAuth(cfg)
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, sessionResponse{
			AuthEnabled:   auth.enabled(),
			Authenticated: auth.isAuthenticated(r),
			ExpiresAt:     authExpiresAt(auth, r),
		})
	})

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method tidak didukung", http.StatusMethodNotAllowed)
			return
		}
		if !auth.enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "web login belum dikonfigurasi"})
			return
		}

		var req loginRequest
		if err := decodeLoginRequest(r, &req); err != nil {
			http.Error(w, "body login tidak valid", http.StatusBadRequest)
			return
		}
		if !auth.authenticate(strings.TrimSpace(req.Username), req.Password) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "username atau password salah"})
			return
		}

		token, expiresAt, err := auth.createSession()
		if err != nil {
			status.setError(err)
			http.Error(w, "gagal membuat sesi login", http.StatusInternalServerError)
			return
		}
		auth.setSessionCookie(w, token, r.TLS != nil, expiresAt)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"authenticated": true,
			"expires_at":    expiresAt.UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method tidak didukung", http.StatusMethodNotAllowed)
			return
		}
		if cookie, err := r.Cookie(webSessionCookieName); err == nil {
			auth.revokeSession(cookie.Value)
		}
		auth.clearSessionCookie(w, r.TLS != nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	statusHandler := func(w http.ResponseWriter, r *http.Request) {
		if !auth.isAuthenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, status.snapshot())
	}
	mux.HandleFunc("/api/status", statusHandler)
	mux.HandleFunc("/status", statusHandler)

	mux.HandleFunc("/api/qr", func(w http.ResponseWriter, r *http.Request) {
		if !auth.isAuthenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		snap := status.snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"qr_code":   snap.QRCode,
			"stage":     snap.Stage,
			"logged_in": snap.LoggedIn,
		})
	})

	fileServer := http.FileServer(http.FS(dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			http.NotFound(w, r)
			return
		}
		serveSPA(w, r, dist, fileServer)
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		status.setError(err)
		log.Printf("web server error: %v", err)
		return
	}

	go func() {
		log.Printf("web server listening on :%s", port)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			status.setError(err)
			log.Printf("web server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
}

func decodeLoginRequest(r *http.Request, req *loginRequest) error {
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") {
		return json.NewDecoder(r.Body).Decode(req)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}
	req.Username = r.FormValue("username")
	req.Password = r.FormValue("password")
	return nil
}

func authExpiresAt(auth *webAuth, r *http.Request) string {
	expiresAt, ok := auth.session(r)
	if !ok || expiresAt.IsZero() {
		return ""
	}
	return expiresAt.UTC().Format(time.RFC3339)
}

func serveSPA(w http.ResponseWriter, r *http.Request, dist fs.FS, fileServer http.Handler) {
	clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean == "." || clean == "/" {
		clean = "index.html"
	}
	if strings.HasPrefix(clean, "assets/") {
		fileServer.ServeHTTP(w, r)
		return
	}
	if clean != "index.html" {
		if _, err := fs.Stat(dist, clean); err != nil {
			clean = "index.html"
		} else {
			fileServer.ServeHTTP(w, r)
			return
		}
	}

	data, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "index tidak tersedia", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
