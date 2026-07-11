package sticker

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

const maxLottieDecompressedSize int64 = 32 << 20

var (
	errUnsupportedLottie = errors.New("format lottie/was belum dikenali")
	errLottieTooLarge    = errors.New("hasil dekompresi lottie melebihi batas")
)

func BuildLottieSticker(data []byte, author string) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("media kosong")
	}
	if out, ok, err := rewriteLottieZip(data, author); ok || err != nil {
		return out, err
	}
	if out, ok, err := rewriteGzipJSON(data, author); ok || err != nil {
		return out, err
	}
	if out, ok, err := rewriteZlibJSON(data, author); ok || err != nil {
		return out, err
	}
	if out, ok, err := rewriteJSON(data, author); ok || err != nil {
		return out, err
	}
	return nil, errUnsupportedLottie
}

func DescribeStickerData(data []byte, st *waE2E.StickerMessage) string {
	magicLen := len(data)
	if magicLen > 16 {
		magicLen = 16
	}
	parts := []string{
		fmt.Sprintf("len=%d", len(data)),
		fmt.Sprintf("magic=%s", strings.ToUpper(hex.EncodeToString(data[:magicLen]))),
		fmt.Sprintf("detect=%s", http.DetectContentType(data)),
	}
	if st != nil {
		parts = append(parts,
			fmt.Sprintf("mime=%s", st.GetMimetype()),
			fmt.Sprintf("animated=%t", st.GetIsAnimated()),
			fmt.Sprintf("lottie=%t", st.GetIsLottie()),
			fmt.Sprintf("thumb=%d", len(st.GetPngThumbnail())),
		)
	}
	return strings.Join(parts, " ")
}

func rewriteLottieZip(data []byte, author string) ([]byte, bool, error) {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return nil, false, nil
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, true, err
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	rewritten := false
	var decompressedSize int64
	for _, file := range zr.File {
		raw, err := readZipFile(file, maxLottieDecompressedSize-decompressedSize)
		if err != nil {
			_ = zw.Close()
			return nil, true, err
		}
		decompressedSize += int64(len(raw))

		if isJSONFile(file.Name) {
			if next, ok, err := rewriteJSONBytes(raw, author); err != nil {
				_ = zw.Close()
				return nil, true, fmt.Errorf("%s: %w", file.Name, err)
			} else if ok {
				raw = next
				rewritten = true
			}
		}

		h := file.FileHeader
		h.Method = file.Method
		writer, err := zw.CreateHeader(&h)
		if err != nil {
			_ = zw.Close()
			return nil, true, err
		}
		if _, err := writer.Write(raw); err != nil {
			_ = zw.Close()
			return nil, true, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, true, err
	}
	if !rewritten {
		return nil, true, errors.New("zip tidak berisi JSON metadata yang bisa diubah")
	}
	return out.Bytes(), true, nil
}

func rewriteGzipJSON(data []byte, author string) ([]byte, bool, error) {
	if !bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		return nil, false, nil
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, true, err
	}
	raw, err := readLottieData(gr, maxLottieDecompressedSize)
	closeErr := gr.Close()
	if err != nil {
		return nil, true, err
	}
	if closeErr != nil {
		return nil, true, closeErr
	}
	next, ok, err := rewriteJSONBytes(raw, author)
	if err != nil || !ok {
		return nil, true, err
	}

	var out bytes.Buffer
	gw := gzip.NewWriter(&out)
	if _, err := gw.Write(next); err != nil {
		_ = gw.Close()
		return nil, true, err
	}
	if err := gw.Close(); err != nil {
		return nil, true, err
	}
	return out.Bytes(), true, nil
}

func rewriteZlibJSON(data []byte, author string) ([]byte, bool, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, false, nil
	}
	raw, err := readLottieData(zr, maxLottieDecompressedSize)
	closeErr := zr.Close()
	if err != nil {
		return nil, true, err
	}
	if closeErr != nil {
		return nil, true, closeErr
	}
	next, ok, err := rewriteJSONBytes(raw, author)
	if err != nil || !ok {
		return nil, true, err
	}

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(next); err != nil {
		_ = zw.Close()
		return nil, true, err
	}
	if err := zw.Close(); err != nil {
		return nil, true, err
	}
	return out.Bytes(), true, nil
}

func rewriteJSON(data []byte, author string) ([]byte, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if !bytes.HasPrefix(trimmed, []byte("{")) {
		return nil, false, nil
	}
	out, ok, err := rewriteJSONBytes(trimmed, author)
	return out, ok, err
}

func rewriteJSONBytes(data []byte, author string) ([]byte, bool, error) {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, false, err
	}
	applyStickerMetadata(obj, author)
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func applyStickerMetadata(obj map[string]any, author string) {
	meta := stickerPack{
		StickerPackID:        packID,
		StickerPackName:      "Kotone Oohara",
		StickerPackPublisher: author,
		AndroidAppStoreLink:  playStore,
		IOSAppStoreLink:      appleStore,
	}
	b, _ := json.Marshal(meta)
	var fields map[string]any
	_ = json.Unmarshal(b, &fields)
	for key, value := range fields {
		obj[key] = value
	}

	for _, key := range []string{"meta", "metadata", "sticker", "stickerPack", "sticker_pack"} {
		if nested, ok := obj[key].(map[string]any); ok {
			for fieldKey, value := range fields {
				nested[fieldKey] = value
			}
		}
	}
}

func readZipFile(file *zip.File, limit int64) ([]byte, error) {
	if limit < 0 || file.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%w %d byte", errLottieTooLarge, maxLottieDecompressedSize)
	}

	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return readLottieData(rc, limit)
}

func readLottieData(r io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("%w %d byte", errLottieTooLarge, maxLottieDecompressedSize)
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w %d byte", errLottieTooLarge, maxLottieDecompressedSize)
	}
	return data, nil
}

func isJSONFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	base := strings.ToLower(filepath.Base(name))
	return ext == ".json" || base == "manifest" || base == "manifest.json"
}
