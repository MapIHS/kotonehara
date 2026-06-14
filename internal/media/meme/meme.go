package meme

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	xwebp "golang.org/x/image/webp"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	defaultMaxDimension = 1024
	defaultMinDimension = 512
)

var (
	boldFontOnce sync.Once
	boldFont     *opentype.Font
	boldFontErr  error
)

//go:embed fonts/TitilliumWeb-Black.ttf
var memegenThickTTF []byte

type Options struct {
	TopText      string
	BottomText   string
	MaxDimension int
	MinDimension int
}

func Render(data []byte, opts Options) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("gambar kosong")
	}

	src, err := decodeSourceImage(data)
	if err != nil {
		return nil, fmt.Errorf("gambar belum bisa dibaca: %w", err)
	}

	maxDim := opts.MaxDimension
	if maxDim <= 0 {
		maxDim = defaultMaxDimension
	}
	minDim := opts.MinDimension
	if minDim <= 0 {
		minDim = defaultMinDimension
	}

	canvas := scaleForMeme(src, minDim, maxDim)
	if text := normalizeMemeText(opts.TopText); text != "" {
		if err := drawMemeText(canvas, text, true); err != nil {
			return nil, err
		}
	}
	if text := normalizeMemeText(opts.BottomText); text != "" {
		if err := drawMemeText(canvas, text, false); err != nil {
			return nil, err
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func decodeSourceImage(data []byte) (image.Image, error) {
	if isWebP(data) {
		if isAnimatedWebP(data) {
			return nil, fmt.Errorf("WebP animasi belum didukung untuk sticker meme")
		}

		img, err := xwebp.Decode(bytes.NewReader(data))
		if err == nil {
			return img, nil
		}
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func isWebP(data []byte) bool {
	return len(data) >= 12 &&
		string(data[:4]) == "RIFF" &&
		string(data[8:12]) == "WEBP"
}

func isAnimatedWebP(data []byte) bool {
	if !isWebP(data) {
		return false
	}
	return bytes.Contains(data, []byte("ANIM")) || bytes.Contains(data, []byte("ANMF"))
}

func scaleForMeme(src image.Image, minDim, maxDim int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	longest := max(w, h)
	if longest <= 0 {
		return image.NewRGBA(image.Rect(0, 0, defaultMinDimension, defaultMinDimension))
	}

	scale := 1.0
	if longest > maxDim {
		scale = float64(maxDim) / float64(longest)
	} else if longest < minDim {
		scale = float64(minDim) / float64(longest)
	}

	outW := max(1, int(math.Round(float64(w)*scale)))
	outH := max(1, int(math.Round(float64(h)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, outW, outH))
	draw.Draw(dst, dst.Bounds(), image.Black, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

func normalizeMemeText(text string) string {
	text = strings.TrimSpace(text)
	if text == "_" {
		return ""
	}
	return strings.ToUpper(text)
}

func drawMemeText(img *image.RGBA, text string, top bool) error {
	if text == "" {
		return nil
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("ukuran gambar tidak valid")
	}

	maxWidth := int(float64(w) * 0.92)
	maxHeight := int(float64(h) * 0.24)
	startSize := int(math.Min(float64(w)/9, float64(h)/5.8))
	if startSize < 16 {
		startSize = 16
	}

	layout, err := fitMemeText(text, maxWidth, maxHeight, startSize)
	if err != nil {
		return err
	}

	metrics := layout.face.Metrics()
	ascent := metrics.Ascent.Ceil()
	totalHeight := layout.lineHeight * len(layout.lines)
	margin := max(8, int(float64(h)*0.035))
	y := margin + ascent
	if !top {
		y = h - margin - totalHeight + ascent
	}

	for _, line := range layout.lines {
		drawCenteredLine(img, layout.face, line, w, y, layout.stroke)
		y += layout.lineHeight
	}
	return nil
}

type memeTextLayout struct {
	lines      []string
	face       font.Face
	lineHeight int
	stroke     int
}

func fitMemeText(text string, maxWidth, maxHeight, startSize int) (memeTextLayout, error) {
	var last memeTextLayout
	for size := startSize; size >= 12; size-- {
		face, err := memeFontFace(float64(size))
		if err != nil {
			return memeTextLayout{}, err
		}

		lines := wrapMemeText(face, text, maxWidth)
		lineHeight := max(1, int(math.Round(float64(face.Metrics().Height.Ceil())*0.92)))
		totalHeight := lineHeight * len(lines)
		if len(lines) > 0 {
			last = memeTextLayout{
				lines:      lines,
				face:       face,
				lineHeight: lineHeight,
				stroke:     max(1, size/16),
			}
		}
		if len(lines) > 0 && totalHeight <= maxHeight && widestLine(face, lines) <= maxWidth {
			return last, nil
		}
	}
	if len(last.lines) == 0 {
		return memeTextLayout{}, fmt.Errorf("teks meme kosong")
	}
	return last, nil
}

func memeFontFace(size float64) (font.Face, error) {
	return FontFace(size)
}

func FontFace(size float64) (font.Face, error) {
	boldFontOnce.Do(func() {
		boldFont, boldFontErr = opentype.Parse(memegenThickTTF)
	})
	if boldFontErr != nil {
		return nil, boldFontErr
	}
	return opentype.NewFace(boldFont, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func wrapMemeText(face font.Face, text string, maxWidth int) []string {
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
			out = append(out, splitLongMemeWord(face, line, maxWidth)...)
			line = word
		}
		out = append(out, splitLongMemeWord(face, line, maxWidth)...)
	}
	return out
}

func splitLongMemeWord(face font.Face, text string, maxWidth int) []string {
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

func drawCenteredLine(img *image.RGBA, face font.Face, text string, width, baselineY, stroke int) {
	advance := font.MeasureString(face, text).Ceil()
	x := (width - advance) / 2
	if x < 0 {
		x = 0
	}

	pt := fixed.P(x, baselineY)
	black := image.NewUniform(color.Black)
	white := image.NewUniform(color.White)

	for dy := -stroke; dy <= stroke; dy++ {
		for dx := -stroke; dx <= stroke; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if dx*dx+dy*dy > stroke*stroke {
				continue
			}
			d := font.Drawer{
				Dst:  img,
				Src:  black,
				Face: face,
				Dot:  fixed.P(x+dx, baselineY+dy),
			}
			d.DrawString(text)
		}
	}

	d := font.Drawer{
		Dst:  img,
		Src:  white,
		Face: face,
		Dot:  pt,
	}
	d.DrawString(text)
}
