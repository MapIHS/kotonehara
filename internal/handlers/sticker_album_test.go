package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestSendAlbumStickersConvertsEachItemAndCountsFailures(t *testing.T) {
	items := []whatsmeow.DownloadableMessage{
		&waE2E.ImageMessage{DirectPath: proto.String("ok")},
		&waE2E.VideoMessage{DirectPath: proto.String("big")},
		&waE2E.ImageMessage{DirectPath: proto.String("bad")},
		&waE2E.ImageMessage{DirectPath: proto.String("gif"), Mimetype: proto.String("image/gif")},
	}
	var animatedSends []bool
	deps := albumStickerDeps{
		download: func(_ context.Context, media whatsmeow.DownloadableMessage) ([]byte, error) {
			if media.GetDirectPath() == "bad" {
				return nil, errors.New("expired")
			}
			return []byte(media.GetDirectPath()), nil
		},
		build: func(_ context.Context, raw []byte, _ bool) ([]byte, error) {
			if string(raw) == "big" {
				return nil, sticker.ErrAnimatedStickerTooLarge
			}
			return raw, nil
		},
		send: func(_ context.Context, _ []byte, animated bool) error {
			animatedSends = append(animatedSends, animated)
			return nil
		},
	}
	sent, total, tooLarge := sendAlbumStickers(context.Background(), items, deps)
	if sent != 2 || total != 4 || tooLarge != 1 {
		t.Fatalf("sent=%d total=%d tooLarge=%d", sent, total, tooLarge)
	}
	if len(animatedSends) != 2 || animatedSends[0] || !animatedSends[1] {
		t.Fatalf("animated flags = %v", animatedSends)
	}
}

func TestSendAlbumStickersCapsItems(t *testing.T) {
	items := make([]whatsmeow.DownloadableMessage, maxAlbumStickers+3)
	for i := range items {
		items[i] = &waE2E.ImageMessage{}
	}
	calls := 0
	deps := albumStickerDeps{
		download: func(context.Context, whatsmeow.DownloadableMessage) ([]byte, error) { return []byte{1}, nil },
		build:    func(_ context.Context, raw []byte, _ bool) ([]byte, error) { return raw, nil },
		send:     func(context.Context, []byte, bool) error { calls++; return nil },
	}
	sent, total, _ := sendAlbumStickers(context.Background(), items, deps)
	if sent != maxAlbumStickers || total != maxAlbumStickers || calls != maxAlbumStickers {
		t.Fatalf("sent=%d total=%d calls=%d", sent, total, calls)
	}
}

func TestSendAlbumStickersUsesStickerPack(t *testing.T) {
	items := []whatsmeow.DownloadableMessage{
		&waE2E.ImageMessage{DirectPath: proto.String("img1")},
		&waE2E.ImageMessage{DirectPath: proto.String("img2")},
		&waE2E.VideoMessage{DirectPath: proto.String("vid1")},
	}

	var capturedPack sticker.StickerPack
	sendPackCalls := 0
	sendCalls := 0

	deps := albumStickerDeps{
		download: func(_ context.Context, media whatsmeow.DownloadableMessage) ([]byte, error) {
			return []byte(media.GetDirectPath()), nil
		},
		build: func(_ context.Context, raw []byte, _ bool) ([]byte, error) {
			return raw, nil
		},
		makeThumb: func(_ context.Context, raw []byte, _ bool) ([]byte, error) {
			return append([]byte("thumb-"), raw...), nil
		},
		send: func(context.Context, []byte, bool) error {
			sendCalls++
			return nil
		},
		sendPack: func(_ context.Context, pack sticker.StickerPack) error {
			sendPackCalls++
			capturedPack = pack
			return nil
		},
		packName:  "My Album Pack",
		publisher: "Alice",
	}

	sent, total, tooLarge := sendAlbumStickers(context.Background(), items, deps)
	if sent != 3 || total != 3 || tooLarge != 0 {
		t.Fatalf("sent=%d total=%d tooLarge=%d", sent, total, tooLarge)
	}
	if sendPackCalls != 1 {
		t.Fatalf("sendPackCalls = %d, want 1", sendPackCalls)
	}
	if sendCalls != 0 {
		t.Fatalf("sendCalls = %d, want 0 when pack succeeds", sendCalls)
	}
	if capturedPack.Name != "My Album Pack" || capturedPack.Publisher != "Alice" {
		t.Errorf("pack metadata mismatch: %s / %s", capturedPack.Name, capturedPack.Publisher)
	}
	if len(capturedPack.Stickers) != 3 {
		t.Fatalf("pack stickers count = %d, want 3", len(capturedPack.Stickers))
	}
	if string(capturedPack.CoverThumbnail) != "thumb-img1" {
		t.Errorf("cover thumbnail = %q, want thumb-img1", string(capturedPack.CoverThumbnail))
	}
}

func TestSendAlbumStickersSingleItemUsesSendDirectly(t *testing.T) {
	items := []whatsmeow.DownloadableMessage{
		&waE2E.ImageMessage{DirectPath: proto.String("only-one")},
	}

	sendPackCalls := 0
	sendCalls := 0

	deps := albumStickerDeps{
		download: func(_ context.Context, media whatsmeow.DownloadableMessage) ([]byte, error) {
			return []byte(media.GetDirectPath()), nil
		},
		build: func(_ context.Context, raw []byte, _ bool) ([]byte, error) {
			return raw, nil
		},
		send: func(_ context.Context, data []byte, animated bool) error {
			sendCalls++
			if string(data) != "only-one" || animated {
				t.Errorf("unexpected send data: %s, %v", data, animated)
			}
			return nil
		},
		sendPack: func(context.Context, sticker.StickerPack) error {
			sendPackCalls++
			return nil
		},
	}

	sent, total, _ := sendAlbumStickers(context.Background(), items, deps)
	if sent != 1 || total != 1 {
		t.Fatalf("sent=%d total=%d", sent, total)
	}
	if sendPackCalls != 0 {
		t.Fatalf("sendPackCalls = %d, want 0 for single item", sendPackCalls)
	}
	if sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1 for single item", sendCalls)
	}
}

func TestSendAlbumStickersPackFallbackToIndividual(t *testing.T) {
	items := []whatsmeow.DownloadableMessage{
		&waE2E.ImageMessage{DirectPath: proto.String("s1")},
		&waE2E.ImageMessage{DirectPath: proto.String("s2")},
	}

	sendPackCalls := 0
	sendCalls := 0

	deps := albumStickerDeps{
		download: func(_ context.Context, media whatsmeow.DownloadableMessage) ([]byte, error) {
			return []byte(media.GetDirectPath()), nil
		},
		build: func(_ context.Context, raw []byte, _ bool) ([]byte, error) {
			return raw, nil
		},
		send: func(context.Context, []byte, bool) error {
			sendCalls++
			return nil
		},
		sendPack: func(context.Context, sticker.StickerPack) error {
			sendPackCalls++
			return errors.New("MMS connection refused")
		},
	}

	sent, total, _ := sendAlbumStickers(context.Background(), items, deps)
	if sent != 2 || total != 2 {
		t.Fatalf("sent=%d total=%d", sent, total)
	}
	if sendPackCalls != 1 {
		t.Fatalf("sendPackCalls = %d, want 1", sendPackCalls)
	}
	if sendCalls != 2 {
		t.Fatalf("sendCalls = %d, want 2 on fallback", sendCalls)
	}
}
