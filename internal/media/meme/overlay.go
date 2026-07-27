package meme

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
)

// RenderOverlay creates a 512x512 transparent PNG with meme text.
func RenderOverlay(opts Options) ([]byte, error) {
	dst := image.NewRGBA(image.Rect(0, 0, 512, 512))
	draw.Draw(dst, dst.Bounds(), image.Transparent, image.Point{}, draw.Src)

	if text := normalizeMemeText(opts.TopText); text != "" {
		if err := drawMemeText(dst, text, true); err != nil {
			return nil, err
		}
	}
	if text := normalizeMemeText(opts.BottomText); text != "" {
		if err := drawMemeText(dst, text, false); err != nil {
			return nil, err
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RenderAnimated creates an animated WebP meme (512x512) by compositing the meme text over the animated media.
func RenderAnimated(ctx context.Context, data []byte, opts Options) ([]byte, error) {
	overlay, err := RenderOverlay(opts)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "memeanim")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	inFile := filepath.Join(dir, "input")
	overlayFile := filepath.Join(dir, "overlay.png")
	outFile := filepath.Join(dir, "output.webp")

	if err := os.WriteFile(inFile, data, 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(overlayFile, overlay, 0600); err != nil {
		return nil, err
	}

	runFfmpeg := func(inputFile string) error {
		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-hide_banner",
			"-loglevel", "error",
			"-y",
			"-hwaccel", "auto",
			"-t", "10",
			"-i", inputFile,
			"-i", overlayFile,
			"-an", "-sn", "-dn",
			"-vcodec", "libwebp",
			"-filter_complex", "[0:v]fps=15,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:-1:-1:color=0x00000000@0,setsar=1[bg]; [bg][1:v]overlay=0:0[outv]",
			"-map", "[outv]",
			"-lossless", "0",
			"-q:v", "45",
			"-compression_level", "4",
			"-loop", "0",
			"-preset", "default",
			outFile,
		)
		return cmd.Run()
	}

	// Try ffmpeg first (works for most MP4s and GIFs)
	if err := runFfmpeg(inFile); err == nil {
		if outData, err := os.ReadFile(outFile); err == nil {
			return outData, nil
		}
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// If ffmpeg fails (e.g. WhatsApp Animated WebP), use ImageMagick to decode to a temporary GIF,
	// then use ffmpeg to overlay and encode the GIF to WebP.
	magickBin := "magick"
	if _, err := exec.LookPath(magickBin); err != nil {
		magickBin = "convert"
	}

	if _, err := exec.LookPath(magickBin); err == nil {
		tempGif := filepath.Join(dir, "temp.gif")
		cmdMagick := exec.CommandContext(ctx, magickBin, inFile, "-coalesce", tempGif)
		if err := cmdMagick.Run(); err == nil {
			if err := runFfmpeg(tempGif); err == nil {
				if outData, err := os.ReadFile(outFile); err == nil {
					return outData, nil
				}
			}
		}
	}

	return nil, errors.New("gagal render animasi meme: pastikan ImageMagick (magick) atau ffmpeg terpasang dan input valid")
}
