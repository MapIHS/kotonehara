package bilibili

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDownloadRetriesOnlyInterruptedRange(t *testing.T) {
	payload := bytes.Repeat([]byte("01234567"), int(downloadChunkSize/8)+27)
	var ranges []string
	broken := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Error("compressed range requested")
		}
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			t.Error(err)
			return
		}
		end = min(end, int64(len(payload))-1)
		if start > 0 && r.Header.Get("If-Range") != `"same-file"` {
			t.Error("missing If-Range")
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
		w.Header().Set("ETag", `"same-file"`)
		w.WriteHeader(http.StatusPartialContent)
		if start > 0 && !broken {
			broken = true
			w.Write(payload[start : start+10])
			return // net/http closes this incomplete response, yielding unexpected EOF.
		}
		w.Write(payload[start : end+1])
	}))
	defer server.Close()
	output := filepath.Join(t.TempDir(), "video")
	n, err := downloadOne(context.Background(), server.Client(), server.URL, nil, output, maxMediaBytes)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(output)
	if n != int64(len(payload)) || !bytes.Equal(data, payload) {
		t.Fatal("retried file is corrupt or incomplete")
	}
	want := []string{fmt.Sprintf("bytes=0-%d", downloadChunkSize-1), fmt.Sprintf("bytes=%d-%d", downloadChunkSize, len(payload)-1), fmt.Sprintf("bytes=%d-%d", downloadChunkSize, len(payload)-1)}
	if !reflect.DeepEqual(ranges, want) {
		t.Fatalf("completed chunk downloaded again: %v", ranges)
	}
}

type failedReader struct {
	io.Reader
	err error
}

func (r failedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		return n, r.err
	}
	return n, err
}

func TestDownloadRestartsAfterIgnoredRange(t *testing.T) {
	for _, resumeWithRange := range []bool{true, false} {
		t.Run(fmt.Sprint(resumeWithRange), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: 200, ContentLength: 20, Header: http.Header{}, Body: io.NopCloser(failedReader{strings.NewReader("unverified-prefix"), io.ErrUnexpectedEOF})}, nil
				}
				if resumeWithRange {
					return &http.Response{StatusCode: 206, ContentLength: 4, Header: http.Header{"Content-Range": []string{"bytes 0-3/4"}}, Body: io.NopCloser(strings.NewReader("good"))}, nil
				}
				return &http.Response{StatusCode: 200, ContentLength: 4, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("good"))}, nil
			})}
			output := filepath.Join(t.TempDir(), "video")
			n, err := downloadOne(context.Background(), client, "https://cdn.bilivideo.com/file", nil, output, 100)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(output)
			if n != 4 || string(data) != "good" || calls != 2 {
				t.Fatalf("unverified prefix retained: %q n=%d calls=%d", data, n, calls)
			}
		})
	}
}

func TestDownloadRejectsMalformedRanges(t *testing.T) {
	for _, value := range []string{"", "bytes 1-3/4", "bytes 0-9/4", "bytes 0-3/*", "bytes 0-3/9999999999999999999999", "bytes 0-3/300000000", "bytes 0-9/10suffix"} {
		t.Run(value, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 206, ContentLength: 4, Header: http.Header{"Content-Range": []string{value}}, Body: io.NopCloser(strings.NewReader("data"))}, nil
			})}
			_, err := downloadOne(context.Background(), client, "https://cdn.bilivideo.com/file", nil, filepath.Join(t.TempDir(), "video"), maxMediaBytes)
			if err == nil || calls != 1 {
				t.Fatalf("invalid range accepted/retried: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestDownloadRejectsChangedRepresentation(t *testing.T) {
	for _, changeETag := range []bool{true, false} {
		t.Run(fmt.Sprint(changeETag), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: 206, ContentLength: 4, Header: http.Header{"Content-Range": []string{"bytes 0-3/8"}, "Etag": []string{`"first"`}}, Body: io.NopCloser(strings.NewReader("data"))}, nil
				}
				etag, contentRange := `"second"`, "bytes 4-7/8"
				if !changeETag {
					etag, contentRange = `"first"`, "bytes 4-7/9"
				}
				return &http.Response{StatusCode: 206, ContentLength: 4, Header: http.Header{"Content-Range": []string{contentRange}, "Etag": []string{etag}}, Body: io.NopCloser(strings.NewReader("data"))}, nil
			})}
			_, err := downloadOne(context.Background(), client, "https://cdn.bilivideo.com/file", nil, filepath.Join(t.TempDir(), "video"), 100)
			if err == nil || calls != 2 {
				t.Fatalf("different representations combined: %d %v", calls, err)
			}
		})
	}
}

func TestDownloadCancellationAndTimeoutDiagnostics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { cancel(); return nil, context.Canceled })}
	_, err := downloadOne(ctx, client, "https://cdn.bilivideo.com/file?secret=hidden", nil, filepath.Join(t.TempDir(), "video"), 100)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	calls := 0
	client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, context.DeadlineExceeded })}
	_, err = downloadOne(context.Background(), client, "https://cdn.bilivideo.com/file?secret=hidden", nil, filepath.Join(t.TempDir(), "video"), 100)
	if err == nil || !strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "hidden") || calls != chunkAttempts {
		t.Fatalf("bad diagnostics/retries: %d %v", calls, err)
	}
}

func TestDownloadRetryHTTPStatus(t *testing.T) {
	for _, status := range []int{403, 404, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := downloadOne(ctx, client, "https://cdn.bilivideo.com/file", nil, filepath.Join(t.TempDir(), "video"), 100)
			want := 1
			if status == 429 || status == 503 {
				want = chunkAttempts
			}
			if err == nil || calls != want {
				t.Fatalf("calls=%d want=%d err=%v", calls, want, err)
			}
		})
	}
}
