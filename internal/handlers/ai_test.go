package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/openai"
	"go.mau.fi/whatsmeow"
)

func TestAIOmitsDefaultSystemPromptFromRequest(t *testing.T) {
	t.Setenv("TAILSCALE_SOCKS5", "")
	for _, system := range []string{"", "  ", "Custom instructions"} {
		t.Run(system, func(t *testing.T) {
			var received []openai.Message
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []openai.Message `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				received = body.Messages
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"Halo juga"}}]}`)
			}))
			defer server.Close()
			var replies []string
			m := &message.Message{Query: "Halo", Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				replies = append(replies, text)
				return whatsmeow.SendResponse{}, nil
			}}
			aiCmd(context.Background(), nil, m, config.Config{OpenAIBaseURL: server.URL, OpenAIAPIKey: "fixture-key", OpenAIModel: "fixture-model", OpenAISystemPrompt: system})
			want := []openai.Message{{Role: "user", Content: "Halo"}}
			if strings.TrimSpace(system) != "" {
				want = append([]openai.Message{{Role: "system", Content: system}}, want...)
			}
			if !reflect.DeepEqual(received, want) || !reflect.DeepEqual(replies, []string{"Halo juga"}) {
				t.Fatalf("messages=%+v replies=%v, want messages=%+v and plain reply", received, replies, want)
			}
		})
	}
}

func aiTestPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

type aiImageResolver struct{ addr string }

func (r aiImageResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr(r.addr)}, nil
}

type aiImageTransport func(*http.Request) (*http.Response, error)

func (f aiImageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAIImageDataURLsValidateContentsAndBounds(t *testing.T) {
	data := aiTestPNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	got, err := loadAIImage(context.Background(), uri, nil, nil)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("data URL: %v", err)
	}
	largePixels := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(largePixels[16:20], 50_000)
	binary.BigEndian.PutUint32(largePixels[20:24], 50_000)
	binary.BigEndian.PutUint32(largePixels[29:33], crc32.ChecksumIEEE(largePixels[12:29]))
	for name, source := range map[string]string{
		"invalid base64":       "data:image/png;base64,%%%",
		"not an image":         "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<html>not image</html>")),
		"unsupported data URI": "data:text/plain;base64,SGk=",
		"unsupported encoding": "data:image/png,plain",
		"too many pixels":      "data:image/png;base64," + base64.StdEncoding.EncodeToString(largePixels),
		"truncated PNG":        "data:image/png;base64," + base64.StdEncoding.EncodeToString(data[:33]),
		"too many bytes":       "data:image/png;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(maxAIImageBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadAIImage(context.Background(), source, nil, nil); err == nil {
				t.Fatal("invalid image accepted")
			}
		})
	}
}

func TestAIImageFetchDoesNotTrustURLsOrResponseHeaders(t *testing.T) {
	data := aiTestPNG(t)
	for _, tc := range []struct {
		name, url, address, body, redirect string
		status                             int
		size                               int64
		valid                              bool
		calls                              int
	}{
		{name: "image with generic MIME", url: "https://images.example/result", address: "8.8.8.8", status: 200, body: string(data), valid: true, calls: 1},
		{name: "HTML pretending to be PNG", url: "https://images.example/result", address: "8.8.8.8", status: 200, body: "<html>error</html>", calls: 1},
		{name: "HTTP failure", url: "https://images.example/result", address: "8.8.8.8", status: 404, body: "secret-token", calls: 1},
		{name: "large content length", url: "https://images.example/result", address: "8.8.8.8", status: 200, size: maxAIImageBytes + 1, calls: 1},
		{name: "large streamed body", url: "https://images.example/result", address: "8.8.8.8", status: 200, size: -1, body: strings.Repeat("x", maxAIImageBytes+1), calls: 1},
		{name: "private IP", url: "http://127.0.0.1/secret", address: "8.8.8.8"},
		{name: "private DNS", url: "https://images.example/result", address: "10.0.0.1"},
		{name: "URL credentials", url: "https://user:secret-token@images.example/result", address: "8.8.8.8"},
		{name: "unsupported scheme", url: "file:///secret-token", address: "8.8.8.8"},
		{name: "private redirect", url: "https://images.example/result", address: "8.8.8.8", status: 302, redirect: "http://127.0.0.1/secret-token", calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			httpClient := &http.Client{Transport: aiImageTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Authorization") != "" {
					t.Error("credentials forwarded to image host")
				}
				header := http.Header{"Content-Type": []string{"image/png"}}
				if tc.redirect != "" {
					header.Set("Location", tc.redirect)
				}
				return &http.Response{StatusCode: tc.status, Header: header, ContentLength: tc.size, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			got, err := loadAIImage(context.Background(), tc.url, httpClient, aiImageResolver{tc.address})
			if tc.valid && (err != nil || !bytes.Equal(got, data)) {
				t.Fatalf("image fetch: %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("invalid result accepted")
			}
			if calls != tc.calls {
				t.Fatalf("HTTP requests=%d want %d", calls, tc.calls)
			}
			if err != nil && strings.Contains(err.Error(), "secret-token") {
				t.Fatal("secret leaked through error")
			}
		})
	}
}

func TestAISendsImageBytesAndKeepsText(t *testing.T) {
	for _, text := range []string{"", "Ini gambarnya"} {
		var texts []string
		var images [][]byte
		err := sendAIReply(context.Background(), openai.Reply{Text: text, Images: []string{"data:image/png;base64,fixture"}}, func(text string) error { texts = append(texts, text); return nil }, func(data []byte) error { images = append(images, data); return nil }, func(context.Context, string) ([]byte, error) { return []byte("image bytes"), nil })
		if err != nil || len(images) != 1 || string(images[0]) != "image bytes" {
			t.Fatalf("images=%v err=%v", images, err)
		}
		if text == "" && len(texts) != 0 || text != "" && !reflect.DeepEqual(texts, []string{text}) {
			t.Fatalf("texts=%v", texts)
		}
	}
}

func TestAIImageFailureDoesNotDiscardOtherImages(t *testing.T) {
	var sent []string
	err := sendAIReply(context.Background(), openai.Reply{Text: "jawaban", Images: []string{"bad", "good"}}, func(text string) error { sent = append(sent, text); return nil }, func(data []byte) error { sent = append(sent, string(data)); return nil }, func(_ context.Context, source string) ([]byte, error) {
		if source == "bad" {
			return nil, errors.New("secret-token")
		}
		return []byte("image"), nil
	})
	if err == nil || err.Error() != "1 dari 2 gambar AI gagal dikirim" || !reflect.DeepEqual(sent, []string{"jawaban", "image"}) {
		t.Fatalf("sent=%v err=%v", sent, err)
	}
}

func TestAIReplyBoundsAndCancellation(t *testing.T) {
	var text string
	images := 0
	err := sendAIReply(context.Background(), openai.Reply{Text: strings.Repeat("あ", maxAIReplySize), Images: []string{"1", "2", "3", "4", "5"}}, func(s string) error { text = s; return nil }, func([]byte) error { images++; return nil }, func(context.Context, string) ([]byte, error) { return []byte{1}, nil })
	if err != nil || !utf8.ValidString(text) || len(text) > maxAIReplySize || images != maxAIImages {
		t.Fatalf("text bytes=%d images=%d err=%v", len(text), images, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadAIImage(ctx, "data:image/png;base64,", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled image: %v", err)
	}
	err = sendAIReply(ctx, openai.Reply{Text: "hello"}, func(string) error { t.Fatal("sent after cancellation"); return nil }, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled reply: %v", err)
	}
}
