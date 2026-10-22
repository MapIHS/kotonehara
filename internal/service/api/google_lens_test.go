package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGoogleLensRequestAndDecode(t *testing.T) {
	image := []byte("\x89PNG\r\n\x1a\nimage-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/google-lens" && r.URL.Query().Get("mode") == "visual":
			if r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "image/png" || r.Header.Get("Idempotency-Key") == "" {
				t.Errorf("Hararest request missing raw image headers or unexpectedly has worker credentials")
			}
			got, err := io.ReadAll(r.Body)
			if err != nil || string(got) != string(image) {
				t.Errorf("image body mismatch: %q, %v", got, err)
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": strings.Repeat("a", 48), "state": "processing", "pollAfterMs": 1,
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/jobs/"+strings.Repeat("a", 48):
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": strings.Repeat("a", 48), "state": "ready", "result": map[string]any{"kind": "data", "data": map[string]any{
					"mode": "visual", "lensPage": "https://lens.google.com/abc",
					"visualMatches": []any{map[string]string{"title": "Match", "url": "https://example.com", "thumbnail": ""}},
				}},
			}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := New(server.URL+"/ignored-prefix/", time.Second).GoogleLens(context.Background(), "visual", image)
	if err != nil {
		t.Fatal(err)
	}
	if result.LensPage != "https://lens.google.com/abc" || len(result.VisualMatches) != 1 || result.VisualMatches[0].Title != "Match" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGoogleLensRejectsInvalidImage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		image []byte
	}{
		{name: "unsupported image", image: []byte("not an image")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New("https://hararest.example", time.Second).GoogleLens(context.Background(), "visual", tc.image); err == nil {
				t.Fatal("expected request validation error")
			}
		})
	}
}
