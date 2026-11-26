package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	exec "github.com/MapIHS/kotonehara/internal/service/mediaproc"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

func convertToVideo(ctx context.Context, data, stickerAudio []byte) ([]byte, bool, error) {
	dir, err := os.MkdirTemp("", "tovideo")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(dir)

	animatedWebP := sticker.IsAnimatedWebP(data)
	inFile := filepath.Join(dir, "input")
	if len(stickerAudio) > 0 && !animatedWebP {
		if http.DetectContentType(data) != "image/webp" {
			return nil, false, fmt.Errorf("stiker audio bukan WebP")
		}
		inFile = filepath.Join(dir, "still.webp")
	}
	outFile := filepath.Join(dir, "output.mp4")

	if err := os.WriteFile(inFile, data, 0600); err != nil {
		return nil, false, err
	}

	// FFmpeg can decode only the first frame of some animated WebP files.
	if animatedWebP {
		gifFile := filepath.Join(dir, "frames.gif")
		if err := exec.CommandContext(ctx, "magick", inFile, "-coalesce", gifFile).Run(); err != nil {
			if err := exec.CommandContext(ctx, "convert", inFile, "-coalesce", gifFile).Run(); err != nil {
				return nil, false, fmt.Errorf("decode animated WebP: %w", err)
			}
		}
		inFile = gifFile
	}

	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	filter := "pad=ceil(iw/2)*2:ceil(ih/2)*2"
	if len(stickerAudio) == 0 {
		args = append(args, "-i", inFile, "-map", "0:v:0", "-map", "0:a?")
	} else {
		audioFile := filepath.Join(dir, "audio")
		if err := os.WriteFile(audioFile, stickerAudio, 0600); err != nil {
			return nil, false, err
		}
		if animatedWebP {
			duration, err := sticker.AnimationDuration(data)
			if err != nil {
				return nil, false, err
			}
			gifDuration, err := exec.ProbeDuration(ctx, inFile)
			if err != nil {
				return nil, false, err
			}
			// GIF uses centisecond frame delays. Align the intermediate
			// animation's total duration with the original WebP timeline.
			args = append(args, "-itsscale", fmt.Sprintf("%.9f", float64(duration)/float64(gifDuration)),
				"-i", inFile, "-i", audioFile, "-map", "0:v:0", "-map", "1:a:0",
				"-af", "apad", "-t", fmt.Sprintf("%.3f", duration.Seconds()), "-fps_mode", "vfr")
			filter += ",tpad=stop_mode=clone:stop_duration=1"
		} else {
			args = append(args, "-f", "image2", "-loop", "1", "-framerate", "15", "-i", inFile,
				"-i", audioFile, "-map", "0:v:0", "-map", "1:a:0", "-shortest")
		}
	}
	args = append(args, "-vf", filter, "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart")
	var encodeErr error
	for _, encoder := range []string{"libx264", "libopenh264"} {
		out, err := exec.CommandContext(ctx, "ffmpeg", append(args, "-c:v", encoder, outFile)...).CombinedOutput()
		if err == nil {
			res, err := os.ReadFile(outFile)
			return res, animatedWebP && len(stickerAudio) == 0, err
		}
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		encodeErr = fmt.Errorf("encode MP4: %w: %s", err, out)
	}
	return nil, false, encodeErr
}

func downloadVideoStickerAudio(ctx context.Context, client *clients.Client, m *message.Message) ([]byte, error) {
	st, ok := m.Media.(*waE2E.StickerMessage)
	if !ok || st.GetAudioMessage() == nil {
		return nil, nil
	}
	audio := st.GetAudioMessage()
	if audio.GetFileLength() > maxStickerAudioSourceBytes {
		return nil, fmt.Errorf("audio stiker terlalu besar")
	}
	data, err := client.WA.Download(ctx, audio)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maxStickerAudioSourceBytes {
		return nil, fmt.Errorf("ukuran audio stiker invalid")
	}
	return data, nil
}

func init() {
	commands.Register(&commands.Command{
		Name:        "tovideo",
		As:          []string{"tomp4"},
		Tags:        "convert",
		Description: "Ubah media atau stiker bersuara menjadi video",
		IsPrefix:    true,
		IsMedia:     true,
		Exec: func(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
			opCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			data, err := client.DownloadMedia(opCtx, m)
			if err != nil || len(data) == 0 {
				m.Reply(ctx, "Media belum bisa diambil, yaa.")
				return
			}

			audio, err := downloadVideoStickerAudio(opCtx, client, m)
			if err != nil {
				m.Reply(ctx, "Audio stikernya belum bisa diambil, yaa.")
				return
			}
			vidData, isGif, err := convertToVideo(opCtx, data, audio)
			if err != nil {
				m.Reply(ctx, "Video belum bisa diproses, yaa.")
				return
			}
			if _, err := client.SendVideo(opCtx, m.From, vidData, isGif, getCaption(m), m.ID); err != nil {
				m.Reply(ctx, "Videonya belum bisa dikirim, yaa.")
				return
			}
		},
	})
}
