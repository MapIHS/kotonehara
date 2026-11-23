package brat

import (
	"bytes"
	"image"
	"testing"
)

func TestRender(t *testing.T) {
	out, err := Render(Options{Text: "ihsan si malas"})
	if err != nil {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" {
		t.Fatalf("format = %q, want png", format)
	}
	if img.Bounds().Dx() != canvasSize || img.Bounds().Dy() != canvasSize {
		t.Fatalf("bounds = %v, want %dx%d", img.Bounds(), canvasSize, canvasSize)
	}
}
