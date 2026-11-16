package imagefx

import (
	"image"
	"image/color"
	"testing"
)

func sample() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	img.SetRGBA(1, 0, color.RGBA{R: 10, G: 20, B: 30, A: 128})
	return img
}

func TestInvert(t *testing.T) {
	out := InvertImg(sample())
	if got := out.RGBAAt(0, 0); got != (color.RGBA{R: 0, G: 255, B: 255, A: 255}) {
		t.Fatalf("pixel 0: %+v", got)
	}
	if got := out.RGBAAt(1, 0); got.A != 128 {
		t.Fatalf("alpha not preserved: %+v", got)
	}
}

func TestGrayLuma(t *testing.T) {
	out := ToGray(sample())
	// Pure red has Rec.601 luma 0.299 * 255 ≈ 76.
	if got := out.GrayAt(0, 0).Y; got < 75 || got > 77 {
		t.Fatalf("red luma = %d", got)
	}
	if out.Bounds() != sample().Bounds() {
		t.Fatal("bounds changed")
	}
}
