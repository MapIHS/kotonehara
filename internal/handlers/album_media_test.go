package handlers

import (
	"context"
	"testing"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/service/api"
	"google.golang.org/protobuf/proto"
)

func TestTikTokAlbumUsesLazySourcesAndSingleCaption(t *testing.T) {
	err := sendTikTokMedia([]string{"https://cdn.example/1", "", "https://cdn.example/2"}, nil, "caption", func(items []clients.AlbumMedia) error {
		if len(items) != 2 || items[0].URL != "https://cdn.example/1" || items[1].URL != "https://cdn.example/2" {
			t.Fatalf("items=%v", items)
		}
		if len(items[0].Data) != 0 || len(items[1].Data) != 0 || items[0].Caption != "caption" || items[1].Caption != "" {
			t.Fatal("eager download or repeated caption")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = sendTikTokMedia(nil, proto.String("https://cdn.example/video"), "caption", func(items []clients.AlbumMedia) error {
		if len(items) != 1 || !items[0].IsVideo || items[0].URL != "https://cdn.example/video" {
			t.Fatalf("items=%v", items)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBilibiliAlbumDownloadsOnlyWhenRequested(t *testing.T) {
	result := &api.BilibiliResult{Media: []api.BilibiliMedia{{Type: "image"}, {Type: "gif"}}}
	calls := 0
	err := sendBilibiliImages(context.Background(), result,
		func(_ context.Context, item api.BilibiliMedia, limit int64) ([]byte, error) {
			calls++
			if limit != 7 {
				t.Fatal("budget not forwarded")
			}
			return []byte(item.Type), nil
		}, func(ctx context.Context, items []clients.AlbumMedia) error {
			if calls != 0 || len(items) != 2 || len(items[0].Data) != 0 || len(items[1].Data) != 0 {
				t.Fatal("eager downloads")
			}
			for i, item := range items {
				data, err := item.Fetch(ctx, 7)
				if err != nil || string(data) != result.Media[i].Type {
					t.Fatalf("data=%s err=%v", data, err)
				}
			}
			return nil
		})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
