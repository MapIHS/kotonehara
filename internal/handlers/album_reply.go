package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/message"
)

func replyAlbumFailure(ctx context.Context, m *message.Message, err error, fallback string) {
	if err == nil {
		return
	}
	var partial *clients.PartialAlbumError
	sent, total := 0, 0
	response := fallback
	if errors.As(err, &partial) && partial.Sent > 0 && partial.Total > partial.Sent {
		sent, total = partial.Sent, partial.Total
		response = fmt.Sprintf("%d dari %d media berhasil dikirim, yaa.", sent, total)
	}
	// A fixed reason and counters suffice for diagnostics without leaking signed
	// media URLs, cookies, source response bodies, or raw transport errors.
	log.Printf("album result: command=%q sent=%d requested=%d reason=%s", m.Command, sent, total, albumFailureReason(err))
	m.Reply(ctx, response)
}

func albumFailureReason(err error) string {
	var httpErr *clients.AlbumHTTPError
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, clients.ErrAlbumLimit):
		return "limit"
	case errors.As(err, &httpErr):
		return fmt.Sprintf("source_http_%d", httpErr.StatusCode)
	default:
		return "media_error"
	}
}
