package sticker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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
