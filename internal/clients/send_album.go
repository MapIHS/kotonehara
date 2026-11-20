package clients

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const (
	MaxAlbumItems             = 20
	MaxAlbumMediaBytes  int64 = 32 << 20
	MaxAlbumTotalBytes  int64 = 128 << 20
	maxAlbumImagePixels int64 = 16_000_000
)

var ErrAlbumLimit = errors.New("album melebihi batas ukuran atau jumlah media")
var albumSlots = make(chan struct{}, 1)

type AlbumMedia struct {
	URL         string
	Headers     map[string]string
	Fetch       func(context.Context, int64) ([]byte, error)
	Data        []byte
	Caption     string
	IsVideo     bool
	GifPlayback bool
}

func NewAlbumImage(data []byte, caption ...string) AlbumMedia {
	capText := ""
	if len(caption) > 0 {
		capText = caption[0]
	}
	return AlbumMedia{
		Data:    data,
		Caption: capText,
		IsVideo: false,
	}
}

func NewAlbumVideo(data []byte, gifPlayback bool, caption ...string) AlbumMedia {
	capText := ""
	if len(caption) > 0 {
		capText = caption[0]
	}
	return AlbumMedia{
		Data:        data,
		Caption:     capText,
		IsVideo:     true,
		GifPlayback: gifPlayback,
	}
}

func NewAlbumImageURL(url string, caption ...string) AlbumMedia {
	item := NewAlbumImage(nil, caption...)
	item.URL = url
	return item
}

func NewAlbumVideoURL(url string, gifPlayback bool, caption ...string) AlbumMedia {
	item := NewAlbumVideo(nil, gifPlayback, caption...)
	item.URL = url
	return item
}

func (c *Client) detectAlbumMediaType(item AlbumMedia) (bool, error) {
	if len(item.Data) == 0 {
		return false, errors.New("data media kosong")
	}
	if item.IsVideo {
		return true, nil
	}

	mime := http.DetectContentType(item.Data)
	if strings.HasPrefix(mime, "video/") || mime == "image/gif" {
		return true, nil
	}

	if len(item.Data) >= 8 && string(item.Data[4:8]) == "ftyp" {
		return true, nil
	}

	if strings.HasPrefix(mime, "image/") {
		return false, nil
	}

	if _, _, _, err := c.DetectImageInfo(item.Data); err == nil {
		return false, nil
	}

	return false, fmt.Errorf("media tidak valid atau format tidak didukung (mime: %s)", mime)
}

func (c *Client) prepareAlbumImage(ctx context.Context, item AlbumMedia) (*waE2E.Message, error) {
	data := item.Data
	mime, width, height, err := c.DetectImageInfo(data)
	if err != nil {
		return nil, fmt.Errorf("gagal mendeteksi info gambar: %w", err)
	}

	if width <= 0 || height <= 0 || int64(width) > maxAlbumImagePixels/int64(height) {
		return nil, fmt.Errorf("%w: gambar maksimal 16 megapiksel", ErrAlbumLimit)
	}
	if mime == "image/webp" {
		jpegData, err := c.ConvertToJPEG(data)
		if err == nil {
			data = jpegData
			mime = "image/jpeg"
		}
	}

	var widthVal, heightVal *uint32
	if width > 0 {
		widthVal = proto.Uint32(uint32(width))
	}
	if height > 0 {
		heightVal = proto.Uint32(uint32(height))
	}

	if int64(len(data)) > MaxAlbumMediaBytes {
		return nil, ErrAlbumLimit
	}
	up, err := c.WA.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("gagal upload gambar: %w", err)
	}

	thumb, _ := c.MakeJPEGThumb(data, 256, 256)

	imgMsg := &waE2E.ImageMessage{
		URL:           &up.URL,
		DirectPath:    &up.DirectPath,
		MediaKey:      up.MediaKey,
		JPEGThumbnail: thumb,
		Mimetype:      proto.String(mime),
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    &up.FileLength,
		Width:         widthVal,
		Height:        heightVal,
	}
	if item.Caption != "" {
		imgMsg.Caption = proto.String(item.Caption)
	}

	return &waE2E.Message{
		ImageMessage: imgMsg,
	}, nil
}

func (c *Client) prepareAlbumVideo(ctx context.Context, item AlbumMedia) (*waE2E.Message, error) {
	data := item.Data
	mime := http.DetectContentType(data)
	if mime == "application/octet-stream" {
		mime = "video/mp4"
	}
	gifPlayback := item.GifPlayback
	if mime == "image/gif" {
		_, width, height, err := c.DetectImageInfo(data)
		if err != nil {
			return nil, err
		}
		if width <= 0 || height <= 0 || int64(width) > maxAlbumImagePixels/int64(height) {
			return nil, ErrAlbumLimit
		}
		gifPlayback = true
		if mp4Data, err := c.convertGifToMP4(ctx, data, MaxAlbumMediaBytes); err == nil {
			data = mp4Data
			mime = "video/mp4"
		}
	}
	if !strings.HasPrefix(mime, "video/") {
		return nil, fmt.Errorf("data bukan video valid (mime: %s)", mime)
	}

	if int64(len(data)) > MaxAlbumMediaBytes {
		return nil, ErrAlbumLimit
	}
	up, err := c.WA.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return nil, fmt.Errorf("gagal upload video: %w", err)
	}

	meta := ProbeVideoInfo(ctx, data)
	thumb, _ := c.MakeVideoThumb(ctx, data, 256, 256)

	vidMsg := &waE2E.VideoMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		GifPlayback:   proto.Bool(gifPlayback),
		Mimetype:      proto.String(mime),
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(data))),
		JPEGThumbnail: thumb,
	}
	if item.Caption != "" {
		vidMsg.Caption = proto.String(item.Caption)
	}
	if meta.Width > 0 {
		vidMsg.Width = proto.Uint32(meta.Width)
	}
	if meta.Height > 0 {
		vidMsg.Height = proto.Uint32(meta.Height)
	}
	if meta.Duration > 0 {
		vidMsg.Seconds = proto.Uint32(meta.Duration)
	}

	return &waE2E.Message{
		VideoMessage: vidMsg,
	}, nil
}

