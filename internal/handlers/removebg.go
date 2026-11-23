package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

const maxRemoveBGAlbumImages = 10

func init() {
	commands.Register(&commands.Command{
		Name: "removebg", As: []string{"rbg", "rmbg", "hapusbg"}, Tags: "convert",
		Description: "Hapus background gambar atau foto dalam album", IsPrefix: true,
		IsMedia: true, AllowAlbum: true, ShowWait: true, Exec: removeBGCommand,
	})
}

func removeBGCommand(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if cfg.RemoveBGURL == "" {
		m.Reply(ctx, "Fitur remove background belum dikonfigurasi.")
		return
	}
	quality := strings.TrimSpace(m.Query)
	if quality == "" {
		quality = "fast"
	}
	process := func(ctx context.Context, media whatsmeow.DownloadableMessage) error {
		opCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
		if sized, ok := media.(interface{ GetFileLength() uint64 }); ok && sized.GetFileLength() > uint64(clients.MaxAlbumMediaBytes) {
			return clients.ErrAlbumLimit
		}
		raw, err := client.WA.Download(opCtx, media)
		if err != nil {
			return err
		}
		if len(raw) == 0 {
			return errors.New("gambar kosong")
		}
		if int64(len(raw)) > clients.MaxAlbumMediaBytes {
			return clients.ErrAlbumLimit
		}
		result, err := api.Shared(cfg.RemoveBGURL, 60*time.Second).RemoveBG(opCtx, raw, "image.png", quality)
		if err != nil {
			return err
		}
		imgData, err := fetchRemoveBGImage(opCtx, client.HTTP, cfg.RemoveBGURL, result.ResultURL)
		if err != nil {
			return err
		}
		_, err = client.SendImage(opCtx, m.From, imgData, "", m.ID)
		return err
	}
	if m.Media == nil && m.QuotedAlbumID != "" {
		items := client.AlbumMedia(m.From, m.QuotedAlbumID)
		if len(items) == 0 {
			m.Reply(ctx, "Foto di album ini belum tersimpan. Balas salah satu fotonya langsung, yaa.")
			return
		}
		batchCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		sent, total := removeBGAlbum(batchCtx, items, process)
		if total == 0 {
			m.Reply(ctx, "Album ini tidak berisi foto, yaa.")
		} else if sent == 0 {
			m.Reply(ctx, "Gagal menghapus background foto album ini, yaa.")
		} else if sent < total {
			m.Reply(ctx, fmt.Sprintf("%d dari %d foto berhasil diproses, yaa.", sent, total))
		}
		return
	}
	if m.Media == nil {
		m.Reply(ctx, "Gambarnya dibutuhkan, yaa.")
		return
	}
	if err := process(ctx, m.Media); err != nil {
		m.Reply(ctx, "Gagal menghapus background atau mengirim hasilnya, yaa.")
	}
}

// Keep only one source/result image in memory; failures do not stop later photos.
func removeBGAlbum(ctx context.Context, items []whatsmeow.DownloadableMessage, process func(context.Context, whatsmeow.DownloadableMessage) error) (sent, total int) {
	var photos []whatsmeow.DownloadableMessage
	for _, item := range items {
		if img, ok := item.(*waE2E.ImageMessage); ok && img != nil {
			photos = append(photos, img)
			if len(photos) == maxRemoveBGAlbumImages {
				break
			}
		}
	}
	total = len(photos)
	for _, photo := range photos {
		if ctx.Err() != nil {
			break
		}
		if err := process(ctx, photo); err == nil {
			sent++
		}
	}
	return
}

func fetchRemoveBGImage(ctx context.Context, httpClient *http.Client, base, resultPath string) ([]byte, error) {
	origin, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	target, err := origin.Parse(resultPath)
	if err != nil {
		return nil, err
	}
	if resultPath == "" || target.Scheme != origin.Scheme || target.Host != origin.Host || target.User != nil {
		return nil, errors.New("alamat hasil tidak valid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	// A result must stay on the configured service, including redirect responses.
	hc := *httpClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("gagal mengambil hasil remove background")
	}
	if resp.ContentLength > clients.MaxAlbumMediaBytes {
		return nil, clients.ErrAlbumLimit
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, clients.MaxAlbumMediaBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > clients.MaxAlbumMediaBytes {
		return nil, clients.ErrAlbumLimit
	}
	if len(data) == 0 {
		return nil, errors.New("hasil gambar kosong")
	}
	return data, nil
}
