package clients

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type AlbumMedia struct {
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

func (c *Client) detectAlbumMediaType(item AlbumMedia) (bool, error) {
	if item.IsVideo {
		return true, nil
	}
	if len(item.Data) == 0 {
		return false, errors.New("data media kosong")
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
		gifPlayback = true
		if mp4Data, err := c.ConvertGifToMP4(ctx, data); err == nil {
			data = mp4Data
			mime = "video/mp4"
		}
	}
	if !strings.HasPrefix(mime, "video/") {
		return nil, fmt.Errorf("data bukan video valid (mime: %s)", mime)
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

func (c *Client) SendAlbum(ctx context.Context, to types.JID, medias []AlbumMedia, opts *waE2E.ContextInfo) ([]whatsmeow.SendResponse, error) {
	if len(medias) == 0 {
		return nil, errors.New("media album kosong")
	}

	if len(medias) == 1 {
		item := medias[0]
		isVideo, err := c.detectAlbumMediaType(item)
		if err != nil {
			return nil, err
		}
		if isVideo {
			resp, err := c.SendVideo(ctx, to, item.Data, item.GifPlayback, item.Caption, opts)
			if err != nil {
				return nil, err
			}
			return []whatsmeow.SendResponse{resp}, nil
		}
		resp, err := c.SendImage(ctx, to, item.Data, item.Caption, opts)
		if err != nil {
			return nil, err
		}
		return []whatsmeow.SendResponse{resp}, nil
	}

	type preparedItem struct {
		msg     *waE2E.Message
		isVideo bool
	}

	prepared := make([]preparedItem, 0, len(medias))
	var expectedImages, expectedVideos uint32

	for i, media := range medias {
		isVideo, err := c.detectAlbumMediaType(media)
		if err != nil {
			return nil, fmt.Errorf("media album index %d: %w", i, err)
		}

		var msg *waE2E.Message
		if isVideo {
			msg, err = c.prepareAlbumVideo(ctx, media)
			if err != nil {
				return nil, fmt.Errorf("gagal menyiapkan video index %d: %w", i, err)
			}
			expectedVideos++
		} else {
			msg, err = c.prepareAlbumImage(ctx, media)
			if err != nil {
				return nil, fmt.Errorf("gagal menyiapkan gambar index %d: %w", i, err)
			}
			expectedImages++
		}
		prepared = append(prepared, preparedItem{msg: msg, isVideo: isVideo})
	}

	albumMsg := &waE2E.Message{
		AlbumMessage: &waE2E.AlbumMessage{
			ExpectedImageCount: proto.Uint32(expectedImages),
			ExpectedVideoCount: proto.Uint32(expectedVideos),
			ContextInfo:        opts,
		},
	}

	albumResp, err := c.WA.SendMessage(ctx, to, albumMsg)
	if err != nil {
		return nil, fmt.Errorf("gagal mengirim album message container: %w", err)
	}

	responses := make([]whatsmeow.SendResponse, 0, len(prepared)+1)
	responses = append(responses, albumResp)

	parentKey := &waCommon.MessageKey{
		ID:        proto.String(string(albumResp.ID)),
		RemoteJID: proto.String(to.ToNonAD().String()),
		FromMe:    proto.Bool(true),
	}

	var sendErrors []error
	for i, item := range prepared {
		item.msg.MessageContextInfo = &waE2E.MessageContextInfo{
			MessageAssociation: &waE2E.MessageAssociation{
				AssociationType:  waE2E.MessageAssociation_MEDIA_ALBUM.Enum(),
				ParentMessageKey: parentKey,
			},
		}

		childResp, err := c.WA.SendMessage(ctx, to, item.msg)
		if err != nil {
			sendErrors = append(sendErrors, fmt.Errorf("gagal mengirim media album index %d: %w", i, err))
			continue
		}
		responses = append(responses, childResp)
	}

	if len(sendErrors) > 0 {
		return responses, errors.Join(sendErrors...)
	}

	return responses, nil
}
