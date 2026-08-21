package handlers

import (
	"errors"
	"io"
	"testing"
)

type tracePreviewReader struct{ remaining int64 }

func (r *tracePreviewReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	r.remaining -= int64(len(p))
	return len(p), nil
}

func TestReadTracePreview(t *testing.T) {
	t.Run("accepts the exact limit", func(t *testing.T) {
		got, err := readTracePreview(&tracePreviewReader{remaining: maxTracePreviewBytes})
		if err != nil {
			t.Fatalf("readTracePreview() error = %v", err)
		}
		if len(got) != maxTracePreviewBytes {
			t.Fatalf("readTracePreview() returned %d bytes, want %d", len(got), maxTracePreviewBytes)
		}
	})

	t.Run("rejects content over the limit", func(t *testing.T) {
		_, err := readTracePreview(&tracePreviewReader{remaining: maxTracePreviewBytes + 1})
		if !errors.Is(err, errTracePreviewTooLarge) {
			t.Fatalf("readTracePreview() error = %v, want %v", err, errTracePreviewTooLarge)
		}
	})
}
