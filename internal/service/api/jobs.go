package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const jobPollInterval = 3 * time.Second

var jobIDPattern = regexp.MustCompile(`^[a-f0-9]{48}$`)

type mediaJob struct {
	PollAfterMs int64  `json:"pollAfterMs"`
	ID          string `json:"id"`
	State       string `json:"state"`
	Error       string `json:"error"`
	Result      struct {
		Kind     string         `json:"kind"`
		Size     int64          `json:"size"`
		MimeType string         `json:"mimeType"`
		Player   *PlayerSession `json:"player"`
	} `json:"result"`
}

func (c *Client) jobURL(path string) (string, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("BASEAPI_URL Hararest belum valid")
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment = path, "", "", ""
	return u.String(), nil
}

func waitJobDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Every retry of a POST uses the same idempotency key, including when the first
// response was lost after the server accepted the job. Never retry via the old
// synchronous endpoint: that could start another download and hit 524 again.
func (c *Client) requestJob(ctx context.Context, method, path, key string, body []byte, delay time.Duration) (*mediaJob, error) {
	endpoint, err := c.jobURL(path)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		req, err := http.NewRequestWithContext(requestCtx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", key)
		}
		resp, requestErr := c.HTTP.Do(req)
		retry, retryDelay := false, delay
		var result APIResponse[mediaJob]
		if requestErr != nil {
			err = requestErr
			retry = true
		} else {
			retry = resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 || resp.StatusCode == 524
			if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
				retryDelay = min(time.Duration(seconds), 60) * time.Second
			}
			expectedStatus := http.StatusOK
			if method == http.MethodPost {
				expectedStatus = http.StatusAccepted
			}
			if resp.StatusCode != expectedStatus {
				err = readAPIError("job", resp)
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
				return nil, fmt.Errorf("Respons job Hararest tidak valid")
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
	return nil, fmt.Errorf("Job Hararest gagal dibuat")
}

func (c *Client) runMediaJob(ctx context.Context, endpoint string, input any, interval time.Duration) (*mediaJob, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	key, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	job, err := c.requestJob(ctx, http.MethodPost, endpoint, key.String(), body, interval)
	if err != nil {
		return nil, err
	}
	id := job.ID
	for {
		switch job.State {
		case "ready":
			return job, nil
		case "failed":
			if job.Error == "" {
				job.Error = "Penyiapan media gagal"
			}
			return nil, fmt.Errorf("Job Hararest gagal: %s", job.Error)
		case "queued", "processing":
		default:
			return nil, fmt.Errorf("Status job Hararest tidak valid: %q", job.State)
		}
		if err := waitJobDelay(ctx, pollDelay(job.PollAfterMs, interval)); err != nil {
			return nil, err
		}
		job, err = c.requestJob(ctx, http.MethodGet, "/api/jobs/"+id, "", nil, interval)
		if err != nil {
			return nil, err
		}
		if job.ID != id {
			return nil, fmt.Errorf("ID job Hararest berubah saat polling")
		}
	}
}

// Respect the server's minimum delay and spread simultaneous pollers.
func pollDelay(serverMs int64, fallback time.Duration) time.Duration {
	delay := fallback
	if serverMs > 0 {
		delay = time.Duration(min(serverMs, int64(30000))) * time.Millisecond
	}
	delay = max(time.Second, min(delay, 30*time.Second))
	return delay + time.Duration(rand.Int64N(int64(delay/5)+1))
}
