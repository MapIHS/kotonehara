package clients

import "fmt"

// PartialAlbumError reports attachments acknowledged by WhatsApp. The album
// container is excluded, so callers can distinguish partial delivery from failure.
type PartialAlbumError struct {
	Sent  int
	Total int
	Cause error
}

func (e *PartialAlbumError) Error() string {
	return fmt.Sprintf("%d dari %d media berhasil dikirim", e.Sent, e.Total)
}
func (e *PartialAlbumError) Unwrap() error { return e.Cause }

func albumResultError(err error, sent, total int) error {
	if err == nil || sent == 0 {
		return err
	}
	return &PartialAlbumError{Sent: sent, Total: total, Cause: err}
}

// AlbumHTTPError contains only the status code, never the source URL or body.
type AlbumHTTPError struct{ StatusCode int }

func (e *AlbumHTTPError) Error() string { return fmt.Sprintf("download media: HTTP %d", e.StatusCode) }
