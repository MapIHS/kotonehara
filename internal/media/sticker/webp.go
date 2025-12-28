package sticker

import (
	"bytes"
	"image"
	idraw "image/draw"

	webpenc "github.com/chai2010/webp"
	xdraw "golang.org/x/image/draw"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

func toWebP512(b []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	const sz = 512
	dst := image.NewRGBA(image.Rect(0, 0, sz, sz))
	idraw.Draw(dst, dst.Bounds(), image.Transparent, image.Point{}, idraw.Src)
	bw, bh := img.Bounds().Dx(), img.Bounds().Dy()
	scale := float64(sz) / float64(bw)
	if float64(bh)*scale > float64(sz) {
		scale = float64(sz) / float64(bh)
	}
	w := int(float64(bw) * scale)
	h := int(float64(bh) * scale)
	x := (sz - w) / 2
	y := (sz - h) / 2
	xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+w, y+h), img, img.Bounds(), xdraw.Over, nil)
	var out bytes.Buffer
	if err := webpenc.Encode(&out, dst, &webpenc.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
