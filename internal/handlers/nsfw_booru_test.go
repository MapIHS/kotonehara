package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func TestRule34CommandRegistration(t *testing.T) {
	cfg := config.Config{Prefix: "."}
	if !commands.CanHandle(".rule34 neko", cfg) || !commands.CanHandle(".r34 neko", cfg) {
		t.Fatal("rule34 command or r34 alias not registered")
	}
	for _, cmd := range commands.Commands() {
		if cmd.Name == "rule34" {
			if cmd.Disable || !cmd.IsPrefix || !cmd.IsQuery || !cmd.IsPrivate || cmd.Tags != "nsfw" {
				t.Fatalf("unexpected rule34 flags: %+v", cmd)
			}
			return
		}
	}
	t.Fatal("rule34 command missing from registry")
}

func TestRule34FetcherAndSelection(t *testing.T) {
	if _, _, err := rule34Fetcher(context.Background(), config.Config{}, "neko", 10); err == nil {
		t.Fatal("expected error when BASEAPI_URL is empty")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nsfw/rule34" || r.URL.Query().Get("tags") != "cat_ears 1girl" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"success","data":[{"id":1,"file_url":"https://img.rule34.xxx/1.webm","tags":"cat_ears","score":5},{"id":2,"file_url":"","sample_url":"https://img.rule34.xxx/2.jpg","tags":"tag1 tag2 tag3 tag4 tag5 tag6 tag7 tag8 tag9 tag10 tag11 tag12 tag13 tag14 tag15 tag16","score":99}]}`)
	}))
	defer server.Close()

	mediaURL, caption, err := rule34Fetcher(context.Background(), config.Config{BASEApiURL: server.URL}, "cat_ears 1girl", 10)
	if err != nil {
		t.Fatal(err)
	}
	if mediaURL != "https://img.rule34.xxx/2.jpg" {
		t.Fatalf("expected non-webm fallback sample URL, got %q", mediaURL)
	}
	if !strings.Contains(caption, "⭐ Score: 99") || !strings.Contains(caption, "...") || strings.Contains(caption, "tag16") {
		t.Fatalf("unexpected bounded caption: %q", caption)
	}

	onlyWebm := []api.Rule34Post{{ID: 3, FileURL: "https://img.rule34.xxx/3.webm?token=1"}}
	_, chosenURL, ok := selectRule34Post(onlyWebm, func(n int) int { return 0 })
	if !ok || chosenURL != onlyWebm[0].FileURL {
		t.Fatalf("expected webm fallback when no preferred media exists: ok=%v url=%q", ok, chosenURL)
	}
}

func TestDetectBooruMediaKind(t *testing.T) {
	gifBytes := []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;")
	if got := detectBooruMediaKind("https://example.org/a.gif", gifBytes); got != booruMediaGIF {
		t.Fatalf("gif detected as %v", got)
	}
	mp4Bytes := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
	if got := detectBooruMediaKind("https://example.org/v.mp4", mp4Bytes); got != booruMediaVideo {
		t.Fatalf("mp4 detected as %v", got)
	}
	binaryBytes := []byte{0x00, 0x01, 0x02, 0x03}
	if got := detectBooruMediaKind("https://example.org/v.webm?x=1", binaryBytes); got != booruMediaDocument {
		t.Fatalf("webm detected as %v", got)
	}
	if got := detectBooruMediaKind("https://example.org/v.mp4?x=1", binaryBytes); got != booruMediaVideo {
		t.Fatalf("mp4 ext fallback detected as %v", got)
	}
	pngBytes := []byte("\x89PNG\r\n\x1a\n")
	if got := detectBooruMediaKind("https://example.org/i.png", pngBytes); got != booruMediaImage {
		t.Fatalf("png detected as %v", got)
	}
}
