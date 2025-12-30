package imagefx

import (
	"image"
	"image/color"
)

func ToGray(img image.Image) *image.Gray {
	b := img.Bounds()
	dst := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			yr := uint8((r*299 + g*587 + b*114 + 500) / 1000 >> 8)
			dst.SetGray(x, y, color.Gray{Y: yr})
		}
	}
	return dst
}
