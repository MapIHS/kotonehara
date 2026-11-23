package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/telegram"
	"github.com/google/uuid"
)

// maxTelegramStickers bounds one download so a large sticker pack does not
// exceed WhatsApp's sticker pack limit (30 stickers per pack) and keeps RAM flat.
const maxTelegramStickers = 30

type telegramStickerDeps struct {
	getStickerSet func(ctx context.Context, name string) (*telegram.StickerSet, error)
	getFile       func(ctx context.Context, fileID string) (*telegram.FileInfo, error)
	downloadFile  func(ctx context.Context, filePath string) ([]byte, error)
	build         func(ctx context.Context, raw []byte, isLottie bool, isAnimated bool) ([]byte, error)
	makeThumb     func(ctx context.Context, raw []byte, isAnimated bool) ([]byte, error)
	send          func(ctx context.Context, data []byte, isLottie bool, isAnimated bool) error
	sendPack      func(ctx context.Context, pack sticker.StickerPack) error
	pause         time.Duration
	packName      string
	publisher     string
}

func fetchStickerBytes(ctx context.Context, s telegram.Sticker, deps telegramStickerDeps) ([]byte, error) {
	fileID := s.FileID
	if fileID == "" && s.Thumbnail != nil {
		fileID = s.Thumbnail.FileID
	}
	if fileID == "" && s.Thumb != nil {
		fileID = s.Thumb.FileID
	}
	if fileID == "" {
		return nil, errors.New("missing file_id")
	}

	info, err := deps.getFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	return deps.downloadFile(ctx, info.FilePath)
}

func fetchStickerThumbBytes(ctx context.Context, s telegram.Sticker, deps telegramStickerDeps) ([]byte, error) {
	var fileID string
	if s.Thumbnail != nil && s.Thumbnail.FileID != "" {
		fileID = s.Thumbnail.FileID
	} else if s.Thumb != nil && s.Thumb.FileID != "" {
		fileID = s.Thumb.FileID
	}
	if fileID == "" {
		return nil, errors.New("no thumbnail available")
	}
	info, err := deps.getFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	return deps.downloadFile(ctx, info.FilePath)
}

