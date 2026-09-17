package bilibili

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/MapIHS/kotonehara/internal/service/api"
)

// Opt-in end-to-end download through Hararest; never connects to WhatsApp.
func TestBilibiliLive(t *testing.T) {
	baseURL := os.Getenv("HARAREST_LIVE_URL")
	if baseURL == "" {
		t.Skip("set HARAREST_LIVE_URL to test a real Bilibili download")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := api.New(baseURL, 90*time.Second)
	targetURL := os.Getenv("BILIBILI_LIVE_VIDEO_URL")
	if targetURL == "" {
		targetURL = "https://www.bilibili.com/video/BV1Ak8Q6hECg/"
	}
	result, err := client.Bilibili(ctx, targetURL, "1080p")
	if err != nil {
		t.Fatal(err)
	}
	selection, err := Select(result, "1080p")
	if err != nil {
		t.Fatal(err)
	}
	data, err := Prepare(ctx, client.HTTP, selection, result.Headers)
	if err != nil {
		t.Fatal(err)
	}
	output := os.Getenv("BILIBILI_LIVE_OUTPUT")
	if output == "" {
		output = filepath.Join(t.TempDir(), "bilibili.mp4")
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	probe, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name,width,height:format=duration", "-of", "json", output).Output()
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(probe, &info); err != nil {
		t.Fatal(err)
	}
	if len(info.Streams) != 2 || info.Streams[0].Codec != "h264" || info.Streams[1].Codec != "aac" {
		t.Fatalf("missing H.264/AAC: %s", probe)
	}
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || duration < result.Duration-2 || duration > result.Duration+2 {
		t.Fatalf("incomplete video: %s", probe)
	}
	if output, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-xerror", "-nostdin", "-i", output, "-map", "0:v:0", "-map", "0:a:0", "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("decode failed: %v %s", err, output)
	}
	t.Logf("downloaded %s: %dx%d, %.2fs, %d bytes; H.264 + AAC and full decode passed", selection.QualityLabel(), info.Streams[0].Width, info.Streams[0].Height, duration, len(data))
}
