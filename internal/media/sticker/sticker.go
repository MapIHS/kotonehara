package sticker

import "context"

func BuildSticker(ctx context.Context, data []byte, author string, isSticker bool, isVideo bool) ([]byte, error) {
	webp := data

	if !isSticker {
		if isVideo {
			w, err := videoToWebP(ctx, data)
			if err != nil {
				return nil, err
			}
			webp = w
		} else {
			w, err := toWebP512(data)
			if err != nil {
				return nil, err
			}
			webp = w
		}
	}
	exif := buildExif(author)
	return webpmuxInjectEXIF(ctx, webp, exif)
}
