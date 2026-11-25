package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRule34RequestsAndDecodesPosts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Accept") != "application/json" {
			t.Errorf("invalid method/header: %s %s", r.Method, r.Header.Get("Accept"))
		}
		if r.URL.Path != "/api/nsfw/rule34" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("tags") != "cat_ears 1girl" || r.URL.Query().Get("limit") != "5" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"success","data":[{"id":101,"file_url":"https://img.rule34.xxx/images/1/abc.jpg","sample_url":"https://img.rule34.xxx/samples/1/sample_abc.jpg","tags":"cat_ears 1girl","score":42,"width":1000,"height":1200,"rating":"explicit"}]}`)
	}))
	defer server.Close()

	client := New(server.URL+"/", time.Second)
	posts, err := client.Rule34(context.Background(), "  cat_ears 1girl  ", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].ID != 101 || posts[0].Score != 42 || posts[0].FileURL == "" {
		t.Fatalf("unexpected posts: %#v", posts)
	}
}

func TestRule34HandlesEmptyResultsDefaultsAndErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tags") == "missing" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"success","data":[]}`)
			return
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("expected default limit=10, got %s", r.URL.Query().Get("limit"))
		}
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `{"status":"error","message":"Rule34 menolak akses"}`)
	}))
	defer server.Close()

	client := New(server.URL, time.Second)

	if _, err := client.Rule34(context.Background(), "   ", 10); err == nil {
		t.Fatal("expected error for blank tags")
	}

	empty, err := client.Rule34(context.Background(), "missing", 10)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty posts=%#v err=%v", empty, err)
	}

	_, err = client.Rule34(context.Background(), "solo", 0)
	if err == nil || !strings.Contains(err.Error(), "Rule34 menolak akses") {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Rule34(ctx, "solo", 10); err == nil {
		t.Fatal("canceled request succeeded")
	}
}
