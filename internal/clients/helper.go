package clients

import (
	"bytes"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

func (c *Client) MakeJPEGThumb(src []byte, maxW, maxH int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	ratio := float64(w) / float64(h)
	nw, nh := maxW, int(float64(maxW)/ratio)
	if nh > maxH {
		nh = maxH
		nw = int(float64(maxH) * ratio)
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 60}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (c *Client) DetectImageInfo(src []byte) (string, int, int, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return "", 0, 0, err
	}
	return "image/" + format, cfg.Width, cfg.Height, nil
}
