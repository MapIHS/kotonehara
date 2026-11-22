package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestYoutubePostRequestsPostEndpoint(t *testing.T) {
	target := "https://youtube.com/post/UgkxThicaK5068CU3syx8JA0BFhKqE5OgZa3?surface=shorts&data=a&si=b"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/youtube/post" || r.URL.Query().Get("url") != target {
			t.Errorf("wrong path/query: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"success","data":{"id":"post","text":"Caption","author":"Author","photos":[{"url":"https://yt3.ggpht.com/one=s0"}]}}`)
	}))
	defer server.Close()
	post, err := New(server.URL, time.Second).YoutubePost(context.Background(), target)
	if err != nil || post.ID != "post" || len(post.Photos) != 1 || post.Text != "Caption" {
		t.Fatalf("post=%v err=%v", post, err)
	}
}

func TestInstagramStatusClassification(t *testing.T) {
	for _, tc := range []struct {
		status   int
		sentinel error
	}{{404, ErrInstagramUnavailable}, {503, ErrInstagramBusy}, {502, nil}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"message":"upstream error"}`)
			}))
			defer server.Close()
			_, err := New(server.URL, time.Second).Instagram(context.Background(), "https://instagram.com/reel/example/")
			if err == nil || (tc.sentinel != nil && !errors.Is(err, tc.sentinel)) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
