package sticker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	exec "github.com/MapIHS/kotonehara/internal/service/mediaproc"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// HoldStill builds a two-frame animation so a static sticker can play audio.
func HoldStill(ctx context.Context, data []byte, author string, duration time.Duration) ([]byte, error) {
	frame, err := toWebP512(data, CropFit)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "sticker-still")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "frame.webp"), filepath.Join(dir, "animated.webp")
	if err := os.WriteFile(in, frame, 0600); err != nil {
		return nil, err
	}
	ms := max(int64(40), duration.Milliseconds())
	first, second := "+"+strconv.FormatInt(ms/2, 10), "+"+strconv.FormatInt(ms-ms/2, 10)
	cmd := exec.CommandContext(ctx, "webpmux", "-frame", in, first, "-frame", in, second, "-loop", "0", "-o", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("buat animasi stiker statis: %w: %s", err, bytes.TrimSpace(output))
	}
	data, err = os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	return BuildSticker(ctx, data, author, true, true)
}

func webpmuxInjectEXIF(ctx context.Context, input []byte, exif []byte) ([]byte, error) {
	if _, err := exec.LookPath("webpmux"); err != nil {
		return nil, errors.New("webpmux belum terpasang")
	}

	dir, err := os.MkdirTemp("", "stx")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	in := filepath.Join(dir, "in.webp")
	out := filepath.Join(dir, "out.webp")
	ex := filepath.Join(dir, "exif.bin")
	if err := os.WriteFile(in, input, 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(ex, exif, 0600); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "webpmux", "-set", "exif", ex, in, "-o", out)
	if cmdOut, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("webpmux timeout")
		}
		if len(cmdOut) > 0 {
			return nil, errors.New(string(bytes.TrimSpace(cmdOut)))
		}
		return nil, err
	}

	data, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty")
	}
	return data, nil
}

func injectWebPEXIF(ctx context.Context, input []byte, exif []byte) ([]byte, error) {
	out, err := webpmuxInjectEXIF(ctx, input, exif)
	if err == nil {
		return out, nil
	}

	riffOut, riffErr := webpRIFFInjectEXIF(input, exif)
	if riffErr == nil {
		return riffOut, nil
	}

	return nil, fmt.Errorf("%v (riff: %w)", err, riffErr)
}
