package sticker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func webpmuxInjectEXIF(ctx context.Context, input []byte, exif []byte) ([]byte, error) {
	dir, _ := os.MkdirTemp("", "stx")
	in := filepath.Join(dir, "in.webp")
	out := filepath.Join(dir, "out.webp")
	ex := filepath.Join(dir, "exif.bin")
	if err := os.WriteFile(in, input, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(ex, exif, 0644); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "webpmux", "-set", "exif", ex, in, "-o", out)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("webpmux timeout")
	}

	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	data, err := os.ReadFile(out)
	os.RemoveAll(dir)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty")
	}
	return data, nil
}
