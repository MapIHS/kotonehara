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
