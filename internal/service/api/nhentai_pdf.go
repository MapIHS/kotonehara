package api

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"regexp"
)

var galleryIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// NhentaiPDF downloads to disk. The caller must close and remove the file.
func (c *Client) NhentaiPDF(ctx context.Context, id string) (*os.File, error) {
	if !galleryIDPattern.MatchString(id) {
		return nil, fmt.Errorf("kode gallery harus berupa angka positif")
	}
	endpoint, err := c.jobURL("/api/nsfw/nhentai/pdf/" + id)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/pdf")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, readAPIError("nhentai pdf", resp)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "application/pdf" {
		return nil, fmt.Errorf("respons Hararest bukan PDF")
	}
	if resp.ContentLength > maxMediaSize {
		return nil, fmt.Errorf("PDF melebihi batas 256 MiB")
	}
	file, err := os.CreateTemp("", "kotonehara-nhentai-*.pdf")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			file.Close()
			os.Remove(file.Name())
		}
	}()
	size, err := io.Copy(file, io.LimitReader(resp.Body, maxMediaSize+1))
	if err != nil {
		return nil, err
	}
	if size < 5 || size > maxMediaSize || (resp.ContentLength >= 0 && size != resp.ContentLength) {
		return nil, fmt.Errorf("ukuran PDF tidak valid atau unduhan tidak lengkap")
	}
	var header [5]byte
	if _, err := file.ReadAt(header[:], 0); err != nil {
		return nil, err
	}
	if string(header[:]) != "%PDF-" {
		return nil, fmt.Errorf("isi respons Hararest bukan PDF")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	ok = true
	return file, nil
}
