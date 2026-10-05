package rpg

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestWebListenerWithRPGDisabled(t *testing.T) {
	server, err := StartHTTP(nil, HTTPConfig{ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer ShutdownHTTP(server)
	response, err := http.Get("http://" + server.Addr + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		Status  string `json:"status"`
		Enabled bool   `json:"rpg_enabled"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || body.Status != "ok" || body.Enabled {
		t.Fatal("unexpected readiness")
	}
	response2, err := http.Get("http://" + server.Addr + "/rpg/api/profile")
	if err != nil {
		t.Fatal(err)
	}
	defer response2.Body.Close()
	if response2.StatusCode != 503 {
		t.Fatal("RPG must stay disabled")
	}
}

func TestWebListenerKeepsGatewayProtection(t *testing.T) {
	_, s, _ := fixture(t)
	server, err := StartHTTP(s, HTTPConfig{ListenAddr: "127.0.0.1:0", PublicURL: "https://api.example.com", GatewaySecret: "12345678901234567890123456789012"})
	if err != nil {
		t.Fatal(err)
	}
	defer ShutdownHTTP(server)
	for _, tc := range []struct {
		path   string
		status int
	}{{"/health", 200}, {"/rpg/api/profile", 401}} {
		response, err := http.Get("http://" + server.Addr + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("%s: %d", tc.path, response.StatusCode)
		}
	}
}
