package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/MapIHS/kotonehara/internal/clients"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestRemoveBGAlbumFiltersCapsAndContinuesAfterFailure(t *testing.T) {
	items := []whatsmeow.DownloadableMessage{&waE2E.VideoMessage{}, (*waE2E.ImageMessage)(nil)}
	for i := 0; i < maxRemoveBGAlbumImages+2; i++ {
		items = append(items, &waE2E.ImageMessage{FileLength: proto.Uint64(uint64(i + 1))})
	}
	var seen []uint64
	sent, total := removeBGAlbum(context.Background(), items, func(_ context.Context, m whatsmeow.DownloadableMessage) error {
		n := m.(*waE2E.ImageMessage).GetFileLength()
		seen = append(seen, n)
		if n == 2 {
			return errors.New("backend failed")
		}
		return nil
	})
	want := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if sent != 9 || total != 10 || !reflect.DeepEqual(seen, want) {
		t.Fatalf("sent=%d total=%d sequence=%v", sent, total, seen)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	sent, total = removeBGAlbum(ctx, items, func(context.Context, whatsmeow.DownloadableMessage) error { calls++; cancel(); return nil })
	if calls != 1 || sent != 1 || total != 10 {
		t.Fatalf("cancellation ignored: calls=%d sent=%d total=%d", calls, sent, total)
	}
}

func TestRemoveBGResultFetchHonorsContextAndLimits(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/result.png":
			w.Write([]byte("result"))
		case "/redirect":
			http.Redirect(w, r, "/result.png", http.StatusFound)
		case "/large":
			w.Header().Set("Content-Length", "33554433")
		}
	}))
	defer s.Close()
	data, err := fetchRemoveBGImage(context.Background(), s.Client(), s.URL, "/result.png")
	if err != nil || string(data) != "result" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	for _, path := range []string{"", "https://other.example/result.png", "/redirect"} {
		if _, err := fetchRemoveBGImage(context.Background(), s.Client(), s.URL, path); err == nil {
			t.Fatalf("accepted invalid result %q", path)
		}
	}
	if _, err := fetchRemoveBGImage(context.Background(), s.Client(), s.URL, "/large"); !errors.Is(err, clients.ErrAlbumLimit) {
		t.Fatalf("size not rejected: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchRemoveBGImage(ctx, s.Client(), s.URL, "/result.png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}
