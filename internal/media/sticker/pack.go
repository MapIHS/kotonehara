package sticker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

const (
	signatureLocalFile       uint32 = 0x04034b50
	signatureCentralDir      uint32 = 0x02014b50
	signatureEndOfCentralDir uint32 = 0x06054b50
	compressionStored        uint16 = 0
	versionNeeded            uint16 = 20
	localHeaderSize          int    = 30
	centralHeaderSize        int    = 46
	eocdSize                 int    = 22
)

type StickerPackItem struct {
	FileName   string
	Data       []byte
	Emojis     []string
	IsAnimated bool
	IsLottie   bool
	Mimetype   string
}

type StickerPackTrayIcon struct {
	FileName string
	Data     []byte
}

type StickerPack struct {
	ID              string
	Name            string
	Publisher       string
	Description     string
	Caption         string
	Stickers        []StickerPackItem
	TrayIcon        StickerPackTrayIcon
	CoverThumbnail  []byte
	ThumbnailWidth  uint32
	ThumbnailHeight uint32
}

// ValidateStickerPack validates that a StickerPack has all required fields and entries.
func ValidateStickerPack(pack *StickerPack) error {
	if pack == nil {
		return errors.New("sticker pack is nil")
	}
	if len(pack.Stickers) == 0 {
		return errors.New("sticker pack requires at least one sticker")
	}
	if pack.TrayIcon.FileName == "" {
		return errors.New("sticker pack trayIcon requires a non-empty fileName")
	}
	if len(pack.TrayIcon.Data) == 0 {
		return errors.New("sticker pack trayIcon requires non-empty data")
	}

	seen := make(map[string]struct{}, len(pack.Stickers)+1)
	for i, s := range pack.Stickers {
		if s.FileName == "" {
			return fmt.Errorf("sticker at index %d requires a non-empty fileName", i)
		}
		if len(s.Data) == 0 {
			return fmt.Errorf("sticker %q has empty data", s.FileName)
		}
		if _, exists := seen[s.FileName]; exists {
			return fmt.Errorf("sticker pack has duplicate fileName: %s", s.FileName)
		}
		seen[s.FileName] = struct{}{}
	}

	if _, exists := seen[pack.TrayIcon.FileName]; exists {
		return fmt.Errorf("sticker pack trayIcon.fileName %s collides with a sticker fileName; the tray icon must be a separate ZIP entry", pack.TrayIcon.FileName)
	}

	return nil
}

type centralEntry struct {
	name        []byte
	crc         uint32
	dataLength  uint32
	localOffset uint32
}

func buildLocalHeader(name []byte, crc uint32, dataLength uint32) []byte {
	out := make([]byte, localHeaderSize+len(name))
	binary.LittleEndian.PutUint32(out[0:4], signatureLocalFile)
	binary.LittleEndian.PutUint16(out[4:6], versionNeeded)
	binary.LittleEndian.PutUint16(out[6:8], 0) // flags = 0, NO data descriptor!
	binary.LittleEndian.PutUint16(out[8:10], compressionStored)
	binary.LittleEndian.PutUint16(out[10:12], 0) // last mod time
	binary.LittleEndian.PutUint16(out[12:14], 0) // last mod date
	binary.LittleEndian.PutUint32(out[14:18], crc)
	binary.LittleEndian.PutUint32(out[18:22], dataLength)
	binary.LittleEndian.PutUint32(out[22:26], dataLength)
	binary.LittleEndian.PutUint16(out[26:28], uint16(len(name)))
	binary.LittleEndian.PutUint16(out[28:30], 0) // extra field length = 0
	copy(out[localHeaderSize:], name)
	return out
}

