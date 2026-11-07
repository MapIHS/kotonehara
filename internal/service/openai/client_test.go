package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestChatPreservesTextAndImages(t *testing.T) {
	const dataURL = "data:image/png;base64,aW1hZ2U="
	for _, tc := range []struct {
		name    string
		message string
		want    Reply
	}{
		{"plain text", `{"content":" halo "}`, Reply{Text: "halo"}},
		{"multipart object", `{"content":[{"type":"text","text":"Ini gambarnya"},{"type":"image_url","image_url":{"url":"https://cdn.example/a.png"}}]}`, Reply{Text: "Ini gambarnya", Images: []string{"https://cdn.example/a.png"}}},
		{"multipart string", `{"content":[{"type":"text","text":"Satu"},{"type":"text","text":"Dua"},{"type":"image_url","image_url":"https://cdn.example/a.png"}]}`, Reply{Text: "Satu\nDua", Images: []string{"https://cdn.example/a.png"}}},
		{"message images", `{"content":"Hasil","images":[{"type":"image_url","image_url":{"url":"` + dataURL + `"}},{"image_url":"https://cdn.example/b.jpg"}]}`, Reply{Text: "Hasil", Images: []string{dataURL, "https://cdn.example/b.jpg"}}},
		{"image only", `{"content":null,"images":[{"image_url":{"url":"` + dataURL + `"}}]}`, Reply{Images: []string{dataURL}}},
		{"image only content", `{"content":[{"type":"image_url","image_url":{"url":"https://cdn.example/a.png"}}]}`, Reply{Images: []string{"https://cdn.example/a.png"}}},
		{"markdown", `{"content":"Hasil\n![kucing](https://cdn.example/a.png)"}`, Reply{Text: "Hasil", Images: []string{"https://cdn.example/a.png"}}},
		{"markdown angle URL and title", `{"content":"![kucing](<https://cdn.example/a.png> \"Judul\")"}`, Reply{Images: []string{"https://cdn.example/a.png"}}},
		{"markdown data URL", `{"content":"![hasil](` + dataURL + `)"}`, Reply{Images: []string{dataURL}}},
		{"deduplicate across fields", `{"content":[{"text":"![a](https://cdn.example/a.png)"},{"image_url":"https://cdn.example/a.png"}],"images":[{"url":"https://cdn.example/a.png"},{"image_url":{"url":"https://cdn.example/b.png"}}]}`, Reply{Images: []string{"https://cdn.example/a.png", "https://cdn.example/b.png"}}},
		{"cap images", `{"content":"Hasil","images":["https://cdn.example/1.png","https://cdn.example/2.png","https://cdn.example/3.png","https://cdn.example/4.png","https://cdn.example/5.png"]}`, Reply{Text: "Hasil", Images: []string{"https://cdn.example/1.png", "https://cdn.example/2.png", "https://cdn.example/3.png", "https://cdn.example/4.png"}}},
		{"unsupported refs ignored", `{"content":"Teks","images":[{"image_url":"file:///private.png"},{"image_url":"javascript:alert(1)"},{"image_url":"/relative.png"},{"image_url":42},{"image_url":{"image_url":{"url":"https://cdn.example/deep.png"}}}]}`, Reply{Text: "Teks"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("missing request authorization")
				}
				var request map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if len(request) != 3 || string(request["stream"]) != "false" || string(request["model"]) != `"fixture-model"` {
					t.Errorf("request fields changed: %v", request)
				}
				if string(request["messages"]) != `[{"role":"user","content":"buat gambar"}]` {
					t.Errorf("messages = %s", request["messages"])
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices":[{"message":`+tc.message+`}]}`)
			}))
			defer server.Close()
			client := New(server.URL+"/v1", "fixture-key", "fixture-model", time.Second)
			got, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "buat gambar"}})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Chat() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestChatRejectsInvalidAndEmptyResponses(t *testing.T) {
	for _, body := range []string{
		`not json`,
		`{"choices":[]}`,
		`{"choices":[{"message":{"content":"  "}}]}`,
		`{"choices":[{"message":{"content":{"unsupported":true}}}]}`,
		`{"choices":[{"message":{"images":[{"image_url":42}]}}]}`,
		`{"choices":[{"message":{"content":[{"type":"image_url","image_url":"file:///private.png"}]}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			_, err := New(server.URL, "key", "model", time.Second).Chat(context.Background(), []Message{{Role: "user", Content: "halo"}})
			if err == nil {
				t.Fatal("invalid/empty reply accepted")
			}
		})
	}
}

func TestChatCompletionTextWrapper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"jawaban","images":["https://cdn.example/a.png"]}}]}`)
	}))
	defer server.Close()
	client := New(server.URL+"/v1/chat/completions/", "key", "model", time.Second)
	text, err := client.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "halo"}})
	if err != nil || text != "jawaban" {
		t.Fatalf("ChatCompletion() = %q, %v", text, err)
	}
}

type replyRoundTripper func(*http.Request) (*http.Response, error)

func (f replyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type oversizedReply struct {
	read, remaining int
	closed          bool
}

func (r *oversizedReply) Read(dst []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(dst), r.remaining)
	for i := range dst[:n] {
		dst[i] = ' '
	}
	r.remaining -= n
	r.read += n
	return n, nil
}

func (r *oversizedReply) Close() error { r.closed = true; return nil }

func TestChatBoundsLargeBase64Responses(t *testing.T) {
	body := &oversizedReply{remaining: maxResponseBytes + 4096}
	client := New("https://provider.example/v1", "key", "model", time.Second)
	client.HTTP.Transport = replyRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
	})
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "halo"}})
	if err == nil || !strings.Contains(err.Error(), "terlalu besar") {
		t.Fatalf("oversized reply error = %v", err)
	}
	if body.read != maxResponseBytes+1 || !body.closed {
		t.Fatalf("read = %d, closed = %t", body.read, body.closed)
	}
}

func TestRouterPreservesImagesOnFailover(t *testing.T) {
	var firstCalls, secondCalls int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls++
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":null,"images":[{"image_url":{"url":"data:image/png;base64,aW1hZ2U="}}]}}]}`)
	}))
	defer second.Close()
	router := NewRouter([]Provider{
		{Name: "first", BaseURL: first.URL, APIKey: "one", Model: "first-model"},
		{Name: "second", BaseURL: second.URL, APIKey: "two", Model: "second-model"},
	}, time.Second)
	answer, err := router.Chat(context.Background(), []Message{{Role: "user", Content: "buat gambar"}})
	if err != nil || answer.Text != "" || !reflect.DeepEqual(answer.Images, []string{"data:image/png;base64,aW1hZ2U="}) || firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("Chat() = %+v, %v; calls %d/%d", answer, err, firstCalls, secondCalls)
	}
}

func TestRouterDoesNotFailoverBadCredentials(t *testing.T) {
	var secondCalls int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls++
	}))
	defer second.Close()
	router := NewRouter([]Provider{
		{BaseURL: first.URL, APIKey: "one", Model: "model"},
		{BaseURL: second.URL, APIKey: "two", Model: "model"},
	}, time.Second)
	_, err := router.Chat(context.Background(), []Message{{Role: "user", Content: "halo"}})
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusUnauthorized || secondCalls != 0 {
		t.Fatalf("error = %v; second calls = %d", err, secondCalls)
	}
}
