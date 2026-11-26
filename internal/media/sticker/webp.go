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

func toWebP512(b []byte, crop Crop) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	const sz = 512
	dst := image.NewRGBA(image.Rect(0, 0, sz, sz))
	idraw.Draw(dst, dst.Bounds(), image.Transparent, image.Point{}, idraw.Src)
	src := crop.Bounds(img.Bounds())
	bw, bh := src.Dx(), src.Dy()
	scale := float64(sz) / float64(bw)
	if float64(bh)*scale > float64(sz) {
		scale = float64(sz) / float64(bh)
	}
	w := max(1, int(float64(bw)*scale))
	h := max(1, int(float64(bh)*scale))
	x := (sz - w) / 2
	y := (sz - h) / 2
	xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+w, y+h), img, src, xdraw.Over, nil)
	var out bytes.Buffer
	if err := webpenc.Encode(&out, dst, &webpenc.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Bounds selects the source area for a still image.
func (crop Crop) Bounds(src image.Rectangle) image.Rectangle {
	if crop == CropDefault || crop == CropFit {
		return src
	}
	side := min(src.Dx(), src.Dy())
	offset := image.Pt((src.Dx()-side)/2, (src.Dy()-side)/2)
	switch crop {
	case CropTop:
		offset.Y = 0
	case CropBottom:
		offset.Y = src.Dy() - side
	case CropLeft:
		offset.X = 0
	case CropRight:
		offset.X = src.Dx() - side
	}
	return image.Rect(0, 0, side, side).Add(src.Min.Add(offset))
}

// ScaleFilter formats an FFmpeg video stream as a square sticker canvas.
func (crop Crop) ScaleFilter() string {
	if crop == CropFit {
		return "format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0,setsar=1"
	}
	x, y := "(iw-ow)/2", "(ih-oh)/2"
	switch crop {
	case CropTop:
		y = "0"
	case CropBottom:
		y = "ih-oh"
	case CropLeft:
		x = "0"
	case CropRight:
		x = "iw-ow"
	}
	return fmt.Sprintf("crop=w='min(iw,ih)':h='min(iw,ih)':x='%s':y='%s':exact=1,scale=512:512,setsar=1", x, y)
}

func videoToWebP(ctx context.Context, data, exif []byte, animatedWebP bool, crop Crop) ([]byte, error) {
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

	// A zero duration encodes the entire clip. Lower quality before trimming.
	filter := "fps=15:start_time=0," + crop.ScaleFilter()
	for _, preset := range []struct{ quality, seconds int }{
		{45, 0}, {30, 0}, {20, 0}, {10, 0},
		{20, 10}, {15, 6}, {10, 3}, {8, 2},
	} {
		args := []string{
			"-hide_banner", "-loglevel", "error", "-y",
			"-copyts", "-start_at_zero", "-i", inFile,
		}
		if preset.seconds > 0 {
			args = append(args, "-t", strconv.Itoa(preset.seconds))
		}
		// Stop oversized encodes early. The cutoff sits above the upload
		// limit so a file cut off by FFmpeg cannot pass the size check.
		args = append(args,
			"-fs", strconv.Itoa(maxAnimatedStickerBytes+16_384),
			"-map", "0:V:0", "-an", "-sn", "-dn", "-vcodec", "libwebp", "-vf", filter,
			"-lossless", "0", "-q:v", strconv.Itoa(preset.quality),
			"-compression_level", "4", "-loop", "0", "-preset", "default", "-vsync", "0", outFile)
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
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
