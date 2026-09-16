// Package bilibili downloads Bilibili streams and prepares a video with audio.
package bilibili

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/service/api"
)

const maxMediaBytes int64 = 256 << 20

var slots = make(chan struct{}, 2)

var qualityCodes = map[string]int{
	"360p": 16, "480p": 32, "720p": 64, "720p60": 74,
	"1080p": 80, "1080p60": 116, "4k": 120,
}

func QualityCode(quality string) (int, bool) {
	code, ok := qualityCodes[quality]
	return code, ok
}

type Selection struct {
	Video    api.BilibiliMedia
	Audio    *api.BilibiliMedia
	Segments []api.BilibiliMedia
}

// Select prefers H.264/AAC for WhatsApp and never treats a silent DASH URL as MP4.
func Select(result *api.BilibiliResult, quality string) (Selection, error) {
	requested, ok := QualityCode(quality)
	if !ok || result == nil {
		return Selection{}, errors.New("kualitas atau data Bilibili tidak valid")
	}
	if result.Format == "progressive" {
		var segments []api.BilibiliMedia
		for _, item := range result.Media {
			if item.Type == "muxed" && item.URL != "" {
				segments = append(segments, item)
			}
		}
		sort.Slice(segments, func(i, j int) bool { return segments[i].Order < segments[j].Order })
		if len(segments) == 0 || len(segments) > 64 {
			return Selection{}, errors.New("segmen video Bilibili tidak tersedia atau terlalu banyak")
		}
		for i, segment := range segments {
			if segment.Order != i+1 {
				return Selection{}, errors.New("segmen video Bilibili tidak lengkap")
			}
		}
		return Selection{Video: segments[0], Segments: segments}, nil
	}
	if result.Format != "dash" {
		return Selection{}, errors.New("format video Bilibili tidak didukung")
	}
	var videos []api.BilibiliMedia
	var audio *api.BilibiliMedia
	for _, item := range result.Media {
		if item.URL == "" {
			continue
		}
		if item.Type == "video" && strings.HasPrefix(item.Codecs, "avc1") && item.Quality > 0 {
			videos = append(videos, item)
		}
		if item.Type == "audio" && strings.HasPrefix(item.Codecs, "mp4a") && (audio == nil || item.Bandwidth > audio.Bandwidth) {
			copy := item
			audio = &copy
		}
	}
	if len(videos) == 0 || audio == nil {
		return Selection{}, errors.New("video H.264 dan audio AAC Bilibili tidak tersedia lengkap")
	}
	sort.SliceStable(videos, func(i, j int) bool {
		if videos[i].Quality == videos[j].Quality {
			return videos[i].Bandwidth > videos[j].Bandwidth
		}
		return videos[i].Quality > videos[j].Quality
	})
	for _, video := range videos {
		if video.Quality <= requested {
			return Selection{Video: video, Audio: audio}, nil
		}
	}
	return Selection{}, errors.New("tidak ada kualitas Bilibili yang sesuai; coba kualitas lebih tinggi")
}

func (s Selection) QualityLabel() string {
	for label, code := range qualityCodes {
		if code == s.Video.Quality {
			return label
		}
	}
	if s.Video.Height > 0 {
		return fmt.Sprintf("%dp", s.Video.Height)
	}
	return s.Video.Label
}

func validMediaURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return errors.New("URL media Bilibili tidak valid")
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range []string{"bilivideo.com", "bilivideo.cn", "akamaized.net", "hdslb.com"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return nil
		}
	}
	return errors.New("domain media Bilibili tidak didukung")
}

// download writes to disk, tries backup CDNs, and bounds the combined input size.
func download(ctx context.Context, client *http.Client, item api.BilibiliMedia, headers map[string]string, filename string, remaining int64) (int64, error) {
	if remaining <= 0 {
		return 0, errors.New("media Bilibili melebihi batas 256 MiB")
	}
	candidates := append([]string{item.URL}, item.BackupURLs...)
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	var lastErr error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if err := validMediaURL(candidate); err != nil {
			lastErr = err
			continue
		}
		n, err := downloadOne(ctx, client, candidate, headers, filename, remaining)
		if err == nil {
			return n, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("gagal mengunduh %s Bilibili: %w", item.Type, lastErr)
}

func downloadOne(ctx context.Context, client *http.Client, source string, headers map[string]string, filename string, limit int64) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return 0, errors.New("URL media tidak valid")
	}
	for key, value := range headers {
		if strings.EqualFold(key, "Referer") || strings.EqualFold(key, "User-Agent") {
			req.Header.Set(key, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, errors.New("koneksi ke CDN gagal")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("CDN HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return 0, errors.New("media Bilibili melebihi batas 256 MiB")
	}
	file, err := os.Create(filename)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(file, io.LimitReader(resp.Body, limit+1))
	closeErr := file.Close()
	if copyErr != nil {
		return 0, errors.New("unduhan media Bilibili terputus")
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if n > limit {
		return 0, errors.New("media Bilibili melebihi batas 256 MiB")
	}
	if n == 0 || (resp.ContentLength >= 0 && n != resp.ContentLength) {
		return 0, errors.New("unduhan media Bilibili tidak lengkap")
	}
	return n, nil
}

// Prepare merges the selected tracks locally. Temporary files are always removed.
func Prepare(ctx context.Context, httpClient *http.Client, selection Selection, headers map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("FFmpeg belum terpasang di server bot")
	}
	dir, err := os.MkdirTemp("", "kotonehara-bilibili-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	client := *httpClient
	client.Timeout = 2 * time.Minute
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("terlalu banyak redirect media")
		}
		return validMediaURL(req.URL.String())
	}
	remaining := maxMediaBytes
	get := func(item api.BilibiliMedia, name string) error {
		n, err := download(ctx, &client, item, headers, filepath.Join(dir, name), remaining)
		remaining -= n
		return err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-n"}
	if len(selection.Segments) > 0 {
		var manifest strings.Builder
		for i, item := range selection.Segments {
			name := fmt.Sprintf("segment-%03d", i)
			if err := get(item, name); err != nil {
				return nil, err
			}
			fmt.Fprintf(&manifest, "file '%s'\n", name)
		}
		if err := os.WriteFile(filepath.Join(dir, "segments.txt"), []byte(manifest.String()), 0600); err != nil {
			return nil, err
		}
		args = append(args, "-protocol_whitelist", "file,crypto", "-f", "concat", "-safe", "1", "-i", filepath.Join(dir, "segments.txt"), "-map", "0:v:0", "-map", "0:a:0", "-c:v", "copy", "-c:a", "aac")
	} else {
		if selection.Audio == nil {
			return nil, errors.New("audio Bilibili tidak tersedia")
		}
		if err := get(selection.Video, "video.m4s"); err != nil {
			return nil, err
		}
		if err := get(*selection.Audio, "audio.m4s"); err != nil {
			return nil, err
		}
		args = append(args, "-protocol_whitelist", "file,crypto", "-i", filepath.Join(dir, "video.m4s"), "-protocol_whitelist", "file,crypto", "-i", filepath.Join(dir, "audio.m4s"), "-map", "0:v:0", "-map", "1:a:0", "-c", "copy")
	}
	output := filepath.Join(dir, "output.mp4")
	args = append(args, "-movflags", "+faststart", output)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("gagal menggabungkan video dan audio Bilibili")
	}
	file, err := os.Open(output)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMediaBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || int64(len(data)) > maxMediaBytes {
		return nil, errors.New("hasil video kosong atau melebihi batas 256 MiB")
	}
	return data, nil
}
