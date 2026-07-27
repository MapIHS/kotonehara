package brat

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os/exec"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed fonts/Arial.ttf
var arialFontTTF []byte

var (
	arialFontOnce sync.Once
	arialFont     *opentype.Font
	arialFontErr  error
)

func fontFace(size float64) (font.Face, error) {
	arialFontOnce.Do(func() {
		arialFont, arialFontErr = opentype.Parse(arialFontTTF)
	})
	if arialFontErr != nil {
		return nil, arialFontErr
	}
	return opentype.NewFace(arialFont, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

const canvasSize = 512

var (
	backgroundColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	textColor       = color.RGBA{R: 23, G: 23, B: 23, A: 255}
)

type Options struct {
	Text string
}

func Render(opts Options) ([]byte, error) {
	text := strings.ToLower(strings.TrimSpace(opts.Text))
	if text == "" {
		return nil, fmt.Errorf("teks kosong")
	}

	layout, fontSize, err := fitText(text, 460, 419)
	if err != nil {
		return nil, err
	}

	// Try using ImageMagick (Pango) to render emojis properly.
	// If ImageMagick is missing or fails, fallback to native Go rendering (no emoji support).
	if out, err := renderWithMagick(layout.lines, fontSize); err == nil && len(out) > 0 {
		return out, nil
	}

	img := image.NewRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	draw.Draw(img, img.Bounds(), image.NewUniform(backgroundColor), image.Point{}, draw.Src)

	drawBratText(img, layout)

	// Add the signature "brat" blur by downscaling and upscaling
	blurFactor := 3
	smallCanvasSize := canvasSize / blurFactor
	small := image.NewRGBA(image.Rect(0, 0, smallCanvasSize, smallCanvasSize))
	xdraw.BiLinear.Scale(small, small.Bounds(), img, img.Bounds(), xdraw.Src, nil)
	
	blurred := image.NewRGBA(img.Bounds())
	xdraw.BiLinear.Scale(blurred, blurred.Bounds(), small, small.Bounds(), xdraw.Src, nil)

	var out bytes.Buffer
	if err := png.Encode(&out, blurred); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func renderWithMagick(lines []string, fontSize int) ([]byte, error) {
	magickBin := "magick"
	if _, err := exec.LookPath(magickBin); err != nil {
		magickBin = "convert"
		if _, err := exec.LookPath(magickBin); err != nil {
			return nil, err
		}
	}

	text := strings.Join(lines, "\n")
	escaped := html.EscapeString(text)
	// Explicitly declare emoji fallback fonts so Pango doesn't drop them
	pangoMarkup := fmt.Sprintf("<span font='Arial, Noto Color Emoji, Apple Color Emoji, Segoe UI Emoji %d'>%s</span>", fontSize, escaped)

	cmd := exec.Command(magickBin,
		"-density", "72",
		"-size", "512x512", "canvas:white",
		"(", "-background", "white", "-fill", "#171717", "+size", "pango:"+pangoMarkup, "-resize", "460x419>", ")",
		"-gravity", "center", "-composite",
		"-filter", "Triangle", "-resize", "170x170!", "-resize", "512x512!",
		"png:-",
	)

	return cmd.Output()
}

type textLayout struct {
	lines      []string
	face       font.Face
	lineHeight int
}

func fitText(text string, maxWidth, maxHeight int) (textLayout, int, error) {
	var last textLayout
	var lastSize int
	for size := 200; size >= 18; size-- {
		face, err := fontFace(float64(size))
		if err != nil {
			return textLayout{}, 0, err
		}
		lines := wrapText(face, text, maxWidth)
		lineHeight := max(1, int(math.Round(float64(face.Metrics().Height.Ceil())*0.88)))
		if len(lines) > 0 {
			last = textLayout{lines: lines, face: face, lineHeight: lineHeight}
			lastSize = size
		}
		if len(lines) > 0 && lineHeight*len(lines) <= maxHeight && widestLine(face, lines) <= maxWidth {
			return last, lastSize, nil
		}
	}
	if len(last.lines) == 0 {
		return textLayout{}, 0, fmt.Errorf("teks kosong")
	}
	return last, lastSize, nil
}

func drawBratText(img *image.RGBA, layout textLayout) {
	totalHeight := layout.lineHeight * len(layout.lines)
	metrics := layout.face.Metrics()
	ascent := metrics.Ascent.Ceil()
	y := (canvasSize-totalHeight)/2 + ascent

	widest := widestLine(layout.face, layout.lines)
	blockX := (canvasSize - widest) / 2
	if blockX < 0 {
		blockX = 0
	}

	src := image.NewUniform(textColor)
	for _, line := range layout.lines {
		x := blockX
		d := font.Drawer{
			Dst:  img,
			Src:  src,
			Face: layout.face,
			Dot:  fixed.P(x, y),
		}
		d.DrawString(line)
		y += layout.lineHeight
	}
}

func wrapText(face font.Face, text string, maxWidth int) []string {
	var out []string
	for _, paragraph := range strings.Split(text, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		words := strings.Fields(paragraph)
		if len(words) == 0 {
			continue
		}

		line := words[0]
		for _, word := range words[1:] {
			candidate := line + " " + word
			if textWidth(face, candidate) <= maxWidth {
				line = candidate
				continue
			}
			out = append(out, splitLongWord(face, line, maxWidth)...)
			line = word
		}
		out = append(out, splitLongWord(face, line, maxWidth)...)
	}
	return out
}

func splitLongWord(face font.Face, text string, maxWidth int) []string {
	if textWidth(face, text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	current := ""
	for _, r := range text {
		candidate := current + string(r)
		if current != "" && textWidth(face, candidate) > maxWidth {
			lines = append(lines, current)
			current = string(r)
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func widestLine(face font.Face, lines []string) int {
	widest := 0
	for _, line := range lines {
		widest = max(widest, textWidth(face, line))
	}
	return widest
}

func textWidth(face font.Face, text string) int {
	return font.MeasureString(face, text).Ceil()
}
