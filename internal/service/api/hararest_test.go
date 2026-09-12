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

func TestYouTubeDownloadOptionalQuality(t *testing.T) {
	for _, tc := range []struct {
		name, quality string
		video         bool
	}{
		{"audio", "", false}, {"audio ignores quality", "720p", false},
		{"default video", "", true}, {"selected video", "720p", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := strings.Repeat("a", 48)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/youtube/jobs":
					if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") == "" {
						t.Error("missing POST/idempotency key")
					}
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["url"] != "https://youtu.be/test?foo=a&bar=b" {
						t.Errorf("URL changed: %v", body)
					}
					format := "audio"
					if tc.video {
						format = "video"
					}
					if body["format"] != format {
						t.Errorf("format=%s", body["format"])
					}
					if tc.video && tc.quality != "" {
						if body["quality"] != tc.quality {
							t.Errorf("quality=%s", body["quality"])
						}
					} else if _, ok := body["quality"]; ok {
						t.Error("optional quality must be omitted")
					}
					w.WriteHeader(http.StatusAccepted)
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "state": "ready", "result": map[string]any{"kind": "file", "size": len("media fixture")}}})
				case "/api/jobs/" + id + "/file":
					if r.Method != http.MethodGet {
						t.Error("file request must be GET")
					}
					w.Write([]byte("media fixture"))
				default:
					t.Errorf("unexpected path=%s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			data, err := New(server.URL, time.Second).YoutubeDownload(context.Background(), "https://youtu.be/test?foo=a&bar=b", tc.quality, tc.video)
			if err != nil || string(data) != "media fixture" {
				t.Fatalf("data=%q error=%v", data, err)
			}
		})
	}
}

