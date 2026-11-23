package clients

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/util/cbcutil"
	"go.mau.fi/whatsmeow/util/hkdfutil"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"google.golang.org/protobuf/proto"
)

type mmsUploadResponse struct {
	URL        string `json:"url"`
	DirectPath string `json:"direct_path"`
}

type uploadMMSFunc func(ctx context.Context, mmsType string, dataToUpload []byte, fileEncSHA []byte) (*mmsUploadResponse, error)
type sendMsgFunc func(ctx context.Context, to types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)

func encryptMedia(mediaKey []byte, appInfo string, plaintext []byte) (dataToUpload, fileSHA256, fileEncSHA256 []byte, err error) {
	pHash := sha256.Sum256(plaintext)
	fileSHA256 = pHash[:]

	mediaKeyExpanded := hkdfutil.SHA256(mediaKey, nil, []byte(appInfo), 112)
	iv := mediaKeyExpanded[:16]
	cipherKey := mediaKeyExpanded[16:48]
	macKey := mediaKeyExpanded[48:80]

	ciphertext, err := cbcutil.Encrypt(cipherKey, iv, plaintext)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to encrypt media: %w", err)
	}

	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(ciphertext)
	dataToUpload = append(ciphertext, h.Sum(nil)[:10]...)

	eHash := sha256.Sum256(dataToUpload)
	fileEncSHA256 = eHash[:]
	return dataToUpload, fileSHA256, fileEncSHA256, nil
}

func (c *Client) uploadMMS(ctx context.Context, mmsType string, dataToUpload []byte, fileEncSHA []byte) (*mmsUploadResponse, error) {
	if c.WA == nil {
		return nil, errors.New("whatsapp client not initialized")
	}

	mediaConn, err := c.WA.DangerousInternals().RefreshMediaConn(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh media connections: %w", err)
	}
	if len(mediaConn.Hosts) == 0 {
		return nil, errors.New("no media hosts available")
	}

	token := base64.URLEncoding.EncodeToString(fileEncSHA)
	q := url.Values{
		"auth":  []string{mediaConn.Auth},
		"token": []string{token},
	}
	uploadURL := url.URL{
		Scheme:   "https",
		Host:     mediaConn.Hosts[0].Hostname,
		Path:     fmt.Sprintf("/mms/%s/%s", mmsType, token),
		RawQuery: q.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), bytes.NewReader(dataToUpload))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare upload request: %w", err)
	}

	req.ContentLength = int64(len(dataToUpload))
	req.Header.Set("Origin", "https://web.whatsapp.com")
	req.Header.Set("Referer", "https://web.whatsapp.com/")
	req.Header.Set("User-Agent", "WhatsApp/2.24.6.77 Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	if mmsType == "sticker-pack" {
		req.Header.Set("Content-Type", "application/zip")
	} else if mmsType == "thumbnail-sticker-pack" {
		req.Header.Set("Content-Type", "image/jpeg")
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("upload to %s failed (status %d): %s", mmsType, resp.StatusCode, string(snippet))
	}

	var res mmsUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode upload response: %w", err)
	}
	if res.DirectPath == "" {
		return nil, errors.New("upload response missing direct_path")
	}
	return &res, nil
}

func generateDefaultTrayIcon(baseImg image.Image) ([]byte, error) {
	dst := image.NewRGBA(image.Rect(0, 0, 96, 96))
	if baseImg != nil {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), baseImg, baseImg.Bounds(), draw.Over, nil)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func generateDefaultCoverThumbnail(baseImg image.Image) ([]byte, error) {
	dst := image.NewRGBA(image.Rect(0, 0, 252, 252))
	if baseImg != nil {
		b := baseImg.Bounds()
		w, h := b.Dx(), b.Dy()
		ratio := float64(w) / float64(h)
		nw, nh := 252, int(252.0/ratio)
		if nh > 252 {
			nh = 252
			nw = int(252.0 * ratio)
		}
		x := (252 - nw) / 2
		y := (252 - nh) / 2
		xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+nw, y+nh), baseImg, b, draw.Over, nil)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 75}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func prepareStickerPack(pack *sticker.StickerPack) error {
	if pack == nil {
		return errors.New("sticker pack is nil")
	}
	if len(pack.Stickers) == 0 {
		return errors.New("sticker pack requires at least one sticker")
	}

	if pack.ID == "" {
		pack.ID = uuid.NewString()
	}
	if pack.Name == "" {
		pack.Name = "Sticker Pack"
	}
	if pack.Publisher == "" {
		pack.Publisher = "Kotonehara"
	}

	// Assign default filenames, mimetypes, and emojis to stickers
	for i := range pack.Stickers {
		if pack.Stickers[i].FileName == "" {
			pack.Stickers[i].FileName = fmt.Sprintf("sticker_%d.webp", i+1)
		}
		if pack.Stickers[i].Mimetype == "" {
			pack.Stickers[i].Mimetype = "image/webp"
		}
		if len(pack.Stickers[i].Emojis) == 0 {
			pack.Stickers[i].Emojis = []string{"✨"}
		}
	}

	// Attempt to decode a base image for thumbnail/tray icon fallback
	var baseImg image.Image
	if len(pack.CoverThumbnail) > 0 {
		baseImg, _, _ = image.Decode(bytes.NewReader(pack.CoverThumbnail))
	}
	if baseImg == nil && len(pack.Stickers) > 0 && len(pack.Stickers[0].Data) > 0 {
		baseImg, _, _ = image.Decode(bytes.NewReader(pack.Stickers[0].Data))
	}

	if len(pack.CoverThumbnail) == 0 {
		thumb, err := generateDefaultCoverThumbnail(baseImg)
		if err != nil {
			return fmt.Errorf("failed to generate default cover thumbnail: %w", err)
		}
		pack.CoverThumbnail = thumb
	}

	if len(pack.TrayIcon.Data) == 0 {
		tray, err := generateDefaultTrayIcon(baseImg)
		if err != nil {
			return fmt.Errorf("failed to generate default tray icon: %w", err)
		}
		pack.TrayIcon.Data = tray
		pack.TrayIcon.FileName = "tray.png"
	} else if pack.TrayIcon.FileName == "" {
		pack.TrayIcon.FileName = "tray.png"
	}

	if pack.ThumbnailWidth == 0 {
		pack.ThumbnailWidth = 252
	}
	if pack.ThumbnailHeight == 0 {
		pack.ThumbnailHeight = 252
	}

	return nil
}

