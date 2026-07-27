package sticker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	idraw "image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

func videoToWebP(ctx context.Context, data []byte) ([]byte, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, errors.New("ffmpeg belum terpasang")
	}

	dir, err := os.MkdirTemp("", "vidtowebp")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	inFile := filepath.Join(dir, "input")
	outFile := filepath.Join(dir, "output.webp")

	if err := os.WriteFile(inFile, data, 0600); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-hwaccel", "auto",
		"-t", "10",
		"-i", inFile,
		"-an",
		"-sn",
		"-dn",
		"-vcodec", "libwebp",
		"-vf", "fps=15,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:-1:-1:color=0x00000000@0,setsar=1",
		"-lossless", "0",
		"-q:v", "45",
		"-compression_level", "4",
		"-loop", "0",
		"-preset", "default",
		"-vsync", "0",
		outFile,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(out) > 0 {
			msg := string(bytes.TrimSpace(out))
			if strings.Contains(msg, "no decoder found") || strings.Contains(msg, "Decoder") {
				return nil, fmt.Errorf("codec video tidak didukung oleh ffmpeg (install ffmpeg lengkap dari RPM Fusion): %s", msg)
			}
			return nil, errors.New(msg)
		}
		return nil, err
	}

	return os.ReadFile(outFile)
}
