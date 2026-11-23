package sticker

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

func TestBuildStickerPackZipRawHeaders(t *testing.T) {
	pack := &StickerPack{
		ID:        "test-pack-1",
		Name:      "Test Pack",
		Publisher: "Kotonehara",
		Stickers: []StickerPackItem{
			{FileName: "sticker_1.webp", Data: []byte("RIFFsticker0"), Emojis: []string{"😊"}},
			{FileName: "sticker_2.webp", Data: []byte("RIFFsticker1"), IsAnimated: true},
		},
		TrayIcon: StickerPackTrayIcon{
			FileName: "tray.png",
			Data:     []byte("trayicondata"),
		},
	}

	zipBytes, err := BuildStickerPackZip(pack)
	if err != nil {
		t.Fatalf("unexpected error building zip: %v", err)
	}

	// Verify local file header of first entry
	if len(zipBytes) < 30 {
		t.Fatalf("zipBytes too short: %d", len(zipBytes))
	}

	sig := binary.LittleEndian.Uint32(zipBytes[0:4])
	if sig != signatureLocalFile {
		t.Fatalf("local file header signature mismatch: got %x, want %x", sig, signatureLocalFile)
	}

	flags := binary.LittleEndian.Uint16(zipBytes[6:8])
	if flags != 0 {
		t.Fatalf("flags must be 0 (no data descriptor), got %x", flags)
	}

	method := binary.LittleEndian.Uint16(zipBytes[8:10])
	if method != 0 {
		t.Fatalf("compression method must be 0 (stored), got %d", method)
	}

	crc := binary.LittleEndian.Uint32(zipBytes[14:18])
	if crc == 0 {
		t.Fatal("CRC in local header must not be 0")
	}

	compSize := binary.LittleEndian.Uint32(zipBytes[18:22])
	uncompSize := binary.LittleEndian.Uint32(zipBytes[22:26])
	if compSize != uint32(len("RIFFsticker0")) || uncompSize != compSize {
		t.Fatalf("sizes in local header incorrect: compSize=%d, uncompSize=%d", compSize, uncompSize)
	}

	nameLen := binary.LittleEndian.Uint16(zipBytes[26:28])
	if nameLen != uint16(len("sticker_1.webp")) {
		t.Fatalf("nameLen = %d, want %d", nameLen, len("sticker_1.webp"))
	}

	extraLen := binary.LittleEndian.Uint16(zipBytes[28:30])
	if extraLen != 0 {
		t.Fatalf("extraLen must be 0, got %d", extraLen)
	}

	// Verify it can also be read by standard zip reader
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("failed to open generated zip with archive/zip: %v", err)
	}

	if len(zr.File) != 3 {
		t.Fatalf("expected 3 files in zip, got %d", len(zr.File))
	}

	expected := map[string][]byte{
		"sticker_1.webp": []byte("RIFFsticker0"),
		"sticker_2.webp": []byte("RIFFsticker1"),
		"tray.png":       []byte("trayicondata"),
	}

	for _, f := range zr.File {
		expectedData, ok := expected[f.Name]
		if !ok {
			t.Errorf("unexpected file in zip: %s", f.Name)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open file %s in zip: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read file %s in zip: %v", f.Name, err)
		}
		if !bytes.Equal(data, expectedData) {
			t.Errorf("file %s data mismatch: got %q, want %q", f.Name, string(data), string(expectedData))
		}
	}
}

func TestValidateStickerPackErrors(t *testing.T) {
	tests := []struct {
		name    string
		pack    *StickerPack
		wantErr string
	}{
		{
			name:    "nil pack",
			pack:    nil,
			wantErr: "nil",
		},
		{
			name: "no stickers",
			pack: &StickerPack{
				TrayIcon: StickerPackTrayIcon{FileName: "tray.png", Data: []byte("data")},
			},
			wantErr: "at least one sticker",
		},
		{
			name: "empty tray icon filename",
			pack: &StickerPack{
				Stickers: []StickerPackItem{{FileName: "0.webp", Data: []byte("stk")}},
				TrayIcon: StickerPackTrayIcon{FileName: "", Data: []byte("data")},
			},
			wantErr: "trayIcon requires a non-empty fileName",
		},
		{
			name: "empty tray icon data",
			pack: &StickerPack{
				Stickers: []StickerPackItem{{FileName: "0.webp", Data: []byte("stk")}},
				TrayIcon: StickerPackTrayIcon{FileName: "tray.png", Data: nil},
			},
			wantErr: "trayIcon requires non-empty data",
		},
		{
			name: "empty sticker filename",
			pack: &StickerPack{
				Stickers: []StickerPackItem{{FileName: "", Data: []byte("stk")}},
				TrayIcon: StickerPackTrayIcon{FileName: "tray.png", Data: []byte("data")},
			},
			wantErr: "non-empty fileName",
		},
		{
			name: "empty sticker data",
			pack: &StickerPack{
				Stickers: []StickerPackItem{{FileName: "0.webp", Data: nil}},
				TrayIcon: StickerPackTrayIcon{FileName: "tray.png", Data: []byte("data")},
			},
			wantErr: "empty data",
		},
		{
			name: "duplicate sticker filename",
			pack: &StickerPack{
				Stickers: []StickerPackItem{
					{FileName: "0.webp", Data: []byte("stk1")},
					{FileName: "0.webp", Data: []byte("stk2")},
				},
				TrayIcon: StickerPackTrayIcon{FileName: "tray.png", Data: []byte("data")},
			},
			wantErr: "duplicate fileName",
		},
		{
			name: "tray icon filename collision with sticker",
			pack: &StickerPack{
				Stickers: []StickerPackItem{
					{FileName: "tray.png", Data: []byte("stk1")},
				},
				TrayIcon: StickerPackTrayIcon{FileName: "tray.png", Data: []byte("data")},
			},
			wantErr: "collides with a sticker fileName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStickerPack(tt.pack)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
