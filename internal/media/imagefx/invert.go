package imagefx

import (
	"image"
	"image/color"
)

func InvertImg(img image.Image) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			rr := uint8(255 - uint8(r>>8))
			gg := uint8(255 - uint8(g>>8))
			bb := uint8(255 - uint8(b>>8))
			aa := uint8(a >> 8)
			dst.SetRGBA(x, y, color.RGBA{R: rr, G: gg, B: bb, A: aa})
		}
	}
	return dst
}
