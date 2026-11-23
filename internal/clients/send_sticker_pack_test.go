package clients

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestSendStickerPackHappyPath(t *testing.T) {
	pack := sticker.StickerPack{
		ID:        "pack-uuid-123",
		Name:      "Cute Pack",
		Publisher: "Author",
		Stickers: []sticker.StickerPackItem{
			{FileName: "0.webp", Data: []byte("RIFFsticker0"), Emojis: []string{"🐱"}},
			{FileName: "1.webp", Data: []byte("RIFFsticker1"), IsAnimated: true},
		},
		TrayIcon: sticker.StickerPackTrayIcon{
			FileName: "tray.png",
			Data:     []byte("trayicondata"),
		},
		CoverThumbnail: []byte("coverthumbnailjpeg"),
	}

	uploadCalls := make(map[string][]byte)
	mockUpload := func(_ context.Context, mmsType string, dataToUpload []byte, fileEncSHA []byte) (*mmsUploadResponse, error) {
		uploadCalls[mmsType] = dataToUpload
		return &mmsUploadResponse{
			URL:        "https://mmg.whatsapp.net/" + mmsType,
			DirectPath: "/v/t62/" + mmsType,
		}, nil
	}

	var sentMsg *waE2E.Message
	var sentTo types.JID
	mockSend := func(_ context.Context, to types.JID, msg *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		sentTo = to
		sentMsg = msg
		return whatsmeow.SendResponse{ID: "msg-id-123"}, nil
	}

	chat := types.NewJID("user1", types.DefaultUserServer)
	quote := &waE2E.ContextInfo{StanzaID: proto.String("trigger-1")}

	resp, err := sendStickerPack(context.Background(), chat, pack, quote, mockUpload, mockSend)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "msg-id-123" {
		t.Errorf("unexpected resp ID: %s", resp.ID)
	}
	if sentTo != chat {
		t.Errorf("unexpected recipient: %s", sentTo)
	}

	if _, ok := uploadCalls["sticker-pack"]; !ok {
		t.Error("missing sticker-pack upload")
	}
	if _, ok := uploadCalls["thumbnail-sticker-pack"]; !ok {
		t.Error("missing thumbnail-sticker-pack upload")
	}

	if sentMsg == nil || sentMsg.StickerPackMessage == nil {
		t.Fatal("sent message does not contain StickerPackMessage")
	}

	sp := sentMsg.StickerPackMessage
	if sp.GetStickerPackID() != "pack-uuid-123" {
		t.Errorf("StickerPackID = %s, want pack-uuid-123", sp.GetStickerPackID())
	}
	if sp.GetName() != "Cute Pack" {
		t.Errorf("Name = %s, want Cute Pack", sp.GetName())
	}
	if sp.GetPublisher() != "Author" {
		t.Errorf("Publisher = %s, want Author", sp.GetPublisher())
	}
	if len(sp.GetStickers()) != 2 {
		t.Fatalf("Stickers count = %d, want 2", len(sp.GetStickers()))
	}
	if sp.GetStickers()[0].GetFileName() != "0.webp" || len(sp.GetStickers()[0].GetEmojis()) != 1 || sp.GetStickers()[0].GetEmojis()[0] != "🐱" {
		t.Errorf("Sticker 0 mismatch: %v", sp.GetStickers()[0])
	}
	if sp.GetStickers()[1].GetFileName() != "1.webp" || !sp.GetStickers()[1].GetIsAnimated() {
		t.Errorf("Sticker 1 mismatch: %v", sp.GetStickers()[1])
	}
	if sp.GetDirectPath() != "/v/t62/sticker-pack" {
		t.Errorf("DirectPath = %s", sp.GetDirectPath())
	}
	if sp.GetThumbnailDirectPath() != "/v/t62/thumbnail-sticker-pack" {
		t.Errorf("ThumbnailDirectPath = %s", sp.GetThumbnailDirectPath())
	}
	if len(sp.GetMediaKey()) != 32 {
		t.Errorf("MediaKey length = %d, want 32", len(sp.GetMediaKey()))
	}
	if sp.GetTrayIconFileName() != "tray.png" {
		t.Errorf("TrayIconFileName = %s, want tray.png", sp.GetTrayIconFileName())
	}
	if sp.GetStickerPackOrigin() != waE2E.StickerPackMessage_USER_CREATED {
		t.Errorf("StickerPackOrigin = %v, want USER_CREATED", sp.GetStickerPackOrigin())
	}

	coverSHA := sha256.Sum256([]byte("coverthumbnailjpeg"))
	expectedHash := base64.StdEncoding.EncodeToString(coverSHA[:])
	if sp.GetImageDataHash() != expectedHash {
		t.Errorf("ImageDataHash = %s, want %s", sp.GetImageDataHash(), expectedHash)
	}

	if sp.GetContextInfo().GetStanzaID() != "trigger-1" {
		t.Errorf("ContextInfo StanzaID = %s, want trigger-1", sp.GetContextInfo().GetStanzaID())
	}
}

func TestSendStickerPackUploadErrorPropagates(t *testing.T) {
	pack := sticker.StickerPack{
		Stickers: []sticker.StickerPackItem{
			{FileName: "0.webp", Data: []byte("data")},
		},
		TrayIcon:       sticker.StickerPackTrayIcon{FileName: "tray.png", Data: []byte("tray")},
		CoverThumbnail: []byte("cover"),
	}

	uploadErr := errors.New("network timeout")
	mockUpload := func(_ context.Context, mmsType string, _ []byte, _ []byte) (*mmsUploadResponse, error) {
		if mmsType == "sticker-pack" {
			return nil, uploadErr
		}
		return &mmsUploadResponse{DirectPath: "/ok"}, nil
	}
	mockSend := func(_ context.Context, _ types.JID, _ *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		t.Fatal("send should not be called when upload fails")
		return whatsmeow.SendResponse{}, nil
	}

	_, err := sendStickerPack(context.Background(), types.EmptyJID, pack, nil, mockUpload, mockSend)
	if !errors.Is(err, uploadErr) {
		t.Fatalf("expected error %v, got %v", uploadErr, err)
	}
}
