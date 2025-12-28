package sticker

import "context"

func BuildSticker(ctx context.Context, data []byte, author string, isSticker bool) ([]byte, error) {
	webp := data

	if !isSticker {
		w, err := toWebP512(data)
		if err != nil {
			return nil, err
		}
		webp = w
	}
	exif := buildExif(author)
	return webpmuxInjectEXIF(ctx, webp, exif)
}
