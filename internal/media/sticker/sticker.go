package sticker

import (
	"context"
	"errors"
	"fmt"
)

func BuildSticker(ctx context.Context, data []byte, author string, isSticker bool, isVideo bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("media kosong")
	}

	exif := buildExif(author)

	if isSticker {
		out, err := injectWebPEXIF(ctx, data, exif)
		if err == nil {
			return out, nil
		}

		webp, convErr := stickerFallbackToWebP(ctx, data, isVideo)
		if convErr != nil {
			return nil, fmt.Errorf("sticker bukan WebP valid dan gagal convert ulang: %w (inject exif: %v)", convErr, err)
		}
		return injectWebPEXIF(ctx, webp, exif)
	}

	webp, err := stickerFallbackToWebP(ctx, data, isVideo)
	if err != nil {
		return nil, err
	}
	return injectWebPEXIF(ctx, webp, exif)
}

func stickerFallbackToWebP(ctx context.Context, data []byte, isVideo bool) ([]byte, error) {
	if isVideo {
		return videoToWebP(ctx, data)
	}
	return toWebP512(data)
}
