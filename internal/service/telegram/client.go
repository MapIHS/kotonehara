package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/service/httpclient"
)

var (
	ErrEmptyToken         = errors.New("telegram bot token is empty")
	ErrStickerSetNotFound = errors.New("sticker pack telegram tidak ditemukan")
	ErrStickerSetEmpty    = errors.New("sticker pack telegram kosong")
	ErrDownloadFailed     = errors.New("gagal mengunduh file telegram")
)

const (
	defaultBaseURL      = "https://api.telegram.org"
	maxTelegramFileSize = 20 << 20 // 20 MB limit
)

type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

func New(token string, timeout time.Duration) *Client {
	return NewWithBaseURL(token, defaultBaseURL, timeout)
}

func NewWithBaseURL(token, baseURL string, timeout time.Duration) *Client {
	hc := httpclient.New("", timeout).HTTP
	return &Client{
		token:   strings.TrimSpace(token),
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    hc,
	}
}

var packURLRegex = regexp.MustCompile(`(?:https?://)?(?:t\.me|telegram\.me)/addstickers/([a-zA-Z0-9_]+)`)
var packNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ExtractPackName extracts the Telegram sticker set name from a URL or raw pack name.
func ExtractPackName(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if m := packURLRegex.FindStringSubmatch(input); len(m) > 1 {
		return m[1]
	}
	if packNameRegex.MatchString(input) {
		return input
	}
	return ""
}

// IsTelegramStickerURL checks whether an input contains a Telegram sticker set URL.
func IsTelegramStickerURL(input string) bool {
	return packURLRegex.MatchString(strings.TrimSpace(input))
}

func (c *Client) GetStickerSet(ctx context.Context, name string) (*StickerSet, error) {
	if c.token == "" {
		return nil, ErrEmptyToken
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("sticker pack name is empty")
	}

	endpoint := fmt.Sprintf("%s/bot%s/getStickerSet?name=%s", c.baseURL, c.token, url.QueryEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getStickerSet request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTelegramFileSize))
	if err != nil {
		return nil, fmt.Errorf("reading getStickerSet body failed: %w", err)
	}

	var res StickerSetResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("unmarshal getStickerSet response failed: %w", err)
	}

	if !res.OK || res.Result == nil {
		if res.Description != "" {
			return nil, fmt.Errorf("%w: %s", ErrStickerSetNotFound, res.Description)
		}
		return nil, ErrStickerSetNotFound
	}

	if len(res.Result.Stickers) == 0 {
		return nil, ErrStickerSetEmpty
	}

	return res.Result, nil
}

func (c *Client) GetFile(ctx context.Context, fileID string) (*FileInfo, error) {
	if c.token == "" {
		return nil, ErrEmptyToken
	}
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, errors.New("file_id is empty")
	}

	endpoint := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", c.baseURL, c.token, url.QueryEscape(fileID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getFile request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTelegramFileSize))
	if err != nil {
		return nil, fmt.Errorf("reading getFile body failed: %w", err)
	}

	var res GetFileResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("unmarshal getFile response failed: %w", err)
	}

	if !res.OK || res.Result == nil || res.Result.FilePath == "" {
		if res.Description != "" {
			return nil, fmt.Errorf("telegram getFile error: %s", res.Description)
		}
		return nil, errors.New("telegram getFile returned empty file_path")
	}

	return res.Result, nil
}

func (c *Client) DownloadFile(ctx context.Context, filePath string) ([]byte, error) {
	if c.token == "" {
		return nil, ErrEmptyToken
	}
	filePath = strings.TrimPrefix(strings.TrimSpace(filePath), "/")
	if filePath == "" {
		return nil, errors.New("file_path is empty")
	}

	endpoint := fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, c.token, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download file request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrDownloadFailed, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTelegramFileSize))
	if err != nil {
		return nil, fmt.Errorf("reading file body failed: %w", err)
	}

	return data, nil
}

func (c *Client) DownloadSticker(ctx context.Context, s Sticker) ([]byte, error) {
	fileID := s.FileID
	if fileID == "" && s.Thumbnail != nil {
		fileID = s.Thumbnail.FileID
	}
	if fileID == "" && s.Thumb != nil {
		fileID = s.Thumb.FileID
	}
	if fileID == "" {
		return nil, errors.New("sticker has no valid file_id")
	}

	info, err := c.GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}

	return c.DownloadFile(ctx, info.FilePath)
}
