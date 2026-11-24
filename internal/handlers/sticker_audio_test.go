package handlers

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractOpusAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg tidak tersedia")
	}

	for _, format := range []string{"mp4", "webm"} {
		t.Run(format, func(t *testing.T) {
			video := makeStickerAudioTestVideo(t, format, true)
			audio, err := extractOpusAudio(context.Background(), video)
			if err != nil {
				t.Fatalf("extractOpusAudio() error = %v", err)
			}
			if len(audio) < 4 || string(audio[:4]) != "OggS" {
				t.Fatalf("hasil bukan OGG Opus: %d byte", len(audio))
			}
		})
	}
}

func TestExtractOpusAudioRejectsVideoWithoutAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg tidak tersedia")
	}

	for _, format := range []string{"mp4", "webm"} {
		t.Run(format, func(t *testing.T) {
			video := makeStickerAudioTestVideo(t, format, false)
			if _, err := extractOpusAudio(context.Background(), video); !errors.Is(err, errStickerAudioStream) {
				t.Fatalf("extractOpusAudio() error = %v, want errStickerAudioStream", err)
			}
		})
	}
}

func TestExtractOpusAudioRejectsEmptyMedia(t *testing.T) {
	if _, err := extractOpusAudio(context.Background(), nil); err == nil {
		t.Fatal("extractOpusAudio() menerima media kosong")
	}
}

func makeStickerAudioTestVideo(t *testing.T, format string, withAudio bool) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "input."+format)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=128x128:rate=10:duration=1",
	}
	if withAudio {
		args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:duration=1")
	}
	args = append(args, "-map", "0:v:0")
	if withAudio {
		args = append(args, "-map", "1:a:0", "-shortest")
	} else {
		args = append(args, "-an")
	}
	if format == "webm" {
		args = append(args, "-c:v", "libvpx-vp9")
		if withAudio {
			args = append(args, "-c:a", "libopus")
		}
	} else {
		args = append(args, "-c:v", "libx264")
		if withAudio {
			args = append(args, "-c:a", "aac")
		}
	}
	args = append(args, "-pix_fmt", "yuv420p", path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("membuat video test: %v: %s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
