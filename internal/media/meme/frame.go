package meme

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// ExtractFirstFrame uses ffmpeg to extract the first frame from a video as PNG.
func ExtractFirstFrame(ctx context.Context, data []byte) ([]byte, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, errors.New("ffmpeg belum terpasang")
	}

	dir, err := os.MkdirTemp("", "memeframe")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	inFile := filepath.Join(dir, "input")
	outFile := filepath.Join(dir, "frame.png")

	if err := os.WriteFile(inFile, data, 0600); err != nil {
		return nil, err
	}

	magickBin := "magick"
	if _, err := exec.LookPath(magickBin); err != nil {
		magickBin = "convert"
	}
	if _, err := exec.LookPath(magickBin); err == nil {
		cmd := exec.CommandContext(ctx, magickBin, inFile+"[0]", outFile)
		if err := cmd.Run(); err == nil {
			if outData, err := os.ReadFile(outFile); err == nil {
				return outData, nil
			}
		}
	}

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-hwaccel", "auto",
		"-i", inFile,
		"-frames:v", "1",
		"-f", "image2",
		outFile,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(out) > 0 {
			return nil, errors.New(string(bytes.TrimSpace(out)))
		}
		return nil, err
	}

	return os.ReadFile(outFile)
}
