package sticker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	idraw "image/draw"
	"os"
	"path/filepath"
	"strconv"

	exec "github.com/MapIHS/kotonehara/internal/service/mediaproc"
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

func videoToWebP(ctx context.Context, data, exif []byte, animatedWebP bool) ([]byte, error) {
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

	if animatedWebP {
		gifFile := filepath.Join(dir, "frames.gif")
		if err := exec.CommandContext(ctx, "magick", inFile, "-coalesce", gifFile).Run(); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if out, err := exec.CommandContext(ctx, "convert", inFile, "-coalesce", gifFile).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("decode WebP animasi: %w: %s", err, out)
			}
		}
		inFile = gifFile
	}

	// Keep the original duration on the first retries. Shorten only when
	// reducing quality and frame rate still cannot meet the upload limit.
	for _, preset := range []struct{ fps, quality, seconds int }{
		{15, 45, 10}, {12, 30, 10}, {8, 20, 10}, {8, 15, 6}, {6, 10, 3}, {5, 8, 2},
	} {
		filter := fmt.Sprintf("fps=%d,crop=w='min(iw,ih)':h='min(iw,ih)':x='(iw-ow)/2':y='(ih-oh)/2':exact=1,scale=512:512,setsar=1", preset.fps)
		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-hide_banner", "-loglevel", "error", "-y",
			"-t", strconv.Itoa(preset.seconds), "-i", inFile,
			"-an", "-sn", "-dn", "-vcodec", "libwebp", "-vf", filter,
			"-lossless", "0", "-q:v", strconv.Itoa(preset.quality),
			"-compression_level", "4", "-loop", "0", "-preset", "default", "-vsync", "0", outFile)
		if out, err := cmd.CombinedOutput(); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("encode stiker animasi: %w: %s", err, bytes.TrimSpace(out))
		}
		info, err := os.Stat(outFile)
		if err != nil {
			return nil, err
		}
		if info.Size() > maxAnimatedStickerBytes {
			continue
		}
		raw, err := os.ReadFile(outFile)
		if err != nil {
			return nil, err
		}
		final, err := injectWebPEXIF(ctx, raw, exif)
		if err != nil {
			return nil, err
		}
		if len(final) <= maxAnimatedStickerBytes {
			return final, nil
		}
	}
	return nil, ErrAnimatedStickerTooLarge
}
