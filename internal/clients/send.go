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
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const maxFetchedMediaSize = 256 << 20

var errFetchedMediaTooLarge = errors.New("media melebihi batas")

func (c *Client) SendText(ctx context.Context, to types.JID, txt string, opts *waE2E.ContextInfo, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	return c.WA.SendMessage(ctx, to, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String(txt),
			ContextInfo: opts,
		},
	}, extra...)
}

func (c *Client) SendSticker(ctx context.Context, to types.JID, data []byte, isLottie bool, isAnimate bool, opts *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	up, err := c.WA.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}
	mime := http.DetectContentType(data)
	if isLottie {
		mime = "application/was"
	}
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			IsLottie:      proto.Bool(isLottie),
			IsAnimated:    proto.Bool(isAnimate),
			Mimetype:      proto.String(mime),
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			StickerSentTS: proto.Int64(time.Now().Unix()),
			FileLength:    proto.Uint64(uint64(len(data))),
			ContextInfo:   opts,
		},
	}
	return c.WA.SendMessage(ctx, to, msg)
}

func (c *Client) FetchBytes(u string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; kotonehara/1.0; +https://github.com/MapIHS/kotonehara)")
	req.Header.Set("Accept", "*/*")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readFetchedMedia(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 120 {
			snippet = snippet[:120]
		}
		if snippet == "" {
			return nil, fmt.Errorf("http %d saat download media", resp.StatusCode)
		}
		return nil, fmt.Errorf("http %d saat download media: %s", resp.StatusCode, snippet)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("media kosong dari %s", u)
	}

	return body, nil
}

func readFetchedMedia(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > maxFetchedMediaSize {
		return nil, fmt.Errorf("%w %d byte", errFetchedMediaTooLarge, maxFetchedMediaSize)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchedMediaSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxFetchedMediaSize {
		return nil, fmt.Errorf("%w %d byte", errFetchedMediaTooLarge, maxFetchedMediaSize)
	}
	return body, nil
}

func (c *Client) SendVideo(ctx context.Context, to types.JID, data []byte, gifPlayback bool, caption string, opts *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	mime := http.DetectContentType(data)
	if mime == "application/octet-stream" {
		mime = "video/mp4"
	}
	if !strings.HasPrefix(mime, "video/") {
		return whatsmeow.SendResponse{}, fmt.Errorf("data bukan video valid (mime: %s)", mime)
	}

	up, err := c.WA.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return whatsmeow.SendResponse{}, err
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
			FileLength:    &up.FileLength,
			ContextInfo:   opts,
		},
	}

	return c.WA.SendMessage(ctx, to, msg)
}

func (c *Client) SendAudio(ctx context.Context, to types.JID, data []byte, ptt bool, opts *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	mime := http.DetectContentType(data)
	switch mime {
	case "application/octet-stream":
		mime = "audio/mpeg"
	case "application/ogg":
		mime = "audio/ogg"
	}
	if !strings.HasPrefix(mime, "audio/") {
		return whatsmeow.SendResponse{}, fmt.Errorf("data bukan audio valid (mime: %s)", mime)
	}

	up, err := c.WA.Upload(ctx, data, whatsmeow.MediaAudio)
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

func (c *Client) SendImage(ctx context.Context, to types.JID, data []byte, caption string, opts *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	mime, width, height, err := c.DetectImageInfo(data)
	if err != nil {
		return whatsmeow.SendResponse{}, err
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
		return whatsmeow.SendResponse{}, err
	}

	thumb, _ := c.MakeJPEGThumb(data, 256, 256)

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &up.URL,
			DirectPath:    &up.DirectPath,
			MediaKey:      up.MediaKey,
			JPEGThumbnail: thumb,
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mime),
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    &up.FileLength,
			Width:         widthVal,
			Height:        heightVal,
			ContextInfo:   opts,
		},
	}
	return c.WA.SendMessage(ctx, to, msg)
}

func (c *Client) SendDocument(ctx context.Context, to types.JID, data []byte, filename, caption string, opts *waE2E.ContextInfo) (whatsmeow.SendResponse, error) {
	up, err := c.WA.Upload(ctx, data, whatsmeow.MediaDocument)
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
			Mimetype:      proto.String(http.DetectContentType(data)),
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			ContextInfo:   opts,
		},
	}
	return c.WA.SendMessage(ctx, to, msg)
}
