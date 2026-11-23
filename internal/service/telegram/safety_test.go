package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type failingTransport struct{ cause error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.cause }
func TestTransportErrorsNeverExposeToken(t *testing.T) {
	for _, cause := range []error{errors.New("failure FAKE_SECRET_TOKEN"), context.Canceled, context.DeadlineExceeded} {
		c := New("FAKE_SECRET_TOKEN", time.Second)
		c.http.Transport = failingTransport{cause}
		for _, call := range []func() error{
			func() error { _, e := c.GetStickerSet(context.Background(), "test"); return e },
			func() error { _, e := c.GetFile(context.Background(), "id"); return e },
			func() error { _, e := c.DownloadFile(context.Background(), "stickers/test.webp"); return e },
		} {
			err := call()
			if err == nil || strings.Contains(err.Error(), "FAKE_SECRET_TOKEN") || strings.Contains(err.Error(), "https://") {
				t.Fatalf("unsafe error: %v", err)
			}
			if (cause == context.Canceled || cause == context.DeadlineExceeded) && !errors.Is(err, cause) {
				t.Fatalf("context cause lost: %v", err)
			}
		}
	}
}
func TestTelegramBodySizeBoundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		size      int
		declared  int64
		wantLarge bool
	}{
		{"exact", maxTelegramFileSize, maxTelegramFileSize, false},
		{"declared oversize", 0, maxTelegramFileSize + 1, true},
		{"chunked oversize", maxTelegramFileSize + 1, -1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Response{ContentLength: tc.declared, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", tc.size)))}
			data, err := readTelegramBody(r)
			if tc.wantLarge {
				if !errors.Is(err, ErrFileTooLarge) || data != nil {
					t.Fatalf("accepted truncated body len=%d err=%v", len(data), err)
				}
			} else if err != nil || len(data) != tc.size {
				t.Fatalf("len=%d err=%v", len(data), err)
			}
		})
	}
}
