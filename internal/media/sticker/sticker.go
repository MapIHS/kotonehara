package sticker

import (
	"bytes"
	"context"
	"errors"
)

type Crop string

const (
	CropDefault Crop = ""
	CropFit     Crop = "fit"
	CropCenter  Crop = "center"
	CropTop     Crop = "top"
	CropBottom  Crop = "bottom"
	CropLeft    Crop = "left"
	CropRight   Crop = "right"
)

const maxAnimatedStickerBytes = 1_000_000

var ErrAnimatedStickerTooLarge = errors.New("stiker animasi masih melebihi 1 MB setelah dikompres")

func BuildSticker(ctx context.Context, data []byte, author string, isSticker bool, isVideo bool) ([]byte, error) {
	return BuildStickerWithCrop(ctx, data, author, isSticker, isVideo, CropDefault)
}

func BuildStickerWithCrop(ctx context.Context, data []byte, author string, isSticker, isVideo bool, crop Crop) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("media kosong")
	}

	exif := buildExif(author)
	animatedWebP := IsAnimatedWebP(data)
	if isSticker && crop == CropDefault {
		out, err := injectWebPEXIF(ctx, data, exif)
		if err == nil && (!animatedWebP || len(out) <= maxAnimatedStickerBytes) {
			return out, nil
		}
	}

	if isVideo || animatedWebP || IsGIF(data) {
		return videoToWebP(ctx, data, exif, animatedWebP, crop)
	}
	webp, err := toWebP512(data, crop)
	if err != nil {
		return nil, err
	}
	return injectWebPEXIF(ctx, webp, exif)
}

func IsGIF(data []byte) bool {
	return bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))
}

func IsAnimatedWebP(data []byte) bool {
	return len(data) >= 30 && string(data[:4]) == "RIFF" &&
		string(data[8:16]) == "WEBPVP8X" && data[20]&0x02 != 0
}
