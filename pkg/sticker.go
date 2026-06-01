package pkg

import (
	"context"
	"fmt"
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

func stc(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if m.Media == nil {
		_, _ = m.Reply(ctx, "Kirim atau balas gambar dulu, yaa.")
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	raw, err := client.WA.Download(opCtx, m.Media)
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

	stc, err := sticker.BuildSticker(opCtx, raw, m.PushName, isSticker, isGif)
	if err != nil {
		if stickerMsg != nil && len(stickerMsg.GetPngThumbnail()) > 0 {
			stc, err = sticker.BuildSticker(opCtx, stickerMsg.GetPngThumbnail(), m.PushName, false, false)
			if err == nil {
				if _, err := client.SendSticker(opCtx, m.From, stc, false, false, m.ID); err != nil {
					_, _ = m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
				}
				return
			}
		}
		if isGif {
			_, _ = m.Reply(ctx, fmt.Sprintf("Sticker animasi/video belum bisa dibuat: %s\n%s", err, sticker.DescribeStickerData(raw, stickerMsg)))
			return
		}
		_, _ = m.Reply(ctx, fmt.Sprintf("Ada yang salah: %s", err))
		return
	}

	if _, err := client.SendSticker(opCtx, m.From, stc, isLottie, isAnimated || isGif, m.ID); err != nil {
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
		Name:     "sticker",
		As:       []string{"s", "stiker"},
		Tags:     "convert",
		IsPrefix: true,
		Exec:     stc,
	})
}
