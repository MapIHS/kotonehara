package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/service/telegram"
)

func TestSendTelegramStickersUsesStickerPack(t *testing.T) {
	set := &telegram.StickerSet{
		Name:  "test_pack",
		Title: "Test Pack Title",
		Stickers: []telegram.Sticker{
			{FileID: "file1", Emoji: "🐱"},
			{FileID: "file2", Emoji: "🐶"},
			{FileID: "file3", Emoji: "🐰"},
		},
	}

	var capturedPack sticker.StickerPack
	sendPackCalls := 0
	sendCalls := 0

	deps := telegramStickerDeps{
		getFile: func(_ context.Context, fileID string) (*telegram.FileInfo, error) {
			return &telegram.FileInfo{FilePath: "stickers/" + fileID + ".webp"}, nil
		},
		downloadFile: func(_ context.Context, filePath string) ([]byte, error) {
			return []byte("raw-" + filePath), nil
		},
		build: func(_ context.Context, raw []byte, _ bool, _ bool) ([]byte, error) {
			return append([]byte("built-"), raw...), nil
		},
		makeThumb: func(_ context.Context, raw []byte, _ bool) ([]byte, error) {
			return append([]byte("thumb-"), raw...), nil
		},
		send: func(context.Context, []byte, bool, bool) error {
			sendCalls++
			return nil
		},
		sendPack: func(_ context.Context, pack sticker.StickerPack) error {
			sendPackCalls++
			capturedPack = pack
			return nil
		},
		packName:  "Test Pack Title",
		publisher: "Alice",
	}

	sent, total, tooLarge := sendTelegramStickers(context.Background(), set, deps)
	if sent != 3 || total != 3 || tooLarge != 0 {
		t.Fatalf("sent=%d total=%d tooLarge=%d", sent, total, tooLarge)
	}
	if sendPackCalls != 1 {
		t.Fatalf("sendPackCalls = %d, want 1", sendPackCalls)
	}
	if sendCalls != 0 {
		t.Fatalf("sendCalls = %d, want 0 when pack succeeds", sendCalls)
	}
	if capturedPack.Name != "Test Pack Title" || capturedPack.Publisher != "Alice" {
		t.Errorf("pack metadata mismatch: name=%q publisher=%q", capturedPack.Name, capturedPack.Publisher)
	}
	if len(capturedPack.Stickers) != 3 {
		t.Fatalf("pack stickers count = %d, want 3", len(capturedPack.Stickers))
	}
	if string(capturedPack.CoverThumbnail) != "thumb-raw-stickers/file1.webp" {
		t.Errorf("cover thumbnail = %q", string(capturedPack.CoverThumbnail))
	}
	if len(capturedPack.Stickers[0].Emojis) == 0 || capturedPack.Stickers[0].Emojis[0] != "🐱" {
		t.Errorf("sticker emoji mismatch: got %v, want ['🐱']", capturedPack.Stickers[0].Emojis)
	}
}

func TestSendTelegramStickersSingleItemUsesSendDirectly(t *testing.T) {
	set := &telegram.StickerSet{
		Name:  "single_item_pack",
		Title: "Single Item",
		Stickers: []telegram.Sticker{
			{FileID: "file_only"},
		},
	}

	sendPackCalls := 0
	sendCalls := 0

	deps := telegramStickerDeps{
		getFile: func(_ context.Context, fileID string) (*telegram.FileInfo, error) {
			return &telegram.FileInfo{FilePath: "stickers/" + fileID + ".webp"}, nil
		},
		downloadFile: func(_ context.Context, filePath string) ([]byte, error) {
			return []byte("data-" + filePath), nil
		},
		build: func(_ context.Context, raw []byte, _ bool, _ bool) ([]byte, error) {
			return raw, nil
		},
		send: func(_ context.Context, data []byte, isLottie bool, isAnimated bool) error {
			sendCalls++
			if string(data) != "data-stickers/file_only.webp" || isLottie || isAnimated {
				t.Errorf("unexpected send data: %s, lottie=%v, anim=%v", data, isLottie, isAnimated)
			}
			return nil
		},
		sendPack: func(context.Context, sticker.StickerPack) error {
			sendPackCalls++
			return nil
		},
	}

	sent, total, _ := sendTelegramStickers(context.Background(), set, deps)
	if sent != 1 || total != 1 {
		t.Fatalf("sent=%d total=%d", sent, total)
	}
	if sendPackCalls != 0 {
		t.Fatalf("sendPackCalls = %d, want 0 for single item", sendPackCalls)
	}
	if sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1 for single item", sendCalls)
	}
}

func TestSendTelegramStickersPackFallbackToIndividual(t *testing.T) {
	set := &telegram.StickerSet{
		Name:  "two_items",
		Title: "Two Items",
		Stickers: []telegram.Sticker{
			{FileID: "s1"},
			{FileID: "s2"},
		},
	}

	sendPackCalls := 0
	sendCalls := 0

	deps := telegramStickerDeps{
		getFile: func(_ context.Context, fileID string) (*telegram.FileInfo, error) {
			return &telegram.FileInfo{FilePath: "stickers/" + fileID + ".webp"}, nil
		},
		downloadFile: func(_ context.Context, filePath string) ([]byte, error) {
			return []byte("raw-" + filePath), nil
		},
		build: func(_ context.Context, raw []byte, _ bool, _ bool) ([]byte, error) {
			return raw, nil
		},
		send: func(context.Context, []byte, bool, bool) error {
			sendCalls++
			return nil
		},
		sendPack: func(context.Context, sticker.StickerPack) error {
			sendPackCalls++
			return errors.New("MMS connection refused")
		},
	}

	sent, total, _ := sendTelegramStickers(context.Background(), set, deps)
	if sent != 2 || total != 2 {
		t.Fatalf("sent=%d total=%d", sent, total)
	}
	if sendPackCalls != 1 {
		t.Fatalf("sendPackCalls = %d, want 1", sendPackCalls)
	}
	if sendCalls != 2 {
		t.Fatalf("sendCalls = %d, want 2 on fallback", sendCalls)
	}
}

