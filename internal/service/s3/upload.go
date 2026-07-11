package s3

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

const maxUploadResponseSize = 1 << 20

var errUploadResponseTooLarge = errors.New("response body melebihi batas")

type Response struct {
	Key         string `json:"key"`
	ContentType string `json:"content_type,omitempty"`
}

func (c *Client) Upload(filename string, file []byte) (string, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}

	if _, err := part.Write(file); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/upload", body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

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
	return fmt.Sprintf("%s/file/%s", c.BaseURL, result.Data.Key), nil
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
