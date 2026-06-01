package sticker

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWebpRIFFInjectEXIFReplacesMetadataWithoutDecoding(t *testing.T) {
	input := fakeExtendedWebP()
	exif := []byte("new-exif")

	out, err := webpRIFFInjectEXIF(input, exif)
	if err != nil {
		t.Fatalf("webpRIFFInjectEXIF() error = %v", err)
	}

	if got, want := int(binary.LittleEndian.Uint32(out[4:8])), len(out)-8; got != want {
		t.Fatalf("RIFF size = %d, want %d", got, want)
	}

	vp8x := webpChunkPayloads(t, out, "VP8X")
	if len(vp8x) != 1 {
		t.Fatalf("VP8X chunk count = %d, want 1", len(vp8x))
	}
	if vp8x[0][0]&webpVP8XEXIFFlag == 0 {
		t.Fatal("VP8X EXIF flag was not set")
	}

	exifChunks := webpChunkPayloads(t, out, "EXIF")
	if len(exifChunks) != 1 {
		t.Fatalf("EXIF chunk count = %d, want 1", len(exifChunks))
	}
	if !bytes.Equal(exifChunks[0], exif) {
		t.Fatalf("EXIF payload = %q, want %q", exifChunks[0], exif)
	}
	if bytes.Contains(out, []byte("old-exif")) {
		t.Fatal("old EXIF payload was not removed")
	}
}

func fakeExtendedWebP() []byte {
	out := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}
	out = appendWebPChunk(out, []byte("VP8X"), []byte{0x02, 0, 0, 0, 0xff, 0x01, 0, 0xff, 0x01, 0})
	out = appendWebPChunk(out, []byte("ANIM"), []byte{0, 0, 0, 0, 0, 0})
	out = appendWebPChunk(out, []byte("EXIF"), []byte("old-exif"))
	out = appendWebPChunk(out, []byte("ANMF"), []byte("not-a-real-frame"))
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out
}

func webpChunkPayloads(t *testing.T, data []byte, fourCC string) [][]byte {
	t.Helper()

	var payloads [][]byte
	end := int(binary.LittleEndian.Uint32(data[4:8])) + 8
	for pos := 12; pos < end; {
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		dataStart := pos + 8
		dataEnd := dataStart + size
		if string(data[pos:pos+4]) == fourCC {
			payloads = append(payloads, data[dataStart:dataEnd])
		}
		pos = dataEnd + size%2
	}
	return payloads
}
