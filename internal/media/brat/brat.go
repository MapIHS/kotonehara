package brat

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"

	"github.com/MapIHS/kotonehara/internal/media/meme"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const canvasSize = 512

var (
	backgroundColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	textColor       = color.RGBA{R: 23, G: 23, B: 23, A: 255}
)

type Options struct {
	Text string
}

func Render(opts Options) ([]byte, error) {
	text := strings.TrimSpace(opts.Text)
	if text == "" {
		return nil, fmt.Errorf("teks kosong")
	}

	img := image.NewRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	draw.Draw(img, img.Bounds(), image.NewUniform(backgroundColor), image.Point{}, draw.Src)

	layout, err := fitText(text, 460, 419)
	if err != nil {
		return nil, err
	}

	drawBratText(img, layout)

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type textLayout struct {
	lines      []string
	face       font.Face
	lineHeight int
}

func fitText(text string, maxWidth, maxHeight int) (textLayout, error) {
	var last textLayout
	for size := 90; size >= 18; size-- {
		face, err := meme.FontFace(float64(size))
		if err != nil {
			return textLayout{}, err
		}
		lines := wrapText(face, text, maxWidth)
		lineHeight := max(1, int(math.Round(float64(face.Metrics().Height.Ceil())*0.88)))
		if len(lines) > 0 {
			last = textLayout{lines: lines, face: face, lineHeight: lineHeight}
		}
		if len(lines) > 0 && lineHeight*len(lines) <= maxHeight && widestLine(face, lines) <= maxWidth {
			return last, nil
		}
	}
	if len(last.lines) == 0 {
		return textLayout{}, fmt.Errorf("teks kosong")
	}
	return last, nil
}

func drawBratText(img *image.RGBA, layout textLayout) {
	totalHeight := layout.lineHeight * len(layout.lines)
	metrics := layout.face.Metrics()
	ascent := metrics.Ascent.Ceil()
	y := (canvasSize-totalHeight)/2 + ascent

	src := image.NewUniform(textColor)
	for _, line := range layout.lines {
		w := font.MeasureString(layout.face, line).Ceil()
		x := (canvasSize - w) / 2
		if x < 0 {
			x = 0
		}
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
