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

func TestBilibiliAPI(t *testing.T) {
	const target = "https://www.bilibili.com/video/BV1Ak8Q6hECg/?p=2&share_source=copy_web"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bilibili" || r.URL.Query().Get("url") != target || r.URL.Query().Get("quality") != "1080p" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("missing Accept header")
		}
		fmt.Fprint(w, `{"status":"success","data":{"id":"BV1Ak8Q6hECg","title":"知更鸟","page":2,"format":"dash","requestedQuality":80,"availableQualities":[80,64],"media":[{"type":"video","quality":80,"url":"https://cdn.bilivideo.com/video","backupUrls":["https://cdn.bilivideo.com/backup"],"codecs":"avc1.640028","width":1920,"height":1080},{"type":"audio","url":"https://cdn.bilivideo.com/audio","codecs":"mp4a.40.2"}],"headers":{"Referer":"https://www.bilibili.com/"}}}`)
	}))
	defer server.Close()
	result, err := New(server.URL, time.Second).Bilibili(context.Background(), target, "1080p")
	if err != nil {
		t.Fatal(err)
	}
	if result.Page != 2 || result.Media[0].Height != 1080 || len(result.Media[0].BackupURLs) != 1 || result.Headers["Referer"] == "" {
		t.Fatalf("incorrect response: %+v", result)
	}
}

func TestBilibiliAPIFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"upstream error", 502, `{"status":"error","message":"login expired"}`},
		{"invalid json", 200, `<html>blocked</html>`},
		{"no media", 200, `{"status":"success","data":{"media":[]}}`},
		{"failed envelope", 200, `{"status":"fail","data":{"media":[{"url":"x"}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			_, err := New(server.URL, time.Second).Bilibili(context.Background(), "https://b23.tv/test", "")
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.status == 502 && !strings.Contains(err.Error(), "login expired") {
				t.Fatal(err)
			}
		})
	}
}

func TestBilibiliOpusAPI(t *testing.T) {
	const id = "1247969693541597185"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bilibili" || r.URL.Query().Get("url") != "https://www.bilibili.com/opus/"+id {
			t.Error("wrong Opus request")
		}
		fmt.Fprint(w, `{"status":"success","data":{"id":"1247969693541597185","description":"Hey？","format":"images","media":[{"type":"gif","url":"https://i0.hdslb.com/bfs/new_dyn/post.gif","mimeType":"image/gif"}],"headers":{"Referer":"https://www.bilibili.com/"}}}`)
	}))
	defer server.Close()
	result, err := New(server.URL, time.Second).Bilibili(context.Background(), "https://www.bilibili.com/opus/"+id, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != id || result.Description != "Hey？" || result.Format != "images" || result.Media[0].Type != "gif" {
		t.Fatalf("wrong Opus response: %+v", result)
	}
}
