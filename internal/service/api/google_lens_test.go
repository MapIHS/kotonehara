package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGoogleLensRequestAndDecode(t *testing.T) {
	image := []byte("\x89PNG\r\n\x1a\nimage-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/google-lens" || r.URL.Query().Get("mode") != "visual" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "image/png" {
			t.Errorf("Hararest request should send the image without worker credentials")
		}
		got, err := io.ReadAll(r.Body)
		if err != nil || string(got) != string(image) {
			t.Errorf("image body mismatch: %q, %v", got, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"mode": "visual", "lensPage": "https://lens.google.com/abc",
				"visualMatches": []any{map[string]string{"title": "Match", "url": "https://example.com", "thumbnail": ""}},
			},
		})
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
