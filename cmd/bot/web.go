package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"go.mau.fi/whatsmeow"
)

const webSessionCookieName = "kotonehara_session"

type botStatus struct {
	mu        sync.RWMutex
	StartedAt time.Time `json:"started_at"`
	Stage     string    `json:"stage"`
	Connected bool      `json:"connected"`
	LoggedIn  bool      `json:"logged_in"`
	JID       string    `json:"jid,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

type webAuth struct {
	username   string
	password   string
	sessionTTL time.Duration

	mu       sync.RWMutex
	sessions map[string]time.Time
}

type webPageData struct {
	Status      botStatus
	Error       string
	Next        string
	AuthEnabled bool
}

var (
	dashboardTemplate = template.Must(template.New("dashboard").Funcs(template.FuncMap{
		"statusClass": statusClass,
		"uptime": func(t time.Time) string {
			return time.Since(t).Round(time.Second).String()
		},
	}).Parse(`<!doctype html>
<html lang="id">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Kotonehara</title>
  <style>
    :root{color-scheme:light dark}
    body{font-family:system-ui,sans-serif;max-width:840px;margin:48px auto;padding:0 16px;line-height:1.6}
    .card{border:1px solid #d0d7de;border-radius:16px;padding:24px;margin-bottom:16px;background:rgba(127,127,127,.06)}
    code{background:rgba(127,127,127,.14);padding:2px 6px;border-radius:6px}
    .ok{color:#17803d}.bad{color:#b42318}.muted{opacity:.75}
    .row{display:flex;gap:12px;flex-wrap:wrap;align-items:center}
    button,a.btn,input{font:inherit}
    button,.btn{border:0;border-radius:10px;padding:10px 14px;text-decoration:none;display:inline-block;cursor:pointer}
    .btn-primary{background:#2563eb;color:#fff}
    .btn-secondary{background:rgba(127,127,127,.14);color:inherit}
    ul{padding-left:20px}
  </style>
</head>
<body>
  <div class="card">
    <div class="row">
      <h1 style="margin:0">Kotonehara</h1>
      {{if .AuthEnabled}}<form method="post" action="/logout" style="margin:0"><button class="btn btn-secondary" type="submit">Logout</button></form>{{end}}
    </div>
    <p class="muted">WhatsApp bot status page.</p>
    <ul>
      <li>Stage: <code>{{.Status.Stage}}</code></li>
      <li>Connected: <strong class="{{statusClass .Status.Connected}}">{{.Status.Connected}}</strong></li>
      <li>Logged in: <strong class="{{statusClass .Status.LoggedIn}}">{{.Status.LoggedIn}}</strong></li>
      <li>JID: <code>{{if .Status.JID}}{{.Status.JID}}{{else}}-{{end}}</code></li>
      <li>Uptime: <code>{{uptime .Status.StartedAt}}</code></li>
      {{if .Status.LastError}}<li>Last error: <code>{{.Status.LastError}}</code></li>{{end}}
    </ul>
    <div class="row">
      <a class="btn btn-secondary" href="/status">JSON status</a>
      <a class="btn btn-secondary" href="/healthz">Health check</a>
    </div>
  </div>
</body>
</html>`))

	loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="id">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Login Kotonehara</title>
  <style>
    :root{color-scheme:light dark}
    body{font-family:system-ui,sans-serif;max-width:480px;margin:72px auto;padding:0 16px;line-height:1.6}
    .card{border:1px solid #d0d7de;border-radius:16px;padding:24px;background:rgba(127,127,127,.06)}
    label{display:block;margin-top:12px;font-weight:600}
    input{width:100%;box-sizing:border-box;padding:10px 12px;border-radius:10px;border:1px solid #d0d7de;font:inherit}
    button{margin-top:16px;border:0;border-radius:10px;padding:10px 14px;background:#2563eb;color:#fff;font:inherit;cursor:pointer}
    .error{color:#b42318}
    .muted{opacity:.75}
  </style>
</head>
<body>
  <div class="card">
    <h1 style="margin-top:0">Kotonehara Login</h1>
    <p class="muted">Masuk dulu untuk melihat dashboard bot.</p>
    {{if .Error}}<p class="error">{{.Error}}</p>{{end}}
    <form method="post" action="/login">
      <input type="hidden" name="next" value="{{.Next}}">
      <label for="username">Username</label>
      <input id="username" name="username" autocomplete="username" required>
      <label for="password">Password</label>
      <input id="password" name="password" type="password" autocomplete="current-password" required>
      <button type="submit">Login</button>
    </form>
  </div>
</body>
</html>`))
)

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

func (a *webAuth) createSession() (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.sessions[token] = time.Now().Add(a.sessionTTL)
	a.mu.Unlock()
	return token, nil
}

func (a *webAuth) revokeSession(token string) {
	if token == "" {
		return
	}
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func (a *webAuth) isAuthenticated(r *http.Request) bool {
	if !a.enabled() {
		return true
	}
	cookie, err := r.Cookie(webSessionCookieName)
	if err != nil {
		return false
	}

	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()

	expiresAt, ok := a.sessions[cookie.Value]
	if !ok {
		return false
	}
	if now.After(expiresAt) {
		delete(a.sessions, cookie.Value)
		return false
	}
	return true
}

func (a *webAuth) setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     webSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Now().Add(a.sessionTTL),
		MaxAge:   int(a.sessionTTL.Seconds()),
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
	}
}

