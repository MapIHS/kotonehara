package sticker

import (
	"context"
	"errors"
)

const maxAnimatedStickerBytes = 500_000

var ErrAnimatedStickerTooLarge = errors.New("stiker animasi masih melebihi 500 KB setelah dikompres")

func BuildSticker(ctx context.Context, data []byte, author string, isSticker bool, isVideo bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("media kosong")
	}

	exif := buildExif(author)
	animatedWebP := len(data) >= 30 && string(data[:4]) == "RIFF" &&
		string(data[8:16]) == "WEBPVP8X" && data[20]&0x02 != 0
	if isSticker {
		out, err := injectWebPEXIF(ctx, data, exif)
		if err == nil && (!animatedWebP || len(out) <= maxAnimatedStickerBytes) {
			return out, nil
		}
	}

	if isVideo || animatedWebP {
		return videoToWebP(ctx, data, exif, animatedWebP)
	}
	webp, err := toWebP512(data)
	if err != nil {
		return nil, err
	}
	return injectWebPEXIF(ctx, webp, exif)
}
