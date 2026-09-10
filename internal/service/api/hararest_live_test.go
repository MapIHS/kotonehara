package api

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Opt-in HTTP smoke test. It calls Hararest only and never connects to WhatsApp.
// Run with HARAREST_LIVE_URL=http://host:8080 go test ./internal/service/api -run '^TestHararestLive$' -v -count=1.
func TestHararestLive(t *testing.T) {
	baseURL := os.Getenv("HARAREST_LIVE_URL")
	if baseURL == "" {
		t.Skip("set HARAREST_LIVE_URL to run live Hararest requests")
	}
	client := New(baseURL, 2*time.Minute)
	const videoURL = "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	t.Run("YouTubeInfo", func(t *testing.T) {
		info, err := client.YoutubeInfo(context.Background(), videoURL)
		if err != nil {
			t.Fatal(err)
		}
		if info.ID != "jNQXAC9IVRw" || len(info.Videos) == 0 {
			t.Fatal("missing video metadata")
		}
		t.Logf("duration=%.3fs qualities=%d", info.Duration, len(info.Videos))
	})
	t.Run("YouTubeAudio", func(t *testing.T) {
		data, err := client.YoutubeDownload(context.Background(), videoURL, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 1024 {
			t.Fatalf("audio too small: %d bytes", len(data))
		}
		t.Logf("audio bytes=%d", len(data))
	})
	t.Run("YouTubeVideo", func(t *testing.T) {
		data, err := client.YoutubeDownload(context.Background(), videoURL, "360p", true)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 12 || string(data[4:8]) != "ftyp" {
			t.Fatal("response is not an MP4 file")
		}
		t.Logf("video bytes=%d", len(data))
	})
	t.Run("YouTubeSearch", func(t *testing.T) {
		results, err := client.YoutubeSearch(context.Background(), "Me at the zoo", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].URL == "" {
			t.Fatal("missing search result")
		}
	})
	t.Run("PinterestSearch", func(t *testing.T) {
		results, err := client.PinterestSearch(context.Background(), "mountain landscape")
		if err != nil {
			t.Fatal(err)
		}
		if len(results.Results) == 0 || len(results.Results[0].Images) == 0 {
			t.Fatal("missing pins")
		}
		t.Logf("pins=%d", len(results.Results))
	})
	t.Run("PixivSearch", func(t *testing.T) {
		results, err := client.PixivSearch(context.Background(), "landscape")
		if err != nil {
			t.Fatal(err)
		}
		if len(results.Results) == 0 || results.Results[0].ID == "" {
			t.Fatal("missing artwork results")
		}
		t.Logf("artworks=%d", len(results.Results))
	})
	t.Run("WaifuSFW", func(t *testing.T) {
		result, err := client.WaifuIm(context.Background(), "waifu", false)
		if err != nil {
			t.Fatal(err)
		}
		if result.URL == "" || result.Width == 0 {
			t.Fatal("missing image metadata")
		}
	})
	t.Run("OCR", func(t *testing.T) {
		parsed, err := opentype.Parse(goregular.TTF)
		if err != nil {
			t.Fatal(err)
		}
		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 48, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			t.Fatal(err)
		}
		defer face.Close()
		large := image.NewRGBA(image.Rect(0, 0, 900, 120))
		draw.Draw(large, large.Bounds(), image.White, image.Point{}, draw.Src)
		drawer := font.Drawer{Dst: large, Src: image.Black, Face: face, Dot: fixed.P(30, 80)}
		drawer.DrawString("HARAREST OCR TEST 12345")
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, large); err != nil {
			t.Fatal(err)
		}
		text, err := client.ExtractOCR(context.Background(), buffer.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "12345") {
			t.Fatalf("unexpected fixture OCR text: %q", text)
		}
		t.Logf("OCR=%q", text)
	})
}