func sendTelegramStickers(ctx context.Context, set *telegram.StickerSet, deps telegramStickerDeps) (sent, total, tooLarge int) {
	if set == nil || len(set.Stickers) == 0 {
		return 0, 0, 0
	}

	stickers := set.Stickers
	if len(stickers) > maxTelegramStickers {
		stickers = stickers[:maxTelegramStickers]
	}
	total = len(stickers)

	// If no sticker pack sender is configured, use legacy sequential sending
	if deps.sendPack == nil {
		for i, s := range stickers {
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
			isLottie := s.IsAnimated
			isAnimated := s.IsAnimated || s.IsVideo
			err := func() error {
				opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()
				raw, err := fetchStickerBytes(opCtx, s, deps)
				if err != nil || len(raw) == 0 {
					return errors.New("download failed")
				}
				data, err := deps.build(opCtx, raw, isLottie, isAnimated)
				if err != nil {
					return err
				}
				return deps.send(opCtx, data, isLottie, isAnimated)
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

	for _, s := range stickers {
		if ctx.Err() != nil {
			return
		}
		isLottie := s.IsAnimated
		isAnimated := s.IsAnimated || s.IsVideo
		err := func() error {
			opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			raw, err := fetchStickerBytes(opCtx, s, deps)
			if err != nil || len(raw) == 0 {
				return errors.New("download failed")
			}
			data, err := deps.build(opCtx, raw, isLottie, isAnimated)
			if err != nil {
				return err
			}

			if len(firstThumb) == 0 && deps.makeThumb != nil {
				if isLottie {
					if thumbRaw, err := fetchStickerThumbBytes(opCtx, s, deps); err == nil && len(thumbRaw) > 0 {
						if thumb, err := deps.makeThumb(opCtx, thumbRaw, false); err == nil && len(thumb) > 0 {
							firstThumb = thumb
						}
					}
				} else {
					if thumb, err := deps.makeThumb(opCtx, raw, isAnimated); err == nil && len(thumb) > 0 {
						firstThumb = thumb
					}
				}
			}

			fileName := fmt.Sprintf("sticker_%d.webp", len(converted)+1)
			mimeType := "image/webp"
			if isLottie {
				fileName = fmt.Sprintf("sticker_%d.was", len(converted)+1)
				mimeType = "application/was"
			}

			emojis := []string{"✨"}
			if s.Emoji != "" {
				emojis = []string{s.Emoji}
			}

			converted = append(converted, sticker.StickerPackItem{
				FileName:   fileName,
				Data:       data,
				IsAnimated: isAnimated,
				IsLottie:   isLottie,
				Mimetype:   mimeType,
				Emojis:     emojis,
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
		if err := deps.send(ctx, converted[0].Data, converted[0].IsLottie, converted[0].IsAnimated); err == nil {
			sent = 1
		}
		return sent, total, tooLarge
	}

	// Multiple stickers: send as Sticker Pack
	packName := deps.packName
	if packName == "" {
		packName = set.Title
		if packName == "" {
			packName = set.Name
		}
		if packName == "" {
			packName = "Sticker Pack"
		}
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
	log.Print("telegram sticker pack send failed; falling back to individual stickers")
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
			if err := deps.send(ctx, s.Data, s.IsLottie, s.IsAnimated); err == nil {
				sent++
			}
		}
	}

	return sent, total, tooLarge
}

func handleTelegramSticker(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config, packQuery string) {
	packName := telegram.ExtractPackName(packQuery)
	if packName == "" {
		_, _ = m.Reply(ctx, "Kirim link sticker pack Telegram yang ingin diunduh, yaa.\nContoh: .telesticker https://t.me/addstickers/animals")
		return
	}

	if cfg.TelegramBotToken == "" {
		_, _ = m.Reply(ctx, "TELEGRAM_BOT_TOKEN belum diatur di env, yaa.")
		return
	}

	tgClient := telegram.New(cfg.TelegramBotToken, 60*time.Second)

	_, _ = m.Reply(ctx, "Tunggu sebentar, sedang mengunduh stiker Telegram...")

	set, err := tgClient.GetStickerSet(ctx, packName)
	if err != nil {
		_, _ = m.Reply(ctx, telegramStickerFailureReply(err))
		return
	}

	publisher := m.PushName
	if publisher == "" {
		publisher = "Kotonehara"
	}
	packTitle := set.Title
	if packTitle == "" {
		packTitle = set.Name
	}

	deps := telegramStickerDeps{
		getStickerSet: func(opCtx context.Context, name string) (*telegram.StickerSet, error) {
			return tgClient.GetStickerSet(opCtx, name)
		},
		getFile: func(opCtx context.Context, fileID string) (*telegram.FileInfo, error) {
			return tgClient.GetFile(opCtx, fileID)
		},
		downloadFile: func(opCtx context.Context, filePath string) ([]byte, error) {
			return tgClient.DownloadFile(opCtx, filePath)
		},
		build: func(opCtx context.Context, raw []byte, isLottie bool, isAnimated bool) ([]byte, error) {
			if isLottie {
				return sticker.BuildLottieSticker(raw, publisher)
			}
			return sticker.BuildSticker(opCtx, raw, publisher, !isAnimated, isAnimated)
		},
		makeThumb: func(opCtx context.Context, raw []byte, animated bool) ([]byte, error) {
			if animated {
				return client.MakeVideoThumb(opCtx, raw, 252, 252)
			}
			return client.MakeJPEGThumb(raw, 252, 252)
		},
		send: func(opCtx context.Context, data []byte, isLottie bool, isAnimated bool) error {
			_, err := client.SendSticker(opCtx, m.From, data, isLottie, isAnimated, m.ID)
			return err
		},
		sendPack: func(opCtx context.Context, pack sticker.StickerPack) error {
			_, err := client.SendStickerPack(opCtx, m.From, pack, m.ID)
			return err
		},
		pause:     400 * time.Millisecond,
		packName:  packTitle,
		publisher: publisher,
	}

	sent, total, tooLarge := sendTelegramStickers(ctx, set, deps)
	if sent == total && total > 0 {
		return
	}
	log.Printf("telegram sticker: sent=%d requested=%d tooLarge=%d", sent, total, tooLarge)
	switch {
	case sent == 0 && tooLarge > 0 && tooLarge == total:
		_, _ = m.Reply(ctx, "Stiker animasi melebihi 500 KB dan belum bisa dikonversi, yaa.")
	case sent == 0:
		_, _ = m.Reply(ctx, "Stiker dari pack Telegram ini belum bisa dibuat, yaa.")
	default:
		_, _ = m.Reply(ctx, fmt.Sprintf("%d dari %d stiker berhasil dibuat, yaa.", sent, total))
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:        "telesticker",
		Description: "Download sticker pack dari link Telegram: <link atau nama pack>",
		As:          []string{"tgsticker", "telegramsticker", "stickertel", "stickertg"},
		Tags:        "downloader",
		IsPrefix:    true,
		IsQuery:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			query := strings.TrimSpace(m.Query)
			if query == "" && m.QuotedMsg != nil {
				query = strings.TrimSpace(m.QuotedText())
			}
			handleTelegramSticker(ctx, client, m, cfg, query)
		},
	})
}

func telegramStickerFailureReply(err error) string {
	switch {
	case errors.Is(err, telegram.ErrStickerSetNotFound):
		return "Sticker pack Telegram tidak ditemukan, yaa."
	case errors.Is(err, telegram.ErrStickerSetEmpty):
		return "Sticker pack Telegram ini kosong, yaa."
	default:
		return "Gagal mengambil stiker Telegram. Coba lagi sebentar, yaa."
	}
}
