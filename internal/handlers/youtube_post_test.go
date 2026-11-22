package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

func TestYoutubePostURLClassification(t *testing.T) {
	for _, raw := range []string{"https://youtube.com/post/UgkxThicaK5068CU3syx8JA0BFhKqE5OgZa3?surface=shorts&data=tracking", "https://www.youtube.com/post/UgExample", "https://m.youtube.com/post/UgExample"} {
		if !isYoutubePostURL(raw) {
			t.Fatalf("not recognized: %s", raw)
		}
	}
	for _, raw := range []string{"https://youtube.com/watch?v=abc", "https://youtu.be/abc", "https://youtube.com.evil.example/post/UgExample", "https://user:pass@youtube.com/post/UgExample", "https://youtube.com:8443/post/UgExample", "ftp://youtube.com/post/UgExample"} {
		if isYoutubePostURL(raw) {
			t.Fatalf("accepted: %s", raw)
		}
	}
}

func TestYoutubePostPhotosStayLazyOrderedAndCaptionedOnce(t *testing.T) {
	var post api.YoutubePostResult
	if err := json.Unmarshal([]byte(`{"author":"Author","text":"Caption","photos":[{"url":"https://yt3.ggpht.com/one=s0"},{"url":"https://yt3.ggpht.com/two=s0"}]}`), &post); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("send failed")
	err := sendYoutubePostPhotos(&post, func(items []clients.AlbumMedia) error {
		if len(items) != 2 || items[0].URL != post.Photos[0].URL || items[1].URL != post.Photos[1].URL {
			t.Fatalf("items=%v", items)
		}
		if len(items[0].Data) > 0 || items[0].Fetch != nil || items[0].IsVideo || items[1].IsVideo {
			t.Fatal("unexpected eager download or video")
		}
		if items[0].Caption != "Author\n\nCaption" || items[1].Caption != "" {
			t.Fatal("caption mismatch")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal("lost partial-send error")
	}
	post.Text = strings.Repeat("🫪", 2000)
	err = sendYoutubePostPhotos(&post, func(items []clients.AlbumMedia) error {
		if len([]rune(items[0].Caption)) != 1024 {
			t.Fatal("caption is unbounded")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestYoutubePostRejectsEmptyAlbum(t *testing.T) {
	for _, post := range []*api.YoutubePostResult{nil, {}, {Photos: nil}} {
		err := sendYoutubePostPhotos(post, func([]clients.AlbumMedia) error { t.Fatal("sent empty album"); return nil })
		if !errors.Is(err, clients.ErrAlbumLimit) {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestInstagramErrorsHaveSafeReplies(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w: secret URL", api.ErrInstagramUnavailable), "Media Instagram tidak tersedia untuk akses publik, yaa."},
		{fmt.Errorf("%w: secret URL", api.ErrInstagramBusy), "Instagram sedang sibuk. Coba lagi sebentar, yaa."},
		{errors.New("secret URL"), "Gagal mengambil media Instagram. Coba lagi sebentar, yaa."},
	} {
		if got := instagramFailureReply(tc.err); got != tc.want {
			t.Fatalf("got=%q", got)
		}
	}
}
