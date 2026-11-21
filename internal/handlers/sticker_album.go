package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

// maxAlbumStickers bounds one .s reply so a large album cannot monopolise the
// bot; media are downloaded and converted one at a time to keep RAM flat.
const maxAlbumStickers = 10

type albumStickerDeps struct {
	download func(ctx context.Context, media whatsmeow.DownloadableMessage) ([]byte, error)
	build    func(ctx context.Context, raw []byte, animated bool) ([]byte, error)
	send     func(ctx context.Context, data []byte, animated bool) error
	pause    time.Duration
}

func stickerFromAlbum(ctx context.Context, client *clients.Client, m *message.Message) {
	items := client.AlbumMedia(m.From, m.QuotedAlbumID)
	if len(items) == 0 {
		_, _ = m.Reply(ctx, "Foto di album ini belum tersimpan (mungkin dikirim sebelum bot aktif). Balas salah satu fotonya langsung, yaa.")
		return
	}
	deps := albumStickerDeps{
		download: func(ctx context.Context, media whatsmeow.DownloadableMessage) ([]byte, error) {
			return client.WA.Download(ctx, media)
		},
		build: func(ctx context.Context, raw []byte, animated bool) ([]byte, error) {
			return sticker.BuildSticker(ctx, raw, m.PushName, false, animated)
		},
		send: func(ctx context.Context, data []byte, animated bool) error {
			_, err := client.SendSticker(ctx, m.From, data, false, animated, m.ID)
			return err
		},
		pause: 400 * time.Millisecond,
	}
	sent, total, tooLarge := sendAlbumStickers(ctx, items, deps)
	if sent == total {
		return
	}
	log.Printf("album sticker: sent=%d requested=%d", sent, total)
	switch {
	case sent == 0 && tooLarge > 0 && tooLarge == total:
		_, _ = m.Reply(ctx, "Stiker animasinya masih lebih dari 500 KB. Coba potong videonya jadi lebih pendek, yaa.")
	case sent == 0:
		_, _ = m.Reply(ctx, "Stiker dari album ini belum bisa dibuat, yaa.")
	default:
		_, _ = m.Reply(ctx, fmt.Sprintf("%d dari %d stiker berhasil dibuat, yaa.", sent, total))
	}
}

func sendAlbumStickers(ctx context.Context, items []whatsmeow.DownloadableMessage, deps albumStickerDeps) (sent, total, tooLarge int) {
	if len(items) > maxAlbumStickers {
		items = items[:maxAlbumStickers]
	}
	total = len(items)
	for i, media := range items {
		if ctx.Err() != nil {
			return
		}
		if i > 0 && deps.pause > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(deps.pause):
			}
		}
		animated := isAnimatedAlbumMedia(media)
		err := func() error {
			opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			raw, err := deps.download(opCtx, media)
			if err != nil || len(raw) == 0 {
				return errors.New("download failed")
			}
			data, err := deps.build(opCtx, raw, animated)
			if err != nil {
				return err
			}
			return deps.send(opCtx, data, animated)
		}()
		if err == nil {
			sent++
		} else if errors.Is(err, sticker.ErrAnimatedStickerTooLarge) {
			tooLarge++
		}
	}
	return
}

func isAnimatedAlbumMedia(media whatsmeow.DownloadableMessage) bool {
	switch v := media.(type) {
	case *waE2E.VideoMessage:
		return true
	case *waE2E.ImageMessage:
		return v.GetMimetype() == "image/gif"
	default:
		return false
	}
}
