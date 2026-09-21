package s3

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

const maxUploadResponseSize = 1 << 20

var errUploadResponseTooLarge = errors.New("response body melebihi batas")

type Response struct {
	Key         string      `json:"key"`
	ContentType string      `json:"content_type,omitempty"`
	UploadURL   string      `json:"upload_url"`
	Method      string      `json:"method"`
	Headers     http.Header `json:"headers"`
}

func (c *Client) Upload(filename string, file []byte) (string, error) {
	contentType := http.DetectContentType(file)
	body, err := json.Marshal(struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int    `json:"size"`
	}{filepath.Base(filename), contentType, len(file)})
	if err != nil {
		return "", err
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	req, err := http.NewRequest(http.MethodPost, baseURL+"/upload/presign", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := readUploadResponse(resp)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upload failed: %s", string(respBody))
	}

	var result APIResponse[Response]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse json: %w", err)

	}
	if result.Data == nil || result.Data.Key == "" {
		if result.Error != "" {
			return "", fmt.Errorf("upload failed: %s", result.Error)
		}
		return "", fmt.Errorf("upload failed: response key kosong")
	}
	data := result.Data
	if data.UploadURL == "" || data.Method != http.MethodPut {
		return "", fmt.Errorf("invalid presigned upload response")
	}
	put, err := http.NewRequest(http.MethodPut, data.UploadURL, bytes.NewReader(file))
	if err != nil {
		return "", fmt.Errorf("invalid upload URL: %w", err)
	}
	if put.URL.Host == "" || (put.URL.Scheme != "https" && put.URL.Scheme != "http") {
		return "", fmt.Errorf("invalid upload URL")
	}
	put.Header = data.Headers.Clone()
	if put.Header == nil {
		put.Header = make(http.Header)
	}
	if length := put.Header.Get("Content-Length"); length != "" && length != strconv.Itoa(len(file)) {
		return "", fmt.Errorf("presigned content length does not match file size")
	}
	put.Header.Del("Content-Length")
	put.ContentLength = int64(len(file))
	if put.Header.Get("Content-Type") == "" {
		put.Header.Set("Content-Type", contentType)
	}
	// Do not follow storage redirects: the URL and headers are signed for one destination.
	uploadClient := *c.HTTP
	uploadClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	stored, err := uploadClient.Do(put)
	if err != nil {
		return "", fmt.Errorf("direct upload failed: %w", err)
	}
	defer stored.Body.Close()
	if stored.StatusCode < 200 || stored.StatusCode >= 300 {
		return "", fmt.Errorf("direct upload failed: HTTP %d", stored.StatusCode)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(stored.Body, maxUploadResponseSize))
	return fmt.Sprintf("%s/file/%s", baseURL, data.Key), nil
}

func readUploadResponse(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > maxUploadResponseSize {
		return nil, fmt.Errorf("%w %d byte", errUploadResponseTooLarge, maxUploadResponseSize)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUploadResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxUploadResponseSize {
		return nil, fmt.Errorf("%w %d byte", errUploadResponseTooLarge, maxUploadResponseSize)
	}
	return body, nil
}