// SendAlbum keeps only one raw attachment at a time; callers should pass URL
// descriptors or Fetch callbacks instead of collecting downloads in memory.
func (c *Client) SendAlbum(ctx context.Context, to types.JID, medias []AlbumMedia, opts *waE2E.ContextInfo) ([]whatsmeow.SendResponse, error) {
	if len(medias) == 0 || len(medias) > MaxAlbumItems {
		return nil, ErrAlbumLimit
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	select {
	case albumSlots <- struct{}{}:
		defer func() { <-albumSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	remaining := MaxAlbumTotalBytes
	prepare := func(ctx context.Context, item AlbumMedia) (*waE2E.Message, error) {
		var err error
		item.Data, err = c.loadAlbumMedia(ctx, item, min(MaxAlbumMediaBytes, remaining))
		if err != nil {
			return nil, err
		}
		remaining -= int64(len(item.Data))
		isVideo, err := c.detectAlbumMediaType(item)
		if err != nil {
			return nil, err
		}
		if isVideo {
			return c.prepareAlbumVideo(ctx, item)
		}
		return c.prepareAlbumImage(ctx, item)
	}
	return sendAlbum(ctx, to, medias, opts, prepare, c.WA.SendMessage)
}

func (c *Client) loadAlbumMedia(ctx context.Context, item AlbumMedia, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, ErrAlbumLimit
	}
	data := item.Data
	var err error
	switch {
	case item.Fetch != nil:
		data, err = item.Fetch(ctx, limit)
	case item.URL != "":
		data, err = c.fetchAlbumBytes(ctx, item.URL, item.Headers, limit)
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("data media kosong")
	}
	if int64(len(data)) > limit {
		return nil, ErrAlbumLimit
	}
	return data, nil
}

func (c *Client) fetchAlbumBytes(ctx context.Context, url string, headers map[string]string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; kotonehara/1.0)")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download media: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, ErrAlbumLimit
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrAlbumLimit
	}
	return data, nil
}

type albumSend func(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)

func sendAlbum(ctx context.Context, to types.JID, medias []AlbumMedia, opts *waE2E.ContextInfo,
	prepare func(context.Context, AlbumMedia) (*waE2E.Message, error), send albumSend,
) ([]whatsmeow.SendResponse, error) {
	if len(medias) == 0 || len(medias) > MaxAlbumItems {
		return nil, ErrAlbumLimit
	}
	prepared := make([]*waE2E.Message, 0, len(medias))
	var failures []error
	var images, videos uint32
	for i, item := range medias {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		msg, err := prepare(ctx, item)
		if err != nil {
			failures = append(failures, fmt.Errorf("media album %d: %w", i+1, err))
			continue
		}
		switch {
		case msg.GetImageMessage() != nil:
			images++
		case msg.GetVideoMessage() != nil:
			videos++
		default:
			return nil, errors.New("tipe media album tidak didukung")
		}
		prepared = append(prepared, msg)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(prepared) == 0 {
		return nil, errors.Join(failures...)
	}
	if len(prepared) == 1 {
		msg := prepared[0]
		if msg.ImageMessage != nil {
			msg.ImageMessage.ContextInfo = opts
		}
		if msg.VideoMessage != nil {
			msg.VideoMessage.ContextInfo = opts
		}
		resp, err := send(ctx, to, msg)
		if err != nil {
			return nil, errors.Join(append(failures, err)...)
		}
		return []whatsmeow.SendResponse{resp}, errors.Join(failures...)
	}
	album := &waE2E.Message{AlbumMessage: &waE2E.AlbumMessage{
		ExpectedImageCount: proto.Uint32(images), ExpectedVideoCount: proto.Uint32(videos), ContextInfo: opts,
	}}
	resp, err := send(ctx, to, album)
	if err != nil {
		return nil, errors.Join(append(failures, err)...)
	}
	responses := []whatsmeow.SendResponse{resp}
	key := &waCommon.MessageKey{ID: proto.String(string(resp.ID)), RemoteJID: proto.String(to.ToNonAD().String()), FromMe: proto.Bool(true)}
	for i, msg := range prepared {
		if err := ctx.Err(); err != nil {
			return responses, errors.Join(append(failures, err)...)
		}
		msg.MessageContextInfo = &waE2E.MessageContextInfo{MessageAssociation: &waE2E.MessageAssociation{
			AssociationType: waE2E.MessageAssociation_MEDIA_ALBUM.Enum(), ParentMessageKey: key,
		}}
		child, err := send(ctx, to, msg)
		if err != nil {
			failures = append(failures, fmt.Errorf("kirim media album %d: %w", i+1, err))
			continue
		}
		responses = append(responses, child)
	}
	return responses, errors.Join(failures...)
}
