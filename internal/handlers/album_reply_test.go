package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
)

func TestAlbumRepliesDescribePartialDeliveryAndKeepDiagnosticsPrivate(t *testing.T) {
	for _, tc := range []struct {
		name         string
		err          error
		want, reason string
	}{
		{"four Pinterest photos", fmt.Errorf("handler: %w", &clients.PartialAlbumError{Sent: 4, Total: 5, Cause: &clients.AlbumHTTPError{StatusCode: 403}}), "4 dari 5 media berhasil dikirim, yaa.", "source_http_403"},
		{"one surviving photo", &clients.PartialAlbumError{Sent: 1, Total: 5, Cause: errors.New("https://private.example/image?token=secret-value")}, "1 dari 5 media berhasil dikirim, yaa.", "media_error"},
		{"cancelled after one", &clients.PartialAlbumError{Sent: 1, Total: 2, Cause: context.Canceled}, "1 dari 2 media berhasil dikirim, yaa.", "cancelled"},
		{"nothing delivered", &clients.AlbumHTTPError{StatusCode: 404}, "Gagal mengirim gambar dari hasil pencarian Pinterest.", "source_http_404"},
		{"all succeeded", nil, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diagnostics bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&diagnostics)
			defer log.SetOutput(previous)
			var replies []string
			m := &message.Message{Command: "pinsearch", Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				replies = append(replies, text)
				return whatsmeow.SendResponse{}, nil
			}}
			replyAlbumFailure(context.Background(), m, tc.err, "Gagal mengirim gambar dari hasil pencarian Pinterest.")
			if tc.want == "" {
				if len(replies) != 0 || diagnostics.Len() != 0 {
					t.Fatal("success produced a failure reply")
				}
				return
			}
			if len(replies) != 1 || replies[0] != tc.want {
				t.Fatalf("replies=%v", replies)
			}
			if !strings.Contains(diagnostics.String(), "reason="+tc.reason) {
				t.Fatalf("diagnostics=%s", diagnostics.String())
			}
			if strings.Contains(diagnostics.String(), "private.example") || strings.Contains(diagnostics.String(), "secret-value") {
				t.Fatal("source credentials leaked")
			}
		})
	}
}
