package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/MapIHS/kotonehara/internal/service/openai"
	_ "golang.org/x/image/webp"
)

const (
	maxAIImageBytes  = 10 << 20
	maxAIImagePixels = 20_000_000
	maxAIImages      = 4
)

// AI-provided URLs are untrusted; reuse the fetch command's DNS pinning and
// redirect checks. Image hosts never receive the AI provider's bearer token.
func loadAIImage(ctx context.Context, source string, base *http.Client, resolver fetchResolver) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var data []byte
	if strings.HasPrefix(strings.ToLower(source), "data:") {
		header, encoded, ok := strings.Cut(source, ",")
		if !ok || !strings.HasPrefix(strings.ToLower(header), "data:image/") || !strings.HasSuffix(strings.ToLower(header), ";base64") {
			return nil, fmt.Errorf("format data gambar AI tidak didukung")
		}
		if base64.StdEncoding.DecodedLen(len(encoded)) > maxAIImageBytes {
			return nil, fmt.Errorf("gambar AI melebihi batas 10 MB")
		}
		var err error
		data, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("data gambar AI tidak valid")
		}
	} else {
		imageCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		defer cancel()
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		u, err := parseFetchURL(source)
		if err != nil || u.User != nil {
			return nil, fmt.Errorf("URL gambar AI tidak valid")
		}
		if err := validateFetchURL(imageCtx, u, resolver); err != nil {
			return nil, fmt.Errorf("alamat gambar AI tidak dapat diakses")
		}
		req, err := newFetchRequest(imageCtx, http.MethodGet, u)
		if err != nil {
			return nil, fmt.Errorf("gagal membuat permintaan gambar AI")
		}
		req.Header.Set("Accept", "image/*")
		resp, err := safeFetchHTTPClient(base, resolver).Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("gagal mengambil gambar AI")
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("server gambar AI mengembalikan HTTP %d", resp.StatusCode)
		}
		if resp.ContentLength > maxAIImageBytes {
			return nil, fmt.Errorf("gambar AI melebihi batas 10 MB")
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxAIImageBytes+1))
		if err != nil {
			return nil, fmt.Errorf("gagal membaca gambar AI")
		}
	}
	if len(data) == 0 || len(data) > maxAIImageBytes {
		return nil, fmt.Errorf("ukuran gambar AI tidak valid")
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		return nil, fmt.Errorf("hasil AI bukan gambar JPEG, PNG, atau WebP")
	}
	info, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || info.Width <= 0 || info.Height <= 0 || int64(info.Width) > maxAIImagePixels/int64(info.Height) {
		return nil, fmt.Errorf("gambar AI rusak atau resolusinya terlalu besar")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("data gambar AI tidak lengkap atau rusak")
	}
	return data, nil
}

// Send text once, then send image bytes as WhatsApp images. Continue after one
// image fails so a broken URL does not discard other usable provider results.
func sendAIReply(ctx context.Context, reply openai.Reply, sendText func(string) error, sendImage func([]byte) error, loadImage func(context.Context, string) ([]byte, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text := strings.TrimSpace(reply.Text)
	if len(text) > maxAIReplySize {
		text = text[:maxAIReplySize-5]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		text += "\n\n..."
	}
	if text != "" {
		if err := sendText(text); err != nil {
			return fmt.Errorf("pesan teks AI gagal dikirim")
		}
	}
	images := reply.Images
	if len(images) > maxAIImages {
		images = images[:maxAIImages]
	}
	failed := 0
	for _, source := range images {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := loadImage(ctx, source)
		if err != nil {
			failed++
			continue
		}
		if err := sendImage(data); err != nil {
			failed++
		}
	}
	if failed != 0 {
		return fmt.Errorf("%d dari %d gambar AI gagal dikirim", failed, len(images))
	}
	return nil
}