func TestHararestResponseContracts(t *testing.T) {
	tests := []struct {
		name, path, body string
		call             func(*Client) (any, error)
	}{
		{"youtube", "/api/youtube/info", `{"status":"success","data":{"id":"video","duration":19.014,"videos":["144p","360p"]}}`, func(c *Client) (any, error) { return c.YoutubeInfo(context.Background(), "https://youtu.be/video") }},
		{"youtube search", "/api/youtube/search", `{"status":"success","data":[{"id":"video","duration":19.014,"url":"https://youtu.be/video"}]}`, func(c *Client) (any, error) { return c.YoutubeSearch(context.Background(), "test", 1) }},
		{"instagram", "/api/instagram", `{"status":"success","data":{"photos":[{"url":"https://cdn.example/photo.jpg","size":1024}],"videos":[{"url":"https://cdn.example/video.mp4","size":2048}]}}`, func(c *Client) (any, error) { return c.Instagram(context.Background(), "https://instagram.com/p/test") }},
		{"facebook", "/api/facebook", `{"status":"success","data":{"videos":[{"quality":"hd","url":"https://cdn.example/video.mp4","size":1024}]}}`, func(c *Client) (any, error) {
			return c.Facebook(context.Background(), "https://facebook.com/watch?v=test")
		}},
		{"tiktok photos", "/api/tiktok/download", `{"status":"success","data":{"id":"123","video":null,"images":["https://cdn.example/photo.jpg"]}}`, func(c *Client) (any, error) {
			return c.Tiktok(context.Background(), "https://tiktok.com/@user/photo/123")
		}},
		{"threads", "/api/threads/download", `{"status":"success","data":{"count":1,"items":[{"media_type":"image","download_url":"https://cdn.example/photo.jpg"}]}}`, func(c *Client) (any, error) {
			return c.Threads(context.Background(), "https://threads.com/@user/post/test")
		}},
		{"x", "/api/twitter/download", `{"status":"success","data":{"status":"ok","media_links":[{"media_type":"video","url":"https://cdn.example/video.mp4"}]}}`, func(c *Client) (any, error) { return c.X(context.Background(), "https://x.com/user/status/123") }},
		{"xiaohongshu", "/api/xiaohongshu", `{"status":"success","data":{"id":"123","type":"normal","cover":"https://cdn.example/photo.jpg","images":[{"url":"https://cdn.example/photo.jpg"}],"video":null}}`, func(c *Client) (any, error) {
			return c.Rednote(context.Background(), "https://xiaohongshu.com/explore/123")
		}},
		{"pinterest", "/api/pinterest", `{"success":true,"data":{"url":"https://cdn.example/photo.jpg","type":"image"}}`, func(c *Client) (any, error) { return c.Pinterest(context.Background(), "https://pin.it/test") }},
		{"pinterest search", "/api/pinterest/search", `{"success":true,"data":{"results":[{"id":"123","images":["https://cdn.example/photo.jpg"]}]}}`, func(c *Client) (any, error) { return c.PinterestSearch(context.Background(), "landscape") }},
		{"pixiv", "/api/pixiv", `{"success":true,"data":{"id":"123","urls":["https://i.pximg.net/photo.jpg"]}}`, func(c *Client) (any, error) { return c.Pixiv(context.Background(), "123") }},
		{"pixiv search", "/api/pixiv/search", `{"success":true,"data":{"results":[{"id":"123","url":"https://i.pximg.net/photo.jpg"}]}}`, func(c *Client) (any, error) { return c.PixivSearch(context.Background(), "landscape") }},
		{"brave", "/api/brave/search", `{"status":"success","data":{"results":[{"title":"Test","link":"https://example.com"}],"totalResults":"1","searchTime":0.2}}`, func(c *Client) (any, error) { return c.SearchBrave(context.Background(), "test") }},
		{"waifu", "/api/nsfw/waifu", `{"status":"success","data":{"items":[{"url":"https://cdn.waifu.im/test.png","source":"https://example.com","width":100,"height":100}],"pageNumber":1}}`, func(c *Client) (any, error) { return c.WaifuIm(context.Background(), "waifu", false) }},
		{"purrbot", "/api/nsfw/purrbot/neko", `{"status":"success","data":{"error":false,"link":"https://cdn.example/test.gif"}}`, func(c *Client) (any, error) {
			v, e := c.PurrBot(context.Background(), "neko")
			return map[string]string{"link": v}, e
		}},
		{"danbooru", "/api/nsfw/danbooru", `{"status":"success","data":[{"id":123,"file_url":"https://cdn.example/photo.jpg"}]}`, func(c *Client) (any, error) { return c.Danbooru(context.Background(), "landscape sky", 1) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path=%s", r.URL.Path)
				}
				if tc.name == "danbooru" && r.URL.Query().Get("tags") != "landscape sky" {
					t.Errorf("tags=%q", r.URL.Query().Get("tags"))
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			data, err := tc.call(New(server.URL, time.Second))
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(data)
			if err != nil || len(encoded) < 3 {
				t.Fatalf("empty or invalid decoded payload: %s %v", encoded, err)
			}
			var envelope map[string]any
			var actual any
			json.Unmarshal([]byte(tc.body), &envelope)
			json.Unmarshal(encoded, &actual)
			expected := envelope["data"]
			if tc.name == "waifu" {
				expected = expected.(map[string]any)["items"].([]any)[0]
			}
			if tc.name == "purrbot" {
				expected = map[string]any{"link": expected.(map[string]any)["link"]}
			}
			assertJSONSubset(t, expected, actual)
		})
	}
}

func TestHararestErrorDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		io.WriteString(w, `{"status":"fail","message":"Download rate limit exceeded"}`)
	}))
	defer server.Close()
	_, err := New(server.URL, time.Second).YoutubeDownload(context.Background(), "https://youtu.be/test", "", false)
	if err == nil || !strings.Contains(err.Error(), "Download rate limit exceeded") {
		t.Fatalf("lost API error details: %v", err)
	}
}

func TestOCRContentType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfixture")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "image/png" {
			t.Errorf("content-type=%s", r.Header.Get("Content-Type"))
		}
		io.WriteString(w, `{"status":"success","data":{"text":"HARAREST TEST"}}`)
	}))
	defer server.Close()
	text, err := New(server.URL, time.Second).ExtractOCR(context.Background(), png)
	if err != nil || text != "HARAREST TEST" {
		t.Fatalf("text=%q error=%v", text, err)
	}
}

func assertJSONSubset(t *testing.T, want, got any) {
	t.Helper()
	switch value := want.(type) {
	case map[string]any:
		object, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected object, got %T", got)
		}
		for key, field := range value {
			actual, exists := object[key]
			if !exists {
				t.Fatalf("decoded field %q is missing", key)
			}
			assertJSONSubset(t, field, actual)
		}
	case []any:
		array, ok := got.([]any)
		if !ok || len(array) != len(value) {
			t.Fatalf("expected %d items, got %#v", len(value), got)
		}
		for i, item := range value {
			assertJSONSubset(t, item, array[i])
		}
	default:
		if want != got {
			t.Fatalf("expected %#v, got %#v", want, got)
		}
	}
}
