package bilibili

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

// Opt-in downloads and GIF conversion; never connects to WhatsApp.
func TestBilibiliOpusLive(t *testing.T) {
	base := os.Getenv("HARAREST_LIVE_URL")
	if base == "" {
		t.Skip("set HARAREST_LIVE_URL to test real Bilibili Opus downloads")
	}
	dir := os.Getenv("BILIBILI_OPUS_OUTPUT_DIR")
	if dir == "" {
		dir = t.TempDir()
	} else if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	client := api.New(base, 90*time.Second)
	for _, tc := range []struct {
		id       string
		count    int
		animated bool
	}{
		{"1000310312918843446", 1, true},
		{"1247969693541597185", 3, false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			result, err := client.Bilibili(ctx, "https://www.bilibili.com/opus/"+tc.id, "")
			if err != nil {
				t.Fatal(err)
			}
			if result.ID != tc.id || result.Format != "images" || len(result.Media) != tc.count {
				t.Fatalf("unexpected post: id=%s format=%s count=%d", result.ID, result.Format, len(result.Media))
			}
			remaining := int64(256 << 20)
			for i, item := range result.Media {
				data, err := FetchImage(ctx, client.HTTP, item, result.Headers, remaining)
				if err != nil {
					t.Fatal(err)
				}
				remaining -= int64(len(data))
				config, format, err := image.DecodeConfig(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				if config.Width != item.Width || config.Height != item.Height {
					t.Fatalf("dimensions differ: downloaded=%dx%d API=%dx%d", config.Width, config.Height, item.Width, item.Height)
				}
				filename := filepath.Join(dir, fmt.Sprintf("%s-%02d.%s", tc.id, i+1, format))
				if err := os.WriteFile(filename, data, 0600); err != nil {
					t.Fatal(err)
				}
				if tc.animated {
					animation, err := gif.DecodeAll(bytes.NewReader(data))
					if err != nil {
						t.Fatal(err)
					}
					if len(animation.Image) < 2 {
						t.Fatal("GIF is not animated")
					}
					mp4, err := (&clients.Client{}).ConvertGifToMP4(ctx, data)
					if err != nil {
						t.Fatal(err)
					}
					converted := filepath.Join(dir, tc.id+"-whatsapp.mp4")
					if err := os.WriteFile(converted, mp4, 0600); err != nil {
						t.Fatal(err)
					}
					probe, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-count_frames", "-select_streams", "v:0", "-show_entries", "stream=nb_read_frames", "-of", "csv=p=0", converted).Output()
					if err != nil {
						t.Fatal(err)
					}
					frames, err := strconv.Atoi(strings.TrimSpace(string(probe)))
					if err != nil || frames < 2 {
						t.Fatalf("converted GIF is not animated: %s", probe)
					}
					t.Logf("animation: original %d frames; WhatsApp MP4 %d frames", len(animation.Image), frames)
				}
				t.Logf("downloaded %s: %dx%d, %d bytes", filename, config.Width, config.Height, len(data))
			}
		})
	}
}
