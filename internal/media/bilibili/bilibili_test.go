package bilibili

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/service/api"
)

func fixtureResult() *api.BilibiliResult {
	return &api.BilibiliResult{Format: "dash", Media: []api.BilibiliMedia{
		{Type: "video", Quality: 120, Codecs: "avc1", URL: "https://cdn.bilivideo.com/4k"},
		{Type: "video", Quality: 80, Codecs: "hvc1", URL: "https://cdn.bilivideo.com/hevc"},
		{Type: "video", Quality: 64, Codecs: "avc1", URL: "https://cdn.bilivideo.com/video", Height: 720},
		{Type: "video", Quality: 32, Codecs: "avc1", URL: "https://cdn.bilivideo.com/480"},
		{Type: "audio", Codecs: "mp4a.40.2", URL: "https://cdn.bilivideo.com/audio-low", Bandwidth: 64000},
		{Type: "audio", Codecs: "mp4a.40.2", URL: "https://cdn.bilivideo.com/audio", Bandwidth: 192000},
	}}
}

func TestSelectCompatibleQualityAndAudio(t *testing.T) {
	selection, err := Select(fixtureResult(), "1080p")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Video.Quality != 64 || selection.Audio.Bandwidth != 192000 || selection.QualityLabel() != "720p" {
		t.Fatalf("bad selection: %+v", selection)
	}
	selection, err = Select(fixtureResult(), "480p")
	if err != nil || selection.Video.Quality != 32 {
		t.Fatalf("quality cap ignored: %+v %v", selection, err)
	}
	for _, result := range []*api.BilibiliResult{nil, {Format: "dash", Media: fixtureResult().Media[:4]}, {Format: "unknown"}} {
		if _, err := Select(result, "1080p"); err == nil {
			t.Fatal("missing audio/invalid format accepted")
		}
	}
}

func TestSelectProgressiveSegments(t *testing.T) {
	result := &api.BilibiliResult{Format: "progressive", Media: []api.BilibiliMedia{
		{Type: "muxed", URL: "https://cdn.bilivideo.com/2", Order: 2, Quality: 32},
		{Type: "muxed", URL: "https://cdn.bilivideo.com/1", Order: 1, Quality: 32},
	}}
	selection, err := Select(result, "1080p")
	if err != nil || selection.Segments[0].Order != 1 {
		t.Fatalf("bad order: %+v %v", selection, err)
	}
	result.Media[0].Order = 3
	if _, err := Select(result, "1080p"); err == nil {
		t.Fatal("missing segment accepted")
	}
}

