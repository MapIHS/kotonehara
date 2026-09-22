package clients

import (
	"context"
	"fmt"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func (c *Client) uploadMediaFile(ctx context.Context, file *os.File, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, string, error) {
	var empty whatsmeow.UploadResponse
	info, err := file.Stat()
	if err != nil {
		return empty, "", err
	}
	if info.Size() <= 0 || info.Size() > maxFetchedMediaSize {
		return empty, "", fmt.Errorf("ukuran media tidak valid")
	}
	var header [512]byte
	n, err := file.ReadAt(header[:], 0)
	if err != nil && err != io.EOF {
		return empty, "", err
	}
	mime := http.DetectContentType(header[:n])
	if kind == whatsmeow.MediaVideo && mime != "video/mp4" {
		return empty, "", fmt.Errorf("file bukan video MP4")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return empty, "", err
	}
	up, err := c.WA.UploadReader(ctx, file, nil, kind)
	return up, mime, err
}
func (c *Client) SendVideoFile(ctx context.Context, to types.JID, file *os.File, gifPlayback bool, caption string, opts *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	up, mime, err := c.uploadMediaFile(ctx, file, whatsmeow.MediaVideo)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	// Extract video metadata (best-effort, non-fatal)
	meta := probeVideoFile(ctx, file.Name())

	// Generate JPEG thumbnail from first frame (best-effort)
	var thumb []byte
	dir, tempErr := os.MkdirTemp("", "kotonehara-thumb-*")
	if tempErr == nil {
		defer os.RemoveAll(dir)
		thumb, _ = c.makeVideoThumbFile(ctx, file.Name(), filepath.Join(dir, "thumb.jpg"), 256, 256)
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			Caption:       proto.String(caption),
			GifPlayback:   proto.Bool(gifPlayback),
			Mimetype:      proto.String(mime),
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
			JPEGThumbnail: thumb,
			ContextInfo:   opts,
		},
	}

	if meta.Width > 0 {
		msg.VideoMessage.Width = proto.Uint32(meta.Width)
	}
	if meta.Height > 0 {
		msg.VideoMessage.Height = proto.Uint32(meta.Height)
	}
	if meta.Duration > 0 {
		msg.VideoMessage.Seconds = proto.Uint32(meta.Duration)
	}

	return c.WA.SendMessage(ctx, to, msg)
}

func (c *Client) SendAudioFile(ctx context.Context, to types.JID, file *os.File, ptt bool, opts *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	up, mime, err := c.uploadMediaFile(ctx, file, whatsmeow.MediaAudio)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			Mimetype:      proto.String(mime),
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    &up.FileLength,
			PTT:           proto.Bool(ptt),
			ContextInfo:   opts,
		},
	}

	return c.WA.SendMessage(ctx, to, msg)
}

func (c *Client) SendDocumentFile(ctx context.Context, to types.JID, file *os.File, filename, caption string, opts *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	up, mime, err := c.uploadMediaFile(ctx, file, whatsmeow.MediaDocument)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileName:      proto.String(filename),
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mime),
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
			ContextInfo:   opts,
		},
	}
	return c.WA.SendMessage(ctx, to, msg)
}
