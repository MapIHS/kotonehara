package rpg

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestBrowserFixture(t *testing.T) {
	dir := os.Getenv("RPG_E2E_DIR")
	if dir == "" {
		t.Skip("browser fixture is opt-in")
	}
	_, s := openTest(t, filepath.Join(dir, "game.db"))
	p, err := s.EnsurePlayer(context.Background(), []string{"100000@lid"}, "Browser tester")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := s.IssueTicket(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := HTTPConfig{PublicURL: os.Getenv("RPG_E2E_ORIGIN"), GatewaySecret: os.Getenv("RPG_E2E_SECRET"), ListenAddr: "127.0.0.1:0"}
	handler, err := NewHTTPHandler(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	data, _ := json.Marshal(map[string]string{"upstream": "http://" + listener.Addr().String(), "ticket": ticket})
	ready := filepath.Join(dir, "ready.json")
	if err = os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(ready)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