func buildCentralDirectory(entries []centralEntry) []byte {
	var totalSize int
	for _, e := range entries {
		totalSize += centralHeaderSize + len(e.name)
	}
	out := make([]byte, totalSize)
	offset := 0
	for _, e := range entries {
		binary.LittleEndian.PutUint32(out[offset:offset+4], signatureCentralDir)
		binary.LittleEndian.PutUint16(out[offset+4:offset+6], versionNeeded)
		binary.LittleEndian.PutUint16(out[offset+6:offset+8], versionNeeded)
		binary.LittleEndian.PutUint16(out[offset+8:offset+10], 0) // flags = 0
		binary.LittleEndian.PutUint16(out[offset+10:offset+12], compressionStored)
		binary.LittleEndian.PutUint16(out[offset+12:offset+14], 0) // last mod time
		binary.LittleEndian.PutUint16(out[offset+14:offset+16], 0) // last mod date
		binary.LittleEndian.PutUint32(out[offset+16:offset+20], e.crc)
		binary.LittleEndian.PutUint32(out[offset+20:offset+24], e.dataLength)
		binary.LittleEndian.PutUint32(out[offset+24:offset+28], e.dataLength)
		binary.LittleEndian.PutUint16(out[offset+28:offset+30], uint16(len(e.name)))
		binary.LittleEndian.PutUint16(out[offset+30:offset+32], 0) // extra field length = 0
		binary.LittleEndian.PutUint16(out[offset+32:offset+34], 0) // comment length = 0
		binary.LittleEndian.PutUint16(out[offset+34:offset+36], 0) // disk number start
		binary.LittleEndian.PutUint16(out[offset+36:offset+38], 0) // internal file attrs
		binary.LittleEndian.PutUint32(out[offset+38:offset+42], 0) // external file attrs
		binary.LittleEndian.PutUint32(out[offset+42:offset+46], e.localOffset)
		offset += centralHeaderSize
		copy(out[offset:offset+len(e.name)], e.name)
		offset += len(e.name)
	}
	return out
}

func buildEOCD(entryCount uint16, centralDirSize uint32, centralDirOffset uint32) []byte {
	out := make([]byte, eocdSize)
	binary.LittleEndian.PutUint32(out[0:4], signatureEndOfCentralDir)
	binary.LittleEndian.PutUint16(out[4:6], 0) // disk number
	binary.LittleEndian.PutUint16(out[6:8], 0) // start disk number
	binary.LittleEndian.PutUint16(out[8:10], entryCount)
	binary.LittleEndian.PutUint16(out[10:12], entryCount)
	binary.LittleEndian.PutUint32(out[12:16], centralDirSize)
	binary.LittleEndian.PutUint32(out[16:20], centralDirOffset)
	binary.LittleEndian.PutUint16(out[20:22], 0) // comment length = 0
	return out
}

func BuildStickerPackZip(pack *StickerPack) ([]byte, error) {
	if err := ValidateStickerPack(pack); err != nil {
		return nil, err
	}

	type rawEntry struct {
		name []byte
		data []byte
	}

	rawEntries := make([]rawEntry, 0, len(pack.Stickers)+1)
	for _, s := range pack.Stickers {
		rawEntries = append(rawEntries, rawEntry{
			name: []byte(s.FileName),
			data: s.Data,
		})
	}
	rawEntries = append(rawEntries, rawEntry{
		name: []byte(pack.TrayIcon.FileName),
		data: pack.TrayIcon.Data,
	})

	var buf bytes.Buffer
	central := make([]centralEntry, 0, len(rawEntries))
	var offset uint32

	for _, entry := range rawEntries {
		crc := crc32.ChecksumIEEE(entry.data)
		dataLen := uint32(len(entry.data))
		header := buildLocalHeader(entry.name, crc, dataLen)

		buf.Write(header)
		buf.Write(entry.data)

		central = append(central, centralEntry{
			name:        entry.name,
			crc:         crc,
			dataLength:  dataLen,
			localOffset: offset,
		})

		offset += uint32(len(header)) + dataLen
	}

	centralDir := buildCentralDirectory(central)
	eocd := buildEOCD(uint16(len(central)), uint32(len(centralDir)), offset)

	buf.Write(centralDir)
	buf.Write(eocd)

	return buf.Bytes(), nil
}
