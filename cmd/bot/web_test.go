package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
)

func TestWebAuthSessionLifecycle(t *testing.T) {
	t.Parallel()

	auth := newWebAuth(config.Config{
		WebUsername:   "admin",
		WebPassword:   "secret",
		WebSessionTTL: time.Minute,
	})
	if !auth.enabled() {
		t.Fatal("expected auth to be enabled")
	}
	if !auth.authenticate("admin", "secret") {
		t.Fatal("expected credentials to pass")
	}
	if auth.authenticate("admin", "wrong") {
		t.Fatal("expected wrong password to fail")
	}

	token, expiresAt, err := auth.createSession()
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: webSessionCookieName, Value: token})
	if !auth.isAuthenticated(req) {
		t.Fatal("expected session to authenticate")
	}

	auth.setSessionCookie(httptest.NewRecorder(), token, false, expiresAt)
	auth.revokeSession(token)
	if auth.isAuthenticated(req) {
		t.Fatal("expected revoked session to fail")
	}
}

func TestDecodeLoginRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"a","password":"b"}`))
	req.Header.Set("Content-Type", "application/json")

	var got loginRequest
	if err := decodeLoginRequest(req, &got); err != nil {
		t.Fatalf("decodeLoginRequest() error = %v", err)
	}
	if got.Username != "a" || got.Password != "b" {
		t.Fatalf("decodeLoginRequest() = %+v", got)
	}
}

func TestWebAuthExpiredSession(t *testing.T) {
	t.Parallel()

	auth := newWebAuth(config.Config{
		WebUsername:   "admin",
		WebPassword:   "secret",
		WebSessionTTL: time.Minute,
	})

	token, _, err := auth.createSession()
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	auth.mu.Lock()
	auth.sessions[token] = time.Now().Add(-time.Minute)
	auth.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: webSessionCookieName, Value: token})
	if auth.isAuthenticated(req) {
		t.Fatal("expected expired session to fail")
	}
}
