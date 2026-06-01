package sticker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const webpVP8XEXIFFlag = 0x08

type webpRIFFChunk struct {
	start     int
	dataStart int
	dataEnd   int
	end       int
	size      int
}

func webpRIFFInjectEXIF(input []byte, exif []byte) ([]byte, error) {
	if len(exif) == 0 {
		return nil, errors.New("exif kosong")
	}
	if len(input) < 12 || !bytes.Equal(input[:4], []byte("RIFF")) || !bytes.Equal(input[8:12], []byte("WEBP")) {
		return nil, errors.New("bukan RIFF WebP")
	}
	if len(exif) > math.MaxUint32 {
		return nil, errors.New("exif terlalu besar")
	}

	riffSize := uint64(binary.LittleEndian.Uint32(input[4:8]))
	if riffSize+8 > uint64(len(input)) {
		return nil, errors.New("ukuran RIFF melebihi file")
	}
	parseEnd := int(riffSize) + 8
	if parseEnd < 12 {
		return nil, errors.New("ukuran RIFF invalid")
	}

	chunks := make([]webpRIFFChunk, 0, 8)
	vp8xIndex := -1
	for pos := 12; pos < parseEnd; {
		if pos+8 > parseEnd {
			return nil, fmt.Errorf("header chunk terpotong di offset %d", pos)
		}

		size := int(binary.LittleEndian.Uint32(input[pos+4 : pos+8]))
		dataStart := pos + 8
		dataEnd := dataStart + size
		if dataEnd < dataStart || dataEnd > parseEnd {
			return nil, fmt.Errorf("chunk %q melebihi file", input[pos:pos+4])
		}

		end := dataEnd
		if size%2 == 1 {
			end++
		}
		if end > parseEnd {
			return nil, fmt.Errorf("padding chunk %q melebihi file", input[pos:pos+4])
		}

		chunk := webpRIFFChunk{
			start:     pos,
			dataStart: dataStart,
			dataEnd:   dataEnd,
			end:       end,
			size:      size,
		}
		if bytes.Equal(input[pos:pos+4], []byte("VP8X")) {
			vp8xIndex = len(chunks)
		}
		chunks = append(chunks, chunk)
		pos = end
	}
	if vp8xIndex < 0 {
		return nil, errors.New("VP8X chunk tidak ada")
	}
	if chunks[vp8xIndex].size < 10 {
		return nil, errors.New("VP8X chunk invalid")
	}

	out := make([]byte, 0, len(input)+8+len(exif)+len(exif)%2)
	out = append(out, input[:12]...)
	for i, chunk := range chunks {
		if bytes.Equal(input[chunk.start:chunk.start+4], []byte("EXIF")) {
			continue
		}
		if i != vp8xIndex {
			out = append(out, input[chunk.start:chunk.end]...)
			continue
		}

		out = append(out, input[chunk.start:chunk.dataStart]...)
		vp8x := append([]byte(nil), input[chunk.dataStart:chunk.dataEnd]...)
		vp8x[0] |= webpVP8XEXIFFlag
		out = append(out, vp8x...)
		if chunk.size%2 == 1 {
			out = append(out, 0)
		}
	}

	out = appendWebPChunk(out, []byte("EXIF"), exif)
	if len(out)-8 > math.MaxUint32 {
		return nil, errors.New("WebP terlalu besar")
	}
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, nil
}

func appendWebPChunk(out []byte, fourCC []byte, payload []byte) []byte {
	out = append(out, fourCC...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	out = append(out, size[:]...)
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0)
	}
	return out
}
