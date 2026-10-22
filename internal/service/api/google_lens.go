package api

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const maxGoogleLensImageBytes = 10 << 20

type GoogleLensMatch struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
}

type GoogleLensResult struct {
	Mode          string            `json:"mode"`
	LensPage      string            `json:"lensPage"`
	AIResponse    string            `json:"aiResponse"`
	VisualMatches []GoogleLensMatch `json:"visualMatches"`
}

// GoogleLens submits an image to Hararest and polls until the search job finishes.
func (c *Client) GoogleLens(ctx context.Context, mode string, image []byte) (*GoogleLensResult, error) {
	if len(image) == 0 || len(image) > maxGoogleLensImageBytes {
		return nil, fmt.Errorf("gambar harus berukuran antara 1 byte dan 10 MiB")
	}
	if mode != "visual" && mode != "ai" {
		return nil, fmt.Errorf("mode Google Lens tidak valid")
	}
	contentType := http.DetectContentType(image)
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return nil, fmt.Errorf("Google Lens hanya mendukung JPEG, PNG, atau WebP")
	}

	endpoint, err := c.jobURL("/api/google-lens")
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(endpoint)
	query := u.Query()
	query.Set("mode", mode)
	u.RawQuery = query.Encode()
	endpoint = u.String()
	key, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	job, err := c.createGoogleLensJob(ctx, endpoint, key.String(), contentType, image)
	if err != nil {
		return nil, err
	}
	id := job.ID
	for {
		switch job.State {
		case "ready":
			if job.Result.Kind != "data" || job.Result.Data == nil || job.Result.Data.Mode != mode {
				return nil, fmt.Errorf("Hasil job Google Lens tidak valid")
			}
			return job.Result.Data, nil
		case "failed":
			if job.Error == "" {
				job.Error = "Google Lens gagal memproses gambar"
			}
			return nil, fmt.Errorf("Job Google Lens gagal: %s", job.Error)
		case "queued", "processing":
		default:
			return nil, fmt.Errorf("Status job Google Lens tidak valid: %q", job.State)
		}
		if err := waitJobDelay(ctx, pollDelay(job.PollAfterMs, jobPollInterval)); err != nil {
			return nil, err
		}
		job, err = c.requestJob(ctx, http.MethodGet, "/api/jobs/"+id, "", nil, jobPollInterval)
		if err != nil {
			return nil, err
		}
		if job.ID != id {
			return nil, fmt.Errorf("ID job Google Lens berubah saat polling")
		}
	}
}

func (c *Client) createGoogleLensJob(ctx context.Context, endpoint, key, contentType string, image []byte) (*mediaJob, error) {
	for attempt := 0; attempt < 3; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(image))
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Idempotency-Key", key)
		resp, requestErr := c.HTTP.Do(req)
		retry, retryDelay := false, jobPollInterval
		var result APIResponse[mediaJob]
		if requestErr != nil {
			err, retry = requestErr, true
		} else {
			retry = resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 || resp.StatusCode == 524
			if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
				retryDelay = min(time.Duration(seconds), 60) * time.Second
			}
			if resp.StatusCode != http.StatusAccepted {
				err = readAPIError("Google Lens", resp)
			} else {
				err = decodeAPIResponse(resp, &result)
			}
			resp.Body.Close()
		}
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil {
			if !jobIDPattern.MatchString(result.Data.ID) {
				return nil, fmt.Errorf("Respons job Google Lens tidak valid")
			}
			return &result.Data, nil
		}
		if !retry || attempt == 2 {
			return nil, err
		}
		if err := waitJobDelay(ctx, retryDelay+time.Duration(rand.Int64N(int64(time.Second)))); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("Job Google Lens gagal dibuat")
}