func buildStickerPackProtoStickers(stickers []sticker.StickerPackItem) []*waE2E.StickerPackMessage_Sticker {
	protoStickers := make([]*waE2E.StickerPackMessage_Sticker, len(stickers))
	for i, s := range stickers {
		mime := s.Mimetype
		if mime == "" {
			mime = "image/webp"
		}
		protoStickers[i] = &waE2E.StickerPackMessage_Sticker{
			FileName:   proto.String(s.FileName),
			Emojis:     s.Emojis,
			IsAnimated: proto.Bool(s.IsAnimated),
			IsLottie:   proto.Bool(s.IsLottie),
			Mimetype:   proto.String(mime),
		}
	}
	return protoStickers
}

func buildStickerPackMessage(
	pack sticker.StickerPack,
	zipBytes []byte,
	bundleUp, coverUp *mmsUploadResponse,
	bundleSHA, bundleEncSHA, coverSHA, coverEncSHA, mediaKey []byte,
	opts *waE2E.ContextInfo,
) *waE2E.Message {
	origin := waE2E.StickerPackMessage_USER_CREATED
	packMsg := &waE2E.StickerPackMessage{
		StickerPackID:       proto.String(pack.ID),
		Name:                proto.String(pack.Name),
		Publisher:           proto.String(pack.Publisher),
		Stickers:            buildStickerPackProtoStickers(pack.Stickers),
		FileLength:          proto.Uint64(uint64(len(zipBytes))),
		FileSHA256:          bundleSHA,
		FileEncSHA256:       bundleEncSHA,
		MediaKey:            mediaKey,
		DirectPath:          proto.String(bundleUp.DirectPath),
		ContextInfo:         opts,
		MediaKeyTimestamp:   proto.Int64(time.Now().Unix()),
		TrayIconFileName:    proto.String(pack.TrayIcon.FileName),
		ThumbnailDirectPath: proto.String(coverUp.DirectPath),
		ThumbnailSHA256:     coverSHA,
		ThumbnailEncSHA256:  coverEncSHA,
		ThumbnailWidth:      proto.Uint32(pack.ThumbnailWidth),
		ThumbnailHeight:     proto.Uint32(pack.ThumbnailHeight),
		ImageDataHash:       proto.String(base64.StdEncoding.EncodeToString(coverSHA)),
		StickerPackSize:     proto.Uint64(uint64(len(zipBytes))),
		StickerPackOrigin:   &origin,
	}

	if pack.Caption != "" {
		packMsg.Caption = proto.String(pack.Caption)
	}
	if pack.Description != "" {
		packMsg.PackDescription = proto.String(pack.Description)
	}

	return &waE2E.Message{
		StickerPackMessage: packMsg,
	}
}

func sendStickerPack(
	ctx context.Context,
	to types.JID,
	pack sticker.StickerPack,
	opts *waE2E.ContextInfo,
	upload uploadMMSFunc,
	send sendMsgFunc,
) (whatsmeow.SendResponse, error) {
	if err := prepareStickerPack(&pack); err != nil {
		return whatsmeow.SendResponse{}, err
	}

	zipBytes, err := sticker.BuildStickerPackZip(&pack)
	if err != nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("failed to build sticker pack zip: %w", err)
	}

	// Generate a single shared mediaKey for both the ZIP bundle and the cover thumbnail
	mediaKey := make([]byte, 32)
	if _, err := rand.Read(mediaKey); err != nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("failed to generate media key: %w", err)
	}

	bundleData, bundleSHA, bundleEncSHA, err := encryptMedia(mediaKey, "WhatsApp Sticker Pack Keys", zipBytes)
	if err != nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("failed to encrypt sticker pack bundle: %w", err)
	}

	coverData, coverSHA, coverEncSHA, err := encryptMedia(mediaKey, "WhatsApp Sticker Pack Thumbnail Keys", pack.CoverThumbnail)
	if err != nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("failed to encrypt sticker pack thumbnail: %w", err)
	}

	bundleUp, err := upload(ctx, "sticker-pack", bundleData, bundleEncSHA)
	if err != nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("failed to upload sticker pack bundle: %w", err)
	}

	coverUp, err := upload(ctx, "thumbnail-sticker-pack", coverData, coverEncSHA)
	if err != nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("failed to upload sticker pack thumbnail: %w", err)
	}

	msg := buildStickerPackMessage(
		pack,
		zipBytes,
		bundleUp,
		coverUp,
		bundleSHA,
		bundleEncSHA,
		coverSHA,
		coverEncSHA,
		mediaKey,
		opts,
	)

	return send(ctx, to, msg)
}

// SendStickerPack builds and transmits a WhatsApp sticker pack message with an encrypted
// ZIP archive bundle and an encrypted cover thumbnail.
func (c *Client) SendStickerPack(
	ctx context.Context,
	to types.JID,
	pack sticker.StickerPack,
	opts *waE2E.ContextInfo,
) (whatsmeow.SendResponse, error) {
	return sendStickerPack(ctx, to, pack, opts, c.uploadMMS, c.WA.SendMessage)
}
