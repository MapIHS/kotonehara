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

func TestSocialProfileRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Accept") != "application/json" {
			t.Errorf("invalid request %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/pinterest/user":
			if r.URL.Query().Get("username") != "https://pinterest.com/some_user/" {
				t.Error("profile URL not preserved")
			}
			fmt.Fprint(w, `{"success":true,"data":{"user":{"username":"some_user","fullName":"Some User","followers":0},"pins":[{"id":"1","title":"Pin","link":"https://pinterest.com/pin/1/"}]}}`)
		case "/api/pixiv/user":
			if r.URL.Query().Get("id") != "https://www.pixiv.net/users/123" || r.URL.Query().Get("mode") != "safe" {
				t.Error("missing user/mode")
			}
			fmt.Fprint(w, `{"success":true,"data":{"user":{"id":"123","name":"Artist"},"artworks":[{"id":"456","nsfw":false,"pageCount":2}]}}`)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL+"/ignored-prefix", time.Second)
	pin, err := client.PinterestUser(context.Background(), "https://pinterest.com/some_user/")
	if err != nil {
		t.Fatal(err)
	}
	if pin.User.Followers == nil || *pin.User.Followers != 0 || pin.User.Following != nil || len(pin.Pins) != 1 {
		t.Fatalf("invalid profile %#v", pin)
	}
	pixiv, err := client.PixivUser(context.Background(), "https://www.pixiv.net/users/123", "safe")
	if err != nil || pixiv.User.ID != "123" || len(pixiv.Artworks) != 1 || pixiv.Artworks[0].PageCount != 2 {
		t.Fatalf("invalid Pixiv profile %#v %v", pixiv, err)
	}
}

func TestPixivSearchModes(t *testing.T) {
	for _, mode := range []string{"safe", "r18", "all", ""} {
		t.Run(mode, func(t *testing.T) {
			want := mode
			if want == "" {
				want = "all"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/pixiv/search" || r.URL.Query().Get("q") != "cat & moon" || r.URL.Query().Get("mode") != want {
					t.Errorf("unexpected request %s", r.URL)
				}
				fmt.Fprint(w, `{"success":true,"data":{"results":[{"id":"12","title":"Moon","url":"https://example.org/image.jpg"}]}}`)
			}))
			defer server.Close()
			client := New(server.URL, time.Second)
			var result *PixivSearchResponse
			var err error
			if mode == "" {
				result, err = client.PixivSearch(context.Background(), "cat & moon")
			} else {
				result, err = client.PixivSearch(context.Background(), "cat & moon", mode)
			}
			if err != nil || len(result.Results) != 1 {
				t.Fatalf("%#v %v", result, err)
			}
		})
	}
	client := New("http://invalid.example", time.Second)
	if _, err := client.PixivSearch(context.Background(), "cat", "bad"); err == nil {
		t.Fatal("invalid search mode accepted")
	}
	if _, err := client.PixivUser(context.Background(), "123", "safe", "all"); err == nil {
		t.Fatal("multiple modes accepted")
	}
}

func TestProfileFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"upstream error", `{"message":"unavailable"}`, 502},
		{"invalid JSON", `not-json`, 200},
		{"missing user", `{"success":false,"data":{}}`, 200},
		{"oversize", strings.Repeat("x", maxAPIResponseSize+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			client := New(server.URL, time.Second)
			if _, err := client.PinterestUser(context.Background(), "user"); err == nil {
				t.Fatal("Pinterest failure accepted")
			}
			if _, err := client.PixivUser(context.Background(), "123"); err == nil {
				t.Fatal("Pixiv failure accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New("http://localhost", time.Second).PinterestUser(ctx, "user"); err == nil {
		t.Fatal("canceled request accepted")
	}
}
