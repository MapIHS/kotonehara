package pkg

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/s3"
	"golang.org/x/net/proxy"
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

	timeout := 15 * time.Second

	httpClient := &http.Client{Timeout: timeout}

	if os.Getenv("TAILSCALE_SOCKS5") == "1" {
		if c, err := newSOCKS5HTTPClient(timeout, "127.0.0.1:1055"); err == nil {
			httpClient = c
		}
	}

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

	stc, err := sticker.BuildSticker(ctx, memeData, m.PushName, false, false)
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

func newSOCKS5HTTPClient(timeout time.Duration, socksAddr string) (*http.Client, error) {
	d, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return d.Dial(network, addr)
	}

	tr := &http.Transport{
		DialContext:           dialContext,
		ForceAttemptHTTP2:     false,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}, nil
}