func TestMediaURLValidation(t *testing.T) {
	for _, source := range []string{"https://cdn.bilivideo.com/a", "https://upos.akamaized.net/a", "http://a.bilivideo.cn/a"} {
		if err := validMediaURL(source); err != nil {
			t.Fatal(err)
		}
	}
	for _, source := range []string{"file:///etc/passwd", "http://127.0.0.1/a", "https://bilivideo.com.evil.test/a", "https://a.bilivideo.com:8080/a", "https://user:pass@a.bilivideo.com/a"} {
		if validMediaURL(source) == nil {
			t.Fatalf("unsafe source accepted: %s", source)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownloadLimitsAndCancellation(t *testing.T) {
	for _, length := range []int64{-1, 10} {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, ContentLength: length, Body: io.NopCloser(strings.NewReader("0123456789")), Header: http.Header{}}, nil
		})}
		_, err := download(context.Background(), client, fixtureResult().Media[0], nil, filepath.Join(t.TempDir(), "media"), 4)
		if err == nil || !strings.Contains(err.Error(), "batas") {
			t.Fatalf("size limit ignored: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := download(ctx, nil, fixtureResult().Media[0], nil, "unused", 4)
	if err != context.Canceled {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func TestPrepareVideoWithAudio(t *testing.T) {
	for _, binary := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skip(binary + " not installed")
		}
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if output, err := exec.Command("ffmpeg", append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("fixture: %v %s", err, output)
		}
	}
	videoPath, audioPath, muxedPath := filepath.Join(dir, "video.mp4"), filepath.Join(dir, "audio.m4a"), filepath.Join(dir, "muxed.mp4")
	run("-f", "lavfi", "-i", "color=c=blue:s=160x90:r=10:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", videoPath)
	run("-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "aac", audioPath)
	run("-i", videoPath, "-i", audioPath, "-map", "0:v", "-map", "1:a", "-c", "copy", muxedPath)
	video, _ := os.ReadFile(videoPath)
	audio, _ := os.ReadFile(audioPath)
	muxed, _ := os.ReadFile(muxedPath)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://www.bilibili.com/" || r.Header.Get("User-Agent") != "fixture-agent" || r.Header.Get("Cookie") != "" {
			t.Error("incorrect media headers")
		}
		switch r.URL.Path {
		case "/broken":
			w.WriteHeader(403)
		case "/video":
			w.Write(video)
		case "/audio":
			w.Write(audio)
		case "/muxed":
			w.Write(muxed)
		case "/private":
			http.Redirect(w, r, "http://127.0.0.1/private", http.StatusFound)
		default:
			w.Write([]byte("invalid media"))
		}
	}))
	defer server.Close()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		copy.URL.Scheme = "http"
		copy.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return http.DefaultTransport.RoundTrip(copy)
	})}
	workDir := t.TempDir()
	t.Setenv("TMPDIR", workDir)
	headers := map[string]string{"Referer": "https://www.bilibili.com/", "User-Agent": "fixture-agent", "Cookie": "must-not-forward"}
	selection, _ := Select(fixtureResult(), "1080p")
	selection.Video.URL = "https://cdn.bilivideo.com/broken"
	selection.Video.BackupURLs = []string{"https://cdn.bilivideo.com/video"}
	progressive := Selection{Segments: []api.BilibiliMedia{
		{Type: "muxed", URL: "https://cdn.bilivideo.com/muxed", Order: 1},
		{Type: "muxed", URL: "https://cdn.bilivideo.com/muxed", Order: 2},
	}}
	for name, choice := range map[string]Selection{"dash": selection, "progressive": progressive} {
		t.Run(name, func(t *testing.T) {
			data, err := Prepare(context.Background(), client, choice, headers)
			if err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, name+"-result.mp4")
			if err := os.WriteFile(output, data, 0600); err != nil {
				t.Fatal(err)
			}
			probe, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_name,codec_type", "-of", "json", output).Output()
			if err != nil {
				t.Fatal(err)
			}
			var info struct {
				Streams []struct {
					CodecName string `json:"codec_name"`
					CodecType string `json:"codec_type"`
				} `json:"streams"`
			}
			if err := json.Unmarshal(probe, &info); err != nil {
				t.Fatal(err)
			}
			if len(info.Streams) != 2 || info.Streams[0].CodecName != "h264" || info.Streams[1].CodecName != "aac" {
				t.Fatalf("missing video/audio: %s", probe)
			}
			pcm, err := exec.Command("ffmpeg", "-v", "error", "-i", output, "-map", "0:a:0", "-f", "s16le", "-").Output()
			if err != nil || len(pcm) == 0 || bytes.Equal(pcm, make([]byte, len(pcm))) {
				t.Fatal("output audio is silent or missing")
			}
		})
	}
	selection.Audio.URL = "https://cdn.bilivideo.com/invalid"
	if _, err := Prepare(context.Background(), client, selection, headers); err == nil {
		t.Fatal("broken audio accepted")
	}
	selection.Video.URL = "https://cdn.bilivideo.com/private"
	selection.Video.BackupURLs = nil
	if _, err := Prepare(context.Background(), client, selection, headers); err == nil {
		t.Fatal("private redirect accepted")
	}
	entries, err := os.ReadDir(workDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}

func ExampleSelection_QualityLabel() {
	fmt.Println((Selection{Video: api.BilibiliMedia{Quality: 80}}).QualityLabel())
	// Output: 1080p
}
