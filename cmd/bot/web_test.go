package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/infra/config"
)

func TestSafeNext(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":            "/",
		"/":           "/",
		"/status":     "/status",
		"//evil.com":  "/",
		"http://evil": "/",
		"status":      "/",
		"/a?next=/b":  "/a?next=/b",
	}

	for input, want := range tests {
		if got := safeNext(input); got != want {
			t.Fatalf("safeNext(%q) = %q, want %q", input, got, want)
		}
	}
}

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

	token, err := auth.createSession()
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: webSessionCookieName, Value: token})
	if !auth.isAuthenticated(req) {
		t.Fatal("expected session to authenticate")
	}

	auth.revokeSession(token)
	if auth.isAuthenticated(req) {
		t.Fatal("expected revoked session to fail")
	}
}

func TestWebAuthExpiredSession(t *testing.T) {
	t.Parallel()

	auth := newWebAuth(config.Config{
		WebUsername:   "admin",
		WebPassword:   "secret",
		WebSessionTTL: time.Minute,
	})

	token, err := auth.createSession()
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	auth.mu.Lock()
	auth.sessions[token] = time.Now().Add(-time.Minute)
	auth.mu.Unlock()

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: webSessionCookieName, Value: token})
	if auth.isAuthenticated(req) {
		t.Fatal("expected expired session to fail")
	}
}
