package pkg

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/s3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

const toURLTimeout = 60 * time.Second

type toURLMedia struct {
	media    whatsmeow.DownloadableMessage
	filename string
	mimeType string
	kind     string
}

func toURLCmd(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if strings.TrimSpace(cfg.BASES3URL) == "" {
		m.Reply(ctx, "BASES3_URL belum diatur, yaa.")
		return
	}

	target, ok := pickToURLMedia(m)
	if !ok {
		m.Reply(ctx, "Kirim atau balas media dulu, yaa.")
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, toURLTimeout)
	defer cancel()

	data, err := client.WA.Download(opCtx, target.media)
	if err != nil || len(data) == 0 {
		m.Reply(ctx, "Media belum bisa diambil, yaa.")
		return
	}

	filename := toURLFilename(target, data)
	publicURL, err := s3.New(cfg.BASES3URL, toURLTimeout).Upload(filename, data)
	if err != nil {
		m.Reply(ctx, "Gagal upload media: "+err.Error())
		return
	}

	m.Reply(ctx, publicURL)
}

func pickToURLMedia(m *message.Message) (toURLMedia, bool) {
	if m == nil {
		return toURLMedia{}, false
	}
	if media, ok := toURLMediaFromMessage(m.QuotedMsg); ok {
		return media, true
	}
	return toURLMediaFromMessage(m.Message)
}

func toURLMediaFromMessage(msg *waE2E.Message) (toURLMedia, bool) {
	if msg == nil {
		return toURLMedia{}, false
	}
	if img := msg.GetImageMessage(); img != nil {
		return toURLMedia{media: img, mimeType: img.GetMimetype(), kind: "image"}, true
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return toURLMedia{media: vid, mimeType: vid.GetMimetype(), kind: "video"}, true
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return toURLMedia{media: aud, mimeType: aud.GetMimetype(), kind: "audio"}, true
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return toURLMedia{media: doc, filename: doc.GetFileName(), mimeType: doc.GetMimetype(), kind: "document"}, true
	}
	if st := msg.GetStickerMessage(); st != nil {
		return toURLMedia{media: st, mimeType: st.GetMimetype(), kind: "sticker"}, true
	}
	if st := msg.GetLottieStickerMessage().GetMessage().GetStickerMessage(); st != nil {
		return toURLMedia{media: st, mimeType: st.GetMimetype(), kind: "sticker"}, true
	}
	return toURLMedia{}, false
}

func toURLFilename(media toURLMedia, data []byte) string {
	name := sanitizeFilename(media.filename)
	if name == "file" || name == "" {
		name = fmt.Sprintf("%s-%d", media.kind, time.Now().UnixNano())
	}

	if filepath.Ext(name) != "" {
		return name
	}

	mimeType := strings.TrimSpace(media.mimeType)
	if mimeType == "" {
		mimeType = detectToURLMime(data)
	}
	if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		return name + exts[0]
	}
	switch mimeType {
	case "image/webp":
		return name + ".webp"
	case "video/mp4":
		return name + ".mp4"
	case "audio/ogg":
		return name + ".ogg"
	case "audio/mpeg":
		return name + ".mp3"
	default:
		return name + ".bin"
	}
}

func detectToURLMime(data []byte) string {
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return http.DetectContentType(data)
}

func init() {
	commands.Register(&commands.Command{
		Name:     "tourl",
		As:       []string{"url", "upload"},
		Tags:     "tools",
		IsPrefix: true,
		Exec:     toURLCmd,
	})
}
