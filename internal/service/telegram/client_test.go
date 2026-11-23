package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExtractPackName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://t.me/addstickers/animals", "animals"},
		{"http://t.me/addstickers/CatPack_123", "CatPack_123"},
		{"https://telegram.me/addstickers/anime_pack", "anime_pack"},
		{"t.me/addstickers/cool_stickers", "cool_stickers"},
		{"https://t.me/addstickers/mypack/", "mypack"},
		{"animals", "animals"},
		{"My_Sticker_Pack_2026", "My_Sticker_Pack_2026"},
		{"", ""},
		{"https://google.com/test", ""},
		{"https://t.me/other/route", ""},
	}

	for _, tt := range tests {
		got := ExtractPackName(tt.input)
		if got != tt.want {
			t.Errorf("ExtractPackName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTelegramClientGetStickerSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/botTESTTOKEN/getStickerSet":
			name := r.URL.Query().Get("name")
			if name == "valid_pack" {
				json.NewEncoder(w).Encode(StickerSetResponse{
					OK: true,
					Result: &StickerSet{
						Name:  "valid_pack",
						Title: "Valid Pack Title",
						Stickers: []Sticker{
							{FileID: "file-1", Width: 512, Height: 512},
							{FileID: "file-2", Width: 512, Height: 512},
						},
					},
				})
				return
			} else if name == "empty_pack" {
				json.NewEncoder(w).Encode(StickerSetResponse{
					OK: true,
					Result: &StickerSet{
						Name:     "empty_pack",
						Title:    "Empty",
						Stickers: []Sticker{},
					},
				})
				return
			}
			json.NewEncoder(w).Encode(StickerSetResponse{
				OK:          false,
				Description: "STICKERSET_INVALID",
			})
		case "/botTESTTOKEN/getFile":
			fileID := r.URL.Query().Get("file_id")
			if fileID == "file-1" {
				json.NewEncoder(w).Encode(GetFileResponse{
					OK: true,
					Result: &FileInfo{
						FileID:   "file-1",
						FilePath: "stickers/file_1.webp",
					},
				})
				return
			}
			json.NewEncoder(w).Encode(GetFileResponse{
				OK:          false,
				Description: "FILE_ID_INVALID",
			})
		case "/file/botTESTTOKEN/stickers/file_1.webp":
			w.Header().Set("Content-Type", "image/webp")
			w.Write([]byte("RIFFWEBPTESTDATA"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewWithBaseURL("TESTTOKEN", server.URL, 5*time.Second)

	// Test GetStickerSet success
	set, err := client.GetStickerSet(context.Background(), "valid_pack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if set.Name != "valid_pack" || len(set.Stickers) != 2 {
		t.Fatalf("unexpected sticker set result: %+v", set)
	}

	// Test GetStickerSet empty
	_, err = client.GetStickerSet(context.Background(), "empty_pack")
	if err == nil || err != ErrStickerSetEmpty {
		t.Fatalf("expected ErrStickerSetEmpty, got: %v", err)
	}

	// Test GetStickerSet not found
	_, err = client.GetStickerSet(context.Background(), "unknown_pack")
	if err == nil {
		t.Fatal("expected error for unknown pack")
	}

	// Test GetFile success
	fileInfo, err := client.GetFile(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fileInfo.FilePath != "stickers/file_1.webp" {
		t.Fatalf("expected stickers/file_1.webp, got %s", fileInfo.FilePath)
	}

	// Test DownloadFile success
	data, err := client.DownloadFile(context.Background(), "stickers/file_1.webp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "RIFFWEBPTESTDATA" {
		t.Fatalf("expected RIFFWEBPTESTDATA, got %s", string(data))
	}

	// Test DownloadSticker helper
	sData, err := client.DownloadSticker(context.Background(), Sticker{FileID: "file-1"})
	if err != nil {
		t.Fatalf("unexpected error downloading sticker: %v", err)
	}
	if string(sData) != "RIFFWEBPTESTDATA" {
		t.Fatalf("expected RIFFWEBPTESTDATA, got %s", string(sData))
	}
}
