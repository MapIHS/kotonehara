package bilibili

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const downloadChunkSize int64 = 4 << 20
const chunkAttempts = 3

var contentRangePattern = regexp.MustCompile(`^bytes ([0-9]+)-([0-9]+)/([0-9]+)$`)

type transferError struct {
	message string
	retry   bool
}

func (e *transferError) Error() string { return e.message }

func interruptedTransfer(err error, received, expected int64) error {
	var diskError *os.PathError
	if errors.As(err, &diskError) {
		return errors.New("gagal menulis file media sementara")
	}
	reason := "koneksi CDN terputus"
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		reason = "timeout koneksi CDN"
	}
	return &transferError{message: fmt.Sprintf("%s (%d dari %d byte bagian diterima)", reason, received, expected), retry: true}
}

type transferState struct {
	offset int64
	total  int64
	etag   string
}

// Keep completed chunks on disk, retry only the current chunk, and start over
// on a different CDN. Never splice unvalidated ranges or two representations.
func downloadOne(ctx context.Context, client *http.Client, source string, headers map[string]string, filename string, limit int64) (int64, error) {
	file, err := os.Create(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	state := transferState{total: -1}
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		done, err := readChunk(ctx, client, source, headers, file, &state, limit)
		if err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			attempt++
			var transient *transferError
			if !errors.As(err, &transient) || !transient.retry || attempt >= chunkAttempts {
				return 0, fmt.Errorf("setelah %d byte tersimpan: %w", state.offset, err)
			}
			timer := time.NewTimer(time.Duration(attempt) * 200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return 0, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		attempt = 0
		if done {
			if err := file.Close(); err != nil {
				return 0, err
			}
			return state.offset, nil
		}
	}
}

func readChunk(ctx context.Context, client *http.Client, source string, headers map[string]string, file *os.File, state *transferState, limit int64) (bool, error) {
	if state.offset == 0 {
		// A failed full-body response may have left unverified bytes on disk.
		if err := file.Truncate(0); err != nil {
			return false, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return false, err
		}
	}
	end := min(state.offset+downloadChunkSize, limit) - 1
	if state.total >= 0 {
		end = min(end, state.total-1)
	}
	if end < state.offset {
		return false, fmt.Errorf("media Bilibili melebihi batas %d byte", limit)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return false, errors.New("URL media tidak valid")
	}
	for key, value := range headers {
		if strings.EqualFold(key, "Referer") || strings.EqualFold(key, "User-Agent") {
			req.Header.Set(key, value)
		}
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", state.offset, end))
	req.Header.Set("Accept-Encoding", "identity")
	if state.offset > 0 && state.etag != "" {
		req.Header.Set("If-Range", state.etag)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, interruptedTransfer(err, 0, end-state.offset+1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return false, &transferError{message: fmt.Sprintf("CDN HTTP %d", resp.StatusCode), retry: resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500}
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return false, errors.New("encoding respons CDN tidak didukung")
	}
	if resp.StatusCode == http.StatusOK {
		// Servers may ignore Range, or If-Range may select a changed full file.
		// Replace the prefix rather than appending a second copy of the video.
		if resp.ContentLength > limit {
			return false, fmt.Errorf("media Bilibili melebihi batas %d byte", limit)
		}
		if err := file.Truncate(0); err != nil {
			return false, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return false, err
		}
		*state = transferState{total: -1}
		n, err := io.Copy(file, io.LimitReader(resp.Body, limit+1))
		if n > limit {
			return false, fmt.Errorf("media Bilibili melebihi batas %d byte", limit)
		}
		if err != nil {
			return false, interruptedTransfer(err, n, resp.ContentLength)
		}
		if n == 0 || (resp.ContentLength >= 0 && n != resp.ContentLength) {
			return false, interruptedTransfer(io.ErrUnexpectedEOF, n, resp.ContentLength)
		}
		state.offset = n
		return true, nil
	}
	parts := contentRangePattern.FindStringSubmatch(resp.Header.Get("Content-Range"))
	if len(parts) != 4 {
		return false, errors.New("Content-Range CDN tidak valid")
	}
	values := make([]int64, 3)
	for i := range values {
		value, err := strconv.ParseInt(parts[i+1], 10, 64)
		if err != nil {
			return false, errors.New("Content-Range CDN tidak valid")
		}
		values[i] = value
	}
	start, last, total := values[0], values[1], values[2]
	if total > limit {
		return false, fmt.Errorf("media Bilibili melebihi batas %d byte", limit)
	}
	if start != state.offset || last < start || last > end || last >= total || (state.total >= 0 && state.total != total) {
		return false, errors.New("rentang atau ukuran media CDN berubah")
	}
	etag := resp.Header.Get("ETag")
	if state.etag != "" && etag != "" && state.etag != etag {
		return false, errors.New("versi media CDN berubah")
	}
	want := last - start + 1
	if resp.ContentLength >= 0 && resp.ContentLength != want {
		return false, errors.New("ukuran bagian CDN tidak cocok dengan Content-Range")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, want+1))
	if err != nil {
		return false, interruptedTransfer(err, int64(len(body)), want)
	}
	if int64(len(body)) < want {
		return false, interruptedTransfer(io.ErrUnexpectedEOF, int64(len(body)), want)
	}
	if int64(len(body)) > want {
		return false, errors.New("respons CDN melebihi rentang yang diminta")
	}
	if _, err := file.Write(body); err != nil {
		return false, err
	}
	state.offset += want
	state.total = total
	if state.etag == "" && strings.HasPrefix(etag, `"`) && strings.HasSuffix(etag, `"`) {
		state.etag = etag
	}
	return state.offset == total, nil
}
