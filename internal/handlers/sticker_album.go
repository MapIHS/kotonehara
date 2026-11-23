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
	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

// maxAlbumStickers bounds one .s reply so a large album cannot monopolise the
// bot; media are downloaded and converted one at a time to keep RAM flat.
const maxAlbumStickers = 10

type albumStickerDeps struct {
	download  func(ctx context.Context, media whatsmeow.DownloadableMessage) ([]byte, error)
	build     func(ctx context.Context, raw []byte, animated bool) ([]byte, error)
	makeThumb func(ctx context.Context, raw []byte, animated bool) ([]byte, error)
	send      func(ctx context.Context, data []byte, animated bool) error
	sendPack  func(ctx context.Context, pack sticker.StickerPack) error
	pause     time.Duration
	packName  string
	publisher string
}

func stickerFromAlbum(ctx context.Context, client *clients.Client, m *message.Message) {
	items := client.AlbumMedia(m.From, m.QuotedAlbumID)
	if len(items) == 0 {
		_, _ = m.Reply(ctx, "Foto di album ini belum tersimpan (mungkin dikirim sebelum bot aktif). Balas salah satu fotonya langsung, yaa.")
		return
	}

	packName := fmt.Sprintf("Album %s", m.PushName)
	if m.PushName == "" {
		packName = "Album Stiker"
	}
	publisher := m.PushName
	if publisher == "" {
		publisher = "Kotonehara"
	}

	deps := albumStickerDeps{
		download: func(ctx context.Context, media whatsmeow.DownloadableMessage) ([]byte, error) {
			return client.WA.Download(ctx, media)
		},
		build: func(ctx context.Context, raw []byte, animated bool) ([]byte, error) {
			return sticker.BuildSticker(ctx, raw, m.PushName, false, animated)
		},
		makeThumb: func(ctx context.Context, raw []byte, animated bool) ([]byte, error) {
			if animated {
				return client.MakeVideoThumb(ctx, raw, 252, 252)
			}
			return client.MakeJPEGThumb(raw, 252, 252)
		},
		send: func(ctx context.Context, data []byte, animated bool) error {
			_, err := client.SendSticker(ctx, m.From, data, false, animated, m.ID)
			return err
		},
		sendPack: func(ctx context.Context, pack sticker.StickerPack) error {
			_, err := client.SendStickerPack(ctx, m.From, pack, m.ID)
			return err
		},
		pause:     400 * time.Millisecond,
		packName:  packName,
		publisher: publisher,
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

	// If no sticker pack sender is configured, use legacy sequential sending
	if deps.sendPack == nil {
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

	// When sendPack is available: download and build all stickers first
	var converted []sticker.StickerPackItem
	var firstThumb []byte

	for _, media := range items {
		if ctx.Err() != nil {
			return
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
			if len(firstThumb) == 0 && deps.makeThumb != nil {
				if thumb, err := deps.makeThumb(opCtx, raw, animated); err == nil && len(thumb) > 0 {
					firstThumb = thumb
				}
			}
			converted = append(converted, sticker.StickerPackItem{
				FileName:   fmt.Sprintf("sticker_%d.webp", len(converted)+1),
				Data:       data,
				IsAnimated: animated,
				Mimetype:   "image/webp",
				Emojis:     []string{"✨"},
			})
			return nil
		}()
		if err != nil && errors.Is(err, sticker.ErrAnimatedStickerTooLarge) {
			tooLarge++
		}
	}

	if len(converted) == 0 {
		return 0, total, tooLarge
	}

	// Single sticker: send individually
	if len(converted) == 1 && deps.send != nil {
		if err := deps.send(ctx, converted[0].Data, converted[0].IsAnimated); err == nil {
			sent = 1
		}
		return sent, total, tooLarge
	}

	// Multiple stickers: send as Sticker Pack
	packName := deps.packName
	if packName == "" {
		packName = "Sticker Pack"
	}
	publisher := deps.publisher
	if publisher == "" {
		publisher = "Kotonehara"
	}

	pack := sticker.StickerPack{
		ID:             uuid.NewString(),
		Name:           packName,
		Publisher:      publisher,
		Stickers:       converted,
		CoverThumbnail: firstThumb,
	}

	err := deps.sendPack(ctx, pack)
	if err == nil {
		sent = len(converted)
		return sent, total, tooLarge
	}

	// Fallback to sending individual stickers if sticker pack transmission fails
	log.Printf("album sticker pack send failed: %v, falling back to individual stickers", err)
	if deps.send != nil {
		for i, s := range converted {
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
			if err := deps.send(ctx, s.Data, s.IsAnimated); err == nil {
				sent++
			}
		}
	}

	return sent, total, tooLarge
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
