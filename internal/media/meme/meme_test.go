package meme

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/media/sticker"
)

func TestRender(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 80, A: 255})
		}
	}

	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatal(err)
	}

	out, err := Render(in.Bytes(), Options{
		TopText:    "hello from kotonehara",
		BottomText: "local meme renderer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("rendered meme is empty")
	}

	img, format, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" {
		t.Fatalf("format = %q, want png", format)
	}
	if img.Bounds().Dx() < defaultMinDimension && img.Bounds().Dy() < defaultMinDimension {
		t.Fatalf("rendered image too small: %v", img.Bounds())
	}
}

func TestRenderStaticWebPSticker(t *testing.T) {
	input := testPNG(t)
	webpSticker, err := sticker.BuildSticker(context.Background(), input, "tester", false, false)
	if err != nil {
		t.Fatal(err)
	}

	out, err := Render(webpSticker, Options{
		TopText:    "from sticker",
		BottomText: "works",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("rendered meme is empty")
	}
}

func TestRenderAnimatedWebPError(t *testing.T) {
	_, err := Render(fakeAnimatedWebP(), Options{TopText: "nope"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "WebP animasi") {
		t.Fatalf("error = %v, want animated WebP message", err)
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()

	src := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 80, A: 255})
		}
	}

	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatal(err)
	}
	return in.Bytes()
}

func fakeAnimatedWebP() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 22, 0, 0, 0, 'W', 'E', 'B', 'P',
		'V', 'P', '8', 'X', 10, 0, 0, 0, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		'A', 'N', 'I', 'M', 0, 0, 0, 0,
	}
}
