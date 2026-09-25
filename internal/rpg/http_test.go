package rpg

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPSessionBoundaries(t *testing.T) {
	for _, origin := range []string{"https://game.example.com", "http://100.89.85.96:1338"} {
		t.Run(origin, func(t *testing.T) { testSessionBoundaries(t, origin) })
	}
}

func testSessionBoundaries(t *testing.T, origin string) {
	t.Helper()
	_, s, p := fixture(t)
	cfg := HTTPConfig{PublicURL: origin, GatewaySecret: strings.Repeat("x", 32), ListenAddr: "127.0.0.1:0"}
	handler, err := NewHTTPHandler(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, path string, body any, cookie *http.Cookie, csrf, origin, secret string) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, "https://internal"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-RPG-Gateway", secret)
		req.Header.Set("X-CSRF-Token", csrf)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	if w := send("GET", "/rpg/api/profile", nil, nil, "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := send("GET", "/rpg/api/profile", nil, nil, "", "", cfg.GatewaySecret); w.Code != 401 {
		t.Fatal(w.Code)
	}
	ticket, err := s.IssueTicket(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	w := send("POST", "/rpg/api/session/exchange", map[string]string{"ticket": ticket}, nil, "", "https://attacker.example", cfg.GatewaySecret)
	if w.Code != 403 {
		t.Fatal("foreign origin accepted", w.Code)
	}
	w = send("POST", "/rpg/api/session/exchange", map[string]string{"ticket": ticket}, nil, "", cfg.PublicURL, cfg.GatewaySecret)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal(cookies)
	}
	cookie := cookies[0]
	secure := strings.HasPrefix(origin, "https://")
	wantCookieName := "hara_rpg_local"
	if secure {
		wantCookieName = "__Secure-hara_rpg"
	}
	if cookie.Secure != secure || cookie.Name != wantCookieName || !cookie.HttpOnly || cookie.Path != "/rpg/api" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe cookie", cookie)
	}
	var exchange struct {
		CSRF string `json:"csrf"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &exchange); err != nil {
		t.Fatal(err)
	}
	w = send("POST", "/rpg/api/session/exchange", map[string]string{"ticket": ticket}, nil, "", cfg.PublicURL, cfg.GatewaySecret)
	if w.Code != 401 {
		t.Fatal("ticket replay", w.Code)
	}
	w = send("GET", "/rpg/api/profile", nil, cookie, "", "", cfg.GatewaySecret)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code)
	}
	var profile struct {
		Profile Profile `json:"profile"`
		CSRF    string  `json:"csrf"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Profile.ID != p.ID || profile.CSRF != exchange.CSRF {
		t.Fatal("profile/CSRF mismatch")
	}
	body := map[string]any{"request_id": "http-battle-start", "stage": 0}
	for _, test := range []struct{ csrf, origin string }{{"", cfg.PublicURL}, {exchange.CSRF, "https://attacker.example"}} {
		w = send("POST", "/rpg/api/battles", body, cookie, test.csrf, test.origin, cfg.GatewaySecret)
		if w.Code != 403 {
			t.Fatal("CSRF bypass", w.Code)
		}
	}
	w = send("POST", "/rpg/api/battles", body, cookie, exchange.CSRF, cfg.PublicURL, cfg.GatewaySecret)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	body["shards"] = 999999
	w = send("POST", "/rpg/api/battles", body, cookie, exchange.CSRF, cfg.PublicURL, cfg.GatewaySecret)
	if w.Code != 400 {
		t.Fatal("client balance injection", w.Code)
	}
	w = send("POST", "/rpg/api/session/logout", map[string]any{}, cookie, exchange.CSRF, cfg.PublicURL, cfg.GatewaySecret)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = send("GET", "/rpg/api/profile", nil, cookie, "", "", cfg.GatewaySecret)
	if w.Code != 401 {
		t.Fatal("revoked session accepted", w.Code)
	}
}
func TestPublicOriginConfiguration(t *testing.T) {
	for _, origin := range []string{"http://game.example.com", "http://8.8.8.8:1338", "http://192.168.1.2:1338", "http://100.63.255.255:1338", "http://100.128.0.0:1338", "http://100.89.85.96.example.com:1338", "http://[fd7a:115c:a1e1::1]:1338", "https://game.example.com/rpg", "https://user:pass@game.example.com", "https://game.example.com/?ticket=x", "javascript:alert(1)"} {
		if err := (HTTPConfig{PublicURL: origin, GatewaySecret: strings.Repeat("x", 32), ListenAddr: "127.0.0.1:0"}).Validate(); err == nil {
			t.Fatal("accepted", origin)
		}
	}
	for _, origin := range []string{"http://127.0.0.1:3000", "http://localhost:8088", "http://[::1]:1338", "http://100.89.85.96:1338", "http://100.64.0.1:1338", "http://100.127.255.254:1338", "http://[fd7a:115c:a1e0::1]:1338", "https://game.example.com"} {
		if err := (HTTPConfig{PublicURL: origin, GatewaySecret: strings.Repeat("x", 32), ListenAddr: "127.0.0.1:0"}).Validate(); err != nil {
			t.Fatal(origin, err)
		}
	}
}
