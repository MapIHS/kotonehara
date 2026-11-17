package clients

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const MaxViewOnceBytes = 64 << 20

var (
	ErrViewOnceTooLarge    = errors.New("view-once media exceeds 64 MiB")
	ErrViewOnceUnavailable = errors.New("view-once media metadata is incomplete")
	ErrViewOnceUnsupported = errors.New("view-once media type is unsupported")
	viewOnceSlots          = make(chan struct{}, 2)
)

type viewOnceTransport interface {
	DownloadToFile(context.Context, whatsmeow.DownloadableMessage, whatsmeow.File) error
	UploadReader(context.Context, io.Reader, io.ReadWriteSeeker, whatsmeow.MediaType) (whatsmeow.UploadResponse, error)
	SendMessage(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

// SendViewOnceAsNormal streams through temporary files without image decoding,
// transcoding, or reading the entire attachment into a byte slice.
func (c *Client) SendViewOnceAsNormal(ctx context.Context, to types.JID, raw *waE2E.Message, quote *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	if c == nil || c.WA == nil {
		return whatsmeow.SendResponse{}, ErrViewOnceUnavailable
	}
	return sendViewOnceAsNormal(ctx, c.WA, to, raw, quote)
}

func sendViewOnceAsNormal(ctx context.Context, transport viewOnceTransport, to types.JID, raw *waE2E.Message, quote *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	var empty whatsmeow.SendResponse
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if raw == nil {
		return empty, ErrViewOnceUnavailable
	}
	// Unwrapping and later rebuilding must not mutate the source/store snapshot.
	source := message.UnwrapMessage(proto.Clone(raw).(*waE2E.Message))
	var media whatsmeow.DownloadableMessage
	var kind whatsmeow.MediaType
	var length uint64
	switch {
	case source.GetImageMessage() != nil:
		media = source.GetImageMessage()
		kind = whatsmeow.MediaImage
		length = source.GetImageMessage().GetFileLength()
	case source.GetVideoMessage() != nil:
		media = source.GetVideoMessage()
		kind = whatsmeow.MediaVideo
		length = source.GetVideoMessage().GetFileLength()
	case source.GetAudioMessage() != nil:
		media = source.GetAudioMessage()
		kind = whatsmeow.MediaAudio
		length = source.GetAudioMessage().GetFileLength()
	default:
		return empty, ErrViewOnceUnsupported
	}
	if length > MaxViewOnceBytes {
		return empty, ErrViewOnceTooLarge
	}
	if media.GetDirectPath() == "" || len(media.GetMediaKey()) != 32 || len(media.GetFileSHA256()) != 32 || len(media.GetFileEncSHA256()) != 32 {
		return empty, ErrViewOnceUnavailable
	}
	select {
	case viewOnceSlots <- struct{}{}:
	case <-ctx.Done():
		return empty, ctx.Err()
	}
	defer func() { <-viewOnceSlots }()
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	file, err := os.CreateTemp("", "kotonehara-rvo-*")
	if err != nil {
		return empty, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	// Permit CBC padding and MAC on the encrypted download; enforce plaintext
	// size independently after decryption. The wrapper also prevents preallocation.
	bounded := &viewOnceFile{File: file, max: MaxViewOnceBytes + 32}
	if err := transport.DownloadToFile(ctx, media, bounded); err != nil {
		return empty, fmt.Errorf("download view-once: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return empty, err
	}
	if info.Size() > MaxViewOnceBytes {
		return empty, ErrViewOnceTooLarge
	}
	if info.Size() == 0 {
		return empty, ErrViewOnceUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return empty, err
	}
	up, err := transport.UploadReader(ctx, file, nil, kind)
	if err != nil {
		return empty, fmt.Errorf("upload view-once: %w", err)
	}
	// Build a clean ordinary media message; no view-once wrapper, original
	// message association, reply context, or stale media/thumbnail download keys.
	out := normalViewOnceMedia(source, up, quote)
	return transport.SendMessage(ctx, to, out)
}

func normalViewOnceMedia(source *waE2E.Message, up whatsmeow.UploadResponse, quote *waE2E.ContextInfo) *waE2E.Message {
	stamp := proto.Int64(time.Now().Unix())
	switch {
	case source.GetImageMessage() != nil:
		m := source.GetImageMessage()
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileSHA256: up.FileSHA256, FileEncSHA256: up.FileEncSHA256, FileLength: &up.FileLength, MediaKeyTimestamp: stamp, Caption: m.Caption, Mimetype: m.Mimetype, Width: m.Width, Height: m.Height, JPEGThumbnail: m.JPEGThumbnail, ContextInfo: quote}}
	case source.GetVideoMessage() != nil:
		m := source.GetVideoMessage()
		return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileSHA256: up.FileSHA256, FileEncSHA256: up.FileEncSHA256, FileLength: &up.FileLength, MediaKeyTimestamp: stamp, Caption: m.Caption, Mimetype: m.Mimetype, Width: m.Width, Height: m.Height, Seconds: m.Seconds, GifPlayback: m.GifPlayback, JPEGThumbnail: m.JPEGThumbnail, ContextInfo: quote}}
	default:
		m := source.GetAudioMessage()
		return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileSHA256: up.FileSHA256, FileEncSHA256: up.FileEncSHA256, FileLength: &up.FileLength, MediaKeyTimestamp: stamp, Mimetype: m.Mimetype, Seconds: m.Seconds, PTT: m.PTT, Waveform: m.Waveform, ContextInfo: quote}}
	}
}

type viewOnceFile struct {
	*os.File
	max int64
}

func (f *viewOnceFile) Write(p []byte) (int, error) {
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	if offset < 0 || offset > f.max || int64(len(p)) > f.max-offset {
		return 0, ErrViewOnceTooLarge
	}
	return f.File.Write(p)
}
func (f *viewOnceFile) WriteAt(p []byte, offset int64) (int, error) {
	if offset < 0 || offset > f.max || int64(len(p)) > f.max-offset {
		return 0, ErrViewOnceTooLarge
	}
	return f.File.WriteAt(p, offset)
}
func (f *viewOnceFile) Truncate(size int64) error {
	if size < 0 || size > f.max {
		return ErrViewOnceTooLarge
	}
	return f.File.Truncate(size)
}

// os.File.ReadFrom would bypass Write's bound if promoted through embedding.
func (f *viewOnceFile) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{f}, r)
}
