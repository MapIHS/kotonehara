package bilibili

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/MapIHS/kotonehara/internal/service/api"
)

func TestFetchOpusImages(t *testing.T) {
	var pngData, gifData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	palette := color.Palette{color.Black, color.White}
	frames := []*image.Paletted{image.NewPaletted(image.Rect(0, 0, 2, 2), palette), image.NewPaletted(image.Rect(0, 0, 2, 2), palette)}
	frames[1].SetColorIndex(0, 0, 1)
	if err := gif.EncodeAll(&gifData, &gif.GIF{Image: frames, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	t.Setenv("TMPDIR", work)
	for _, tc := range []struct {
		name, kind string
		data       []byte
		limit      int64
		bad        bool
	}{
		{"png", "image", pngData.Bytes(), 1 << 20, false},
		{"animated gif", "gif", gifData.Bytes(), 1 << 20, false},
		{"html", "image", []byte("<html>blocked</html>"), 1 << 20, true},
		{"false gif", "gif", pngData.Bytes(), 1 << 20, true},
		{"oversize", "image", pngData.Bytes(), 10, true},
		{"video", "video", pngData.Bytes(), 1 << 20, true},
		{"no budget", "image", pngData.Bytes(), 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Referer") != "https://www.bilibili.com/" || r.Header.Get("Cookie") != "" {
					t.Error("invalid image headers")
				}
				return &http.Response{StatusCode: 200, ContentLength: int64(len(tc.data)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(tc.data))}, nil
			})}
			data, err := FetchImage(context.Background(), client, api.BilibiliMedia{Type: tc.kind, URL: "https://i0.hdslb.com/bfs/new_dyn/image"}, map[string]string{"Referer": "https://www.bilibili.com/", "Cookie": "must-not-forward"}, tc.limit)
			if tc.bad {
				if err == nil {
					t.Fatal("invalid image accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, tc.data) {
				t.Fatal("image bytes changed")
			}
			if tc.kind == "gif" {
				g, err := gif.DecodeAll(bytes.NewReader(data))
				if err != nil || len(g.Image) != 2 {
					t.Fatal("GIF animation lost")
				}
			}
		})
	}
	files, err := os.ReadDir(work)
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary files leaked: %v %v", files, err)
	}
}