func startWebServer(ctx context.Context, status *botStatus, cfg config.Config) {
	port := os.Getenv("PORT")
	if port == "" {
		return
	}

	auth := newWebAuth(cfg)
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if !auth.enabled() {
			http.Error(w, "web login belum dikonfigurasi", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if auth.isAuthenticated(r) {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			renderLogin(w, "Login dulu, yaa.", safeNext(r.URL.Query().Get("next")))
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "form login tidak valid", http.StatusBadRequest)
				return
			}
			next := safeNext(r.FormValue("next"))
			username := strings.TrimSpace(r.FormValue("username"))
			password := r.FormValue("password")
			if !auth.authenticate(username, password) {
				renderLogin(w, "Username atau password salah.", next)
				return
			}

			token, err := auth.createSession()
			if err != nil {
				status.setError(err)
				http.Error(w, "gagal membuat sesi login", http.StatusInternalServerError)
				return
			}
			auth.setSessionCookie(w, token, r.TLS != nil)
			http.Redirect(w, r, next, http.StatusSeeOther)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method tidak didukung", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method tidak didukung", http.StatusMethodNotAllowed)
			return
		}
		if cookie, err := r.Cookie(webSessionCookieName); err == nil {
			auth.revokeSession(cookie.Value)
		}
		auth.clearSessionCookie(w, r.TLS != nil)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if !auth.isAuthenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status.snapshot())
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !auth.isAuthenticated(r) {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := webPageData{
			Status:      status.snapshot(),
			AuthEnabled: auth.enabled(),
		}
		if err := dashboardTemplate.Execute(w, data); err != nil {
			status.setError(err)
			http.Error(w, "gagal render dashboard", http.StatusInternalServerError)
		}
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
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
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

func renderLogin(w http.ResponseWriter, errMsg, next string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := webPageData{
		Error:       errMsg,
		Next:        next,
		AuthEnabled: true,
	}
	if err := loginTemplate.Execute(w, data); err != nil {
		http.Error(w, "gagal render login", http.StatusInternalServerError)
	}
}

func safeNext(next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return "/"
	}
	if strings.HasPrefix(next, "//") {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil || u.IsAbs() || !strings.HasPrefix(next, "/") {
		return "/"
	}
	return next
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func statusClass(ok bool) string {
	if ok {
		return "ok"
	}
	return "bad"
}
