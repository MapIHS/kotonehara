package handlers

import (
	"errors"
	"reflect"
	"testing"
)

func TestTikTokPhotoPostWithNilVideo(t *testing.T) {
	var captions []string
	var images []string
	fetch := func(url string) ([]byte, error) {
		if url == "broken" {
			return nil, errors.New("download failed")
		}
		return []byte(url), nil
	}
	image := func(data []byte, caption string) error {
		images = append(images, string(data))
		captions = append(captions, caption)
		return nil
	}
	video := func([]byte, string) error { t.Fatal("photo post sent as video"); return nil }
	err := sendTikTokMedia([]string{"broken", "first", "second"}, nil, "caption", fetch, image, video)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(images, []string{"first", "second"}) || !reflect.DeepEqual(captions, []string{"caption", ""}) {
		t.Fatalf("images=%v captions=%v", images, captions)
	}
}

func TestTikTokMissingMedia(t *testing.T) {
	fetch := func(string) ([]byte, error) { t.Fatal("should not fetch missing media"); return nil, nil }
	send := func([]byte, string) error { t.Fatal("should not send missing media"); return nil }
	if err := sendTikTokMedia(nil, nil, "", fetch, send, send); err == nil {
		t.Fatal("missing media must return an error")
	}
}

func TestTikTokVideoSendFailure(t *testing.T) {
	url := "https://cdn.example/video.mp4"
	failure := errors.New("send failed")
	fetch := func(string) ([]byte, error) { return []byte("video"), nil }
	image := func([]byte, string) error { t.Fatal("video sent as image"); return nil }
	video := func([]byte, string) error { return failure }
	err := sendTikTokMedia(nil, &url, "", fetch, image, video)
	if !errors.Is(err, failure) {
		t.Fatalf("expected send failure, got %v", err)
	}
}
