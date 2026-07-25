package pkg

import (
	"context"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

const (
	sourceCodeURL       = "https://github.com/MapIHS/kotonehara"
	sourceCodeThumbnail = "https://repository-images.githubusercontent.com/1123906229/9268b94b-d044-46bb-b25d-273a9ddeb6c5"
	sourceCodeTitle     = "GitHub - MapIHS/kotonehara: Bot Whatsapp Use Whatsmeow library"
	sourceCodeDesc      = "Bot Whatsapp Use Whatsmeow library. Contribute to MapIHS/kotonehara development by creating an account on GitHub."
)

var sourceCodePreviewCache struct {
	sync.Mutex
	thumbnail []byte
	width     uint32
	height    uint32
}

func init() {
	commands.Register(&commands.Command{
		Name:     "sc",
		As:       []string{"sourcecode"},
		Tags:     "main",
		IsPrefix: true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			thumbnail, width, height := sourceCodePreviewThumbnail(client)
			preview := sourceCodePreviewMessage(thumbnail, m.ID)
			attachHighQualityPreview(ctx, client, preview, thumbnail, width, height)
			_, _ = client.WA.SendMessage(ctx, m.From, &waE2E.Message{
				ExtendedTextMessage: preview,
			})
		},
	})
}

func sourceCodePreviewThumbnail(client *clients.Client) ([]byte, uint32, uint32) {
	if client == nil {
		return nil, 0, 0
	}
	sourceCodePreviewCache.Lock()
	defer sourceCodePreviewCache.Unlock()
	if len(sourceCodePreviewCache.thumbnail) > 0 {
		return sourceCodePreviewCache.thumbnail, sourceCodePreviewCache.width, sourceCodePreviewCache.height
	}

	data, err := client.FetchBytes(sourceCodeThumbnail)
	if err != nil {
		return nil, 0, 0
	}
	thumbnail, err := client.MakeJPEGThumb(data, 1200, 630)
	if err != nil {
		return nil, 0, 0
	}
	_, width, height, err := client.DetectImageInfo(thumbnail)
	if err != nil || width <= 0 || height <= 0 {
		return thumbnail, 0, 0
	}
	sourceCodePreviewCache.thumbnail = thumbnail
	sourceCodePreviewCache.width = uint32(width)
	sourceCodePreviewCache.height = uint32(height)
	return sourceCodePreviewCache.thumbnail, sourceCodePreviewCache.width, sourceCodePreviewCache.height
}

// attachHighQualityPreview uploads the thumbnail using WhatsApp's link-preview
// media type. JPEGThumbnail alone renders as the compact preview; these fields
// enable the large preview card used by current WhatsApp clients.
func attachHighQualityPreview(ctx context.Context, client *clients.Client, preview *waE2E.ExtendedTextMessage, thumbnail []byte, width, height uint32) {
	if client == nil || client.WA == nil || preview == nil || len(thumbnail) == 0 || width == 0 || height == 0 {
		return
	}
	upload, err := client.WA.Upload(ctx, thumbnail, whatsmeow.MediaLinkThumbnail)
	if err != nil {
		return
	}
	preview.ThumbnailDirectPath = proto.String(upload.DirectPath)
	preview.ThumbnailSHA256 = upload.FileSHA256
	preview.ThumbnailEncSHA256 = upload.FileEncSHA256
	preview.MediaKey = upload.MediaKey
	preview.MediaKeyTimestamp = proto.Int64(time.Now().Unix())
	preview.ThumbnailWidth = proto.Uint32(width)
	preview.ThumbnailHeight = proto.Uint32(height)
}

func sourceCodePreviewMessage(thumbnail []byte, contextInfo *waE2E.ContextInfo) *waE2E.ExtendedTextMessage {
	text := sourceCodeURL + "\nhttps://github.com/sanxlab/hararest\nhttps://github.com/MapIHS/removebg-be\n\nPake ajh, jangan lupa kasih stars"
	return &waE2E.ExtendedTextMessage{
		Text:          proto.String(text),
		MatchedText:   proto.String(sourceCodeURL),
		Title:         proto.String(sourceCodeTitle),
		Description:   proto.String(sourceCodeDesc),
		JPEGThumbnail: thumbnail,
		ContextInfo:   contextInfo,
	}
}
