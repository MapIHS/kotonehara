package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreatePlayerUsesAPIOrigin(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/player/jobs" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["query"] != "artist & title" || body["baseUrl"] != server.URL {
			t.Errorf("incorrect query or origin: %v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": strings.Repeat("a", 48), "state": "ready", "result": map[string]any{"kind": "player", "player": PlayerSession{
			ID: "session", Title: "Title", HTML: "<p>Player</p>", Size: 1024,
			WSURL: "wss://api.example.com/ws/player/session", PlayerURL: "https://api.example.com/player/session",
			ExpiresAt: time.Now().Add(time.Minute),
		}}}})
	}))
	defer server.Close()
	session, err := New(server.URL, time.Second).CreatePlayer(context.Background(), "artist & title")
	if err != nil || session.Title != "Title" {
		t.Fatalf("session=%v error=%v", session, err)
	}
}

func TestCreatePlayerErrorAndMissingSession(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"busy", 503, `{"message":"Player sedang penuh"}`, "Player sedang penuh"},
		{"missing", 202, `{"status":"success","data":{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","state":"ready","result":{"kind":"player"}}}`, "tidak valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := New(server.URL, time.Second).CreatePlayer(context.Background(), "title")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
