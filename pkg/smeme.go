package pkg

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/httpclient"
	"github.com/MapIHS/kotonehara/internal/service/s3"
)

func cleanMemeText(text string) string {
	if text == "" {
		return "_"
	}
	text = strings.ReplaceAll(text, "_", "__")
	text = strings.ReplaceAll(text, "-", "--")
	text = strings.ReplaceAll(text, " ", "_")
	text = strings.ReplaceAll(text, "?", "~q")
	text = strings.ReplaceAll(text, "%", "~p")
	text = strings.ReplaceAll(text, "#", "~h")
	text = strings.ReplaceAll(text, "/", "~s")
	return text
}

func smeme(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if m.Media == nil {
		m.Reply(ctx, "Kirim atau balas gambar dulu, yaa.")
		return
	}

	if m.IsVideo || m.IsQuotedVideo {
		return
	}

	args := strings.Join(strings.Fields(m.Body)[1:], " ")
	parts := strings.Split(args, "|")

	topText := "_"
	bottomText := "_"

	if len(parts) >= 1 {
		topText = cleanMemeText(strings.TrimSpace(parts[0]))
	}
	if len(parts) >= 2 {
		bottomText = cleanMemeText(strings.TrimSpace(parts[1]))
	}

	raw, err := client.WA.Download(ctx, m.Media)
	if err != nil || len(raw) == 0 {
		m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
		return
	}

	ext := "png"

	ctInput := http.DetectContentType(raw)

	if ctInput == "image/webp" {
		if isAnimatedWebP(raw) {
			ext = "gif"
		} else {
			ext = "png"
		}
	} else if strings.HasPrefix(ctInput, "image/") {
		parts := strings.Split(ctInput, "/")
		if len(parts) == 2 {
			ext = parts[1]
		}
	}

	s3p := s3.New(cfg.BASES3URL, 15*time.Second)
	publicURL, err := s3p.Upload("temp."+ext, raw)
	if err != nil {
		m.Reply(ctx, "Gagal upload ke server sementara: "+err.Error())
		return
	}

	targetURL := fmt.Sprintf("%s/images/custom/%s/%s.%s?background=%s",
		cfg.MemeHost,
		topText,
		bottomText,
		ext,
		publicURL,
	)

	httpClient := httpclient.New("", 15*time.Second).HTTP

	resp, err := httpClient.Get(targetURL)
	if err != nil {
		m.Reply(ctx, "Gagal akses meme server: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		m.Reply(ctx, fmt.Sprintf("Meme server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(b))))
		return
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		m.Reply(ctx, "Response bukan gambar: "+ct+" "+strings.TrimSpace(string(b)))
		return
	}

	memeData, err := io.ReadAll(resp.Body)
	if err != nil || len(memeData) == 0 {
		m.Reply(ctx, "Gagal baca gambar dari meme server.")
		return
	}

	isGif := false
	isWebp := false

	switch ext {
	case "gif":
		isGif = true
	case "webp":
		isWebp = true
	}

	stc, err := sticker.BuildSticker(ctx, memeData, m.PushName, isWebp, isGif)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Ada yang salah: %s", err))
	}

	if _, err := client.SendSticker(ctx, m.From, stc, m.ID); err != nil {
		m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
		return
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:     "smeme",
		As:       []string{"stickermeme"},
		Tags:     "convert",
		IsPrefix: true,
		Exec:     smeme,
	})
}

func isAnimatedWebP(data []byte) bool {
	if len(data) < 16 {
		return false
	}

	// cek signature WEBP
	if string(data[8:12]) != "WEBP" {
		return false
	}

	// cari chunk ANIM
	return strings.Contains(string(data), "ANIM")
}
