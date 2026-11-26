package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

const stickerCropUsage = "Opsi: -c center/atas/bawah/kiri/kanan/fit. Contoh: -c atas."

func parseStickerCrop(query string) (sticker.Crop, bool) {
	args := strings.Fields(strings.ToLower(query))
	if len(args) == 0 {
		return sticker.CropDefault, true
	}
	if len(args) != 2 || (args[0] != "-c" && args[0] != "--crop") {
		return sticker.CropDefault, false
	}
	switch args[1] {
	case "center", "tengah":
		return sticker.CropCenter, true
	case "top", "atas":
		return sticker.CropTop, true
	case "bottom", "bawah":
		return sticker.CropBottom, true
	case "left", "kiri":
		return sticker.CropLeft, true
	case "right", "kanan":
		return sticker.CropRight, true
	case "fit":
		return sticker.CropFit, true
	default:
		return sticker.CropDefault, false
	}
}

func stc(ctx context.Context, client *clients.Client, m *message.Message, _ config.Config) {
	crop, ok := parseStickerCrop(m.Query)
	if !ok {
		_, _ = m.Reply(ctx, stickerCropUsage)
		return
	}
	if m.Media == nil && m.QuotedAlbumID != "" {
		stickerFromAlbum(ctx, client, m, crop)
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	raw, err := client.DownloadMedia(opCtx, m)
	if err != nil || len(raw) == 0 {
		_, _ = m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
		return
	}

	isGif := false

	if m.IsVideo || m.IsQuotedVideo {
		isGif = true
	} else if m.IsGif || m.IsQuotedGif {
		isGif = true
	} else if m.IsQuotedStickerGif {
		isGif = true
	}

	stickerMsg := stickerMessageFromCommand(m)
	isSticker, isAnimated, isLottie := stickerFlags(stickerMsg)
	if m.Msg != nil && m.Msg.IsLottieSticker {
		isSticker = true
		isAnimated = true
		isLottie = true
	}
	if stickerMsg != nil && stickerMsg.GetMimetype() == "application/was" {
		isLottie = true
	}

	if isLottie {
		if crop != sticker.CropDefault {
			_, _ = m.Reply(ctx, "Crop belum tersedia untuk stiker Lottie, yaa.")
			return
		}
		stc, err := sticker.BuildLottieSticker(raw, m.PushName)
		if err != nil {
			_, _ = m.Reply(ctx, fmt.Sprintf("Lottie/WAS belum bisa diedit: %s\n%s", err, sticker.DescribeStickerData(raw, stickerMsg)))
			return
		}
		if _, err := client.SendSticker(opCtx, m.From, stc, true, true, m.ID); err != nil {
			_, _ = m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
		}
		return
	}

	stc, err := sticker.BuildStickerWithCrop(opCtx, raw, m.PushName, isSticker, isGif, crop)
	if err != nil {
		if errors.Is(err, sticker.ErrAnimatedStickerTooLarge) {
			_, _ = m.Reply(ctx, "Stiker animasinya masih lebih dari 1 MB. Coba potong GIF jadi lebih pendek, yaa.")
			return
		}
		if stickerMsg != nil && len(stickerMsg.GetPngThumbnail()) > 0 {
			stc, err = sticker.BuildStickerWithCrop(opCtx, stickerMsg.GetPngThumbnail(), m.PushName, false, false, crop)
			if err == nil {
				if _, err := client.SendSticker(opCtx, m.From, stc, false, false, m.ID); err != nil {
					_, _ = m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
				}
				return
			}
		}
		if isGif || isAnimated || sticker.IsGIF(raw) {
			_, _ = m.Reply(ctx, fmt.Sprintf("Sticker animasi/video belum bisa dibuat: %s\n%s", err, sticker.DescribeStickerData(raw, stickerMsg)))
			return
		}
		_, _ = m.Reply(ctx, fmt.Sprintf("Ada yang salah: %s", err))
		return
	}

	if _, err := client.SendSticker(opCtx, m.From, stc, false, sticker.IsAnimatedWebP(stc), m.ID); err != nil {
		_, _ = m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
		return
	}
}

func stickerFlags(st *waE2E.StickerMessage) (isSticker, isAnimated, isLottie bool) {
	if st == nil {
		return false, false, false
	}
	return true, st.GetIsAnimated(), st.GetIsLottie()
}

func stickerMessageFromCommand(m *message.Message) *waE2E.StickerMessage {
	if st := stickerMessage(m.QuotedMsg); st != nil {
		return st
	}
	return stickerMessage(m.Message)
}

func stickerMessage(msg *waE2E.Message) *waE2E.StickerMessage {
	if msg == nil {
		return nil
	}
	if st := msg.GetStickerMessage(); st != nil {
		return st
	}
	return msg.GetLottieStickerMessage().GetMessage().GetStickerMessage()
}

func init() {
	commands.Register(&commands.Command{
		Name:        "sticker",
		Description: "Ubah gambar, video, atau GIF menjadi stiker. " + stickerCropUsage,
		As:          []string{"s", "stiker"},
		Tags:        "convert",
		IsPrefix:    true,
		IsMedia:     true,
		AllowAlbum:  true,
		Exec:        stc,
	})
}