func TestSendTelegramStickersCapsMaxStickers(t *testing.T) {
	stickers := make([]telegram.Sticker, maxTelegramStickers+15)
	for i := range stickers {
		stickers[i] = telegram.Sticker{FileID: "s"}
	}
	set := &telegram.StickerSet{
		Name:     "large_pack",
		Stickers: stickers,
	}

	calls := 0
	deps := telegramStickerDeps{
		getFile: func(_ context.Context, _ string) (*telegram.FileInfo, error) {
			return &telegram.FileInfo{FilePath: "s.webp"}, nil
		},
		downloadFile: func(_ context.Context, _ string) ([]byte, error) { return []byte{1}, nil },
		build:        func(_ context.Context, raw []byte, _ bool, _ bool) ([]byte, error) { return raw, nil },
		send:         func(context.Context, []byte, bool, bool) error { calls++; return nil },
	}

	sent, total, _ := sendTelegramStickers(context.Background(), set, deps)
	if sent != maxTelegramStickers || total != maxTelegramStickers || calls != maxTelegramStickers {
		t.Fatalf("sent=%d total=%d calls=%d, want %d", sent, total, calls, maxTelegramStickers)
	}
}

func TestSendTelegramStickersHandlesLottieAndAnimated(t *testing.T) {
	set := &telegram.StickerSet{
		Name:  "lottie_pack",
		Title: "Lottie Pack",
		Stickers: []telegram.Sticker{
			{FileID: "lottie1", IsAnimated: true},
			{FileID: "video1", IsVideo: true},
		},
	}

	var capturedPack sticker.StickerPack
	deps := telegramStickerDeps{
		getFile: func(_ context.Context, fileID string) (*telegram.FileInfo, error) {
			return &telegram.FileInfo{FilePath: fileID}, nil
		},
		downloadFile: func(_ context.Context, filePath string) ([]byte, error) {
			return []byte("data-" + filePath), nil
		},
		build: func(_ context.Context, raw []byte, isLottie bool, isAnimated bool) ([]byte, error) {
			return raw, nil
		},
		sendPack: func(_ context.Context, pack sticker.StickerPack) error {
			capturedPack = pack
			return nil
		},
	}

	sent, total, tooLarge := sendTelegramStickers(context.Background(), set, deps)
	if sent != 2 || total != 2 || tooLarge != 0 {
		t.Fatalf("sent=%d total=%d tooLarge=%d", sent, total, tooLarge)
	}

	if len(capturedPack.Stickers) != 2 {
		t.Fatalf("expected 2 stickers, got %d", len(capturedPack.Stickers))
	}

	s0 := capturedPack.Stickers[0]
	if !s0.IsLottie || !s0.IsAnimated || s0.Mimetype != "application/was" || s0.FileName != "sticker_1.was" {
		t.Errorf("lottie sticker fields mismatch: %+v", s0)
	}

	s1 := capturedPack.Stickers[1]
	if s1.IsLottie || !s1.IsAnimated || s1.Mimetype != "image/webp" || s1.FileName != "sticker_2.webp" {
		t.Errorf("video sticker fields mismatch: %+v", s1)
	}
}

func TestSendTelegramStickersTooLargeCount(t *testing.T) {
	set := &telegram.StickerSet{
		Name: "mixed_size",
		Stickers: []telegram.Sticker{
			{FileID: "ok"},
			{FileID: "too_large"},
		},
	}

	deps := telegramStickerDeps{
		getFile: func(_ context.Context, fileID string) (*telegram.FileInfo, error) {
			return &telegram.FileInfo{FilePath: fileID}, nil
		},
		downloadFile: func(_ context.Context, filePath string) ([]byte, error) {
			return []byte(filePath), nil
		},
		build: func(_ context.Context, raw []byte, _ bool, _ bool) ([]byte, error) {
			if string(raw) == "too_large" {
				return nil, sticker.ErrAnimatedStickerTooLarge
			}
			return raw, nil
		},
		send: func(context.Context, []byte, bool, bool) error {
			return nil
		},
		sendPack: func(context.Context, sticker.StickerPack) error {
			return nil
		},
	}

	sent, total, tooLarge := sendTelegramStickers(context.Background(), set, deps)
	if sent != 1 || total != 2 || tooLarge != 1 {
		t.Fatalf("sent=%d total=%d tooLarge=%d", sent, total, tooLarge)
	}
}

func TestTelegramFailureReplyDoesNotExposeRawError(t *testing.T) {
	got := telegramStickerFailureReply(errors.New("transport failed: https://api.telegram.org/botFAKE_SECRET_TOKEN/getStickerSet"))
	if got != "Gagal mengambil stiker Telegram. Coba lagi sebentar, yaa." {
		t.Fatalf("unexpected reply: %s", got)
	}
}
