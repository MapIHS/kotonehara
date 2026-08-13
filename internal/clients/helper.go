package clients

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

func (c *Client) MakeJPEGThumb(src []byte, maxW, maxH int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	ratio := float64(w) / float64(h)
	nw, nh := maxW, int(float64(maxW)/ratio)
	if nh > maxH {
		nh = maxH
		nw = int(float64(maxH) * ratio)
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 60}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (c *Client) DetectImageInfo(src []byte) (string, int, int, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return "", 0, 0, err
	}
	return "image/" + format, cfg.Width, cfg.Height, nil
}

func (c *Client) MakeVideoThumb(ctx context.Context, src []byte, maxW, maxH int) ([]byte, error) {
	dir, err := os.MkdirTemp("", "vidthumb")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	inFile := filepath.Join(dir, "input")
	outFile := filepath.Join(dir, "thumb.jpg")

	if err := os.WriteFile(inFile, src, 0600); err != nil {
		return nil, err
	}

	scale := "scale=256:-1"
	if maxW > 0 && maxH > 0 {
		scale = "scale=min(" + strconv.Itoa(maxW) + ",iw):min(" + strconv.Itoa(maxH) + ",ih):force_original_aspect_ratio=decrease"
	} else if maxW > 0 {
		scale = "scale=" + strconv.Itoa(maxW) + ":-1"
	} else if maxH > 0 {
		scale = "scale=-1:" + strconv.Itoa(maxH)
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-i", inFile,
		"-frames:v", "1",
		"-vf", scale,
		"-q:v", "4",
		outFile,
	)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return os.ReadFile(outFile)
}

// VideoMeta holds basic video metadata extracted by ffprobe.
type VideoMeta struct {
	Width    uint32
	Height   uint32
	Duration uint32 // seconds
}

// ProbeVideoInfo extracts width, height, and duration from raw video bytes
// using ffprobe. Returns zero-value VideoMeta on any error (best-effort).
func ProbeVideoInfo(ctx context.Context, data []byte) VideoMeta {
	dir, err := os.MkdirTemp("", "videoprobe")
	if err != nil {
		return VideoMeta{}
	}
	defer os.RemoveAll(dir)

	inFile := filepath.Join(dir, "input.mp4")
	if err := os.WriteFile(inFile, data, 0600); err != nil {
		return VideoMeta{}
	}

	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration",
		"-show_entries", "format=duration",
		"-of", "csv=p=0:s=,",
		inFile,
	).Output()
	if err != nil {
		return VideoMeta{}
	}

	var meta VideoMeta
	// ffprobe outputs lines like "1280,720,123.456" (stream) and "123.456" (format)
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		parts := bytes.Split(line, []byte(","))
		if len(parts) >= 3 {
			w, _ := strconv.ParseUint(string(parts[0]), 10, 32)
			h, _ := strconv.ParseUint(string(parts[1]), 10, 32)
			if w > 0 && h > 0 {
				meta.Width = uint32(w)
				meta.Height = uint32(h)
			}
			if d, err := strconv.ParseFloat(string(parts[2]), 64); err == nil && d > 0 {
				meta.Duration = uint32(d)
			}
		} else if len(parts) == 1 && meta.Duration == 0 {
			if d, err := strconv.ParseFloat(string(parts[0]), 64); err == nil && d > 0 {
				meta.Duration = uint32(d)
			}
		}
	}
	return meta
}
