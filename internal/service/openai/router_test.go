package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRouterFailsOverOnRateLimit(t *testing.T) {
	var firstCalls, secondCalls int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls++
		http.Error(w, `{"error":{"message":"rate limited"}}`, http.StatusTooManyRequests)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"jawaban"}}]}`))
	}))
	defer second.Close()

	router := NewRouter([]Provider{
		{Name: "first", BaseURL: first.URL + "/v1", APIKey: "one", Model: "first-model"},
		{Name: "second", BaseURL: second.URL + "/v1", APIKey: "two", Model: "second-model"},
	}, time.Second)

	answer, err := router.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "halo"}})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if answer != "jawaban" || firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("answer=%q first=%d second=%d, want jawaban/1/1", answer, firstCalls, secondCalls)
	}
}

func TestRouterRotatesStartingProvider(t *testing.T) {
	var mu sync.Mutex
	models := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		models = append(models, payload.Model)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	router := NewRouter([]Provider{
		{BaseURL: server.URL + "/v1", APIKey: "one", Model: "model-one"},
		{BaseURL: server.URL + "/v1", APIKey: "two", Model: "model-two"},
	}, time.Second)
	for range 2 {
		if _, err := router.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "halo"}}); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := models, []string{"model-one", "model-two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}
