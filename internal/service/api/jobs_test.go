package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMediaJobRetriesCreationWithoutDuplicateKeyAndPolls(t *testing.T) {
	id := strings.Repeat("b", 48)
	creates, polls := 0, 0
	var key string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := "queued"
		if r.Method == http.MethodPost {
			creates++
			if creates == 1 {
				key = r.Header.Get("Idempotency-Key")
				// Simulate a proxy losing the response after the origin accepted it.
				w.WriteHeader(524)
				return
			}
			if key == "" || r.Header.Get("Idempotency-Key") != key {
				t.Error("retry must reuse the idempotency key")
			}
			w.WriteHeader(http.StatusAccepted)
		} else {
			polls++
			if r.URL.Path != "/api/jobs/"+id {
				t.Errorf("wrong status path: %s", r.URL.Path)
			}
			state = "processing"
			if polls == 2 {
				state = "ready"
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "state": state}})
	}))
	defer server.Close()
	job, err := New(server.URL, time.Second).runMediaJob(context.Background(), "/api/youtube/jobs", map[string]string{"url": "https://youtu.be/test", "format": "audio"}, time.Millisecond)
	if err != nil || job.State != "ready" || creates != 2 || polls != 2 {
		t.Fatalf("job=%v err=%v creates=%d polls=%d", job, err, creates, polls)
	}
}

func TestMediaJobStopsOnFailureExpiryAndInvalidResponses(t *testing.T) {
	id := strings.Repeat("c", 48)
	for _, tc := range []struct {
		name, state, returnedID string
		status                  int
	}{
		{"failed", "failed", id, 200}, {"expired", "", id, 410},
		{"wrong id", "ready", strings.Repeat("d", 48), 200}, {"bad state", "unknown", id, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method == http.MethodPost {
					w.WriteHeader(202)
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "state": "queued"}})
				} else {
					w.WriteHeader(tc.status)
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": tc.returnedID, "state": tc.state, "error": "upstream gagal"}})
				}
			}))
			defer server.Close()
			_, err := New(server.URL, time.Second).runMediaJob(context.Background(), "/api/youtube/jobs", nil, time.Millisecond)
			if err == nil || requests != 2 {
				t.Fatalf("err=%v requests=%d", err, requests)
			}
			if tc.state == "failed" && !strings.Contains(err.Error(), "upstream gagal") {
				t.Fatal("job error details were lost")
			}
		})
	}
}

func TestMediaJobContextCancelsPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": strings.Repeat("a", 48), "state": "queued"}})
		cancel()
	}))
	defer server.Close()
	_, err := New(server.URL, time.Second).runMediaJob(ctx, "/api/youtube/jobs", nil, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestMediaJobRejectsMalformedIDWithoutFollowingServerURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		w.Write([]byte(`{"data":{"id":"../../evil","state":"ready","result":{"fileUrl":"http://evil.test"}}}`))
	}))
	defer server.Close()
	_, err := New(server.URL, time.Second).runMediaJob(context.Background(), "/api/youtube/jobs", nil, time.Millisecond)
	if err == nil {
		t.Fatal("malformed job ID accepted")
	}
}

func TestYouTubeJobRejectsTruncatedFile(t *testing.T) {
	id := strings.Repeat("a", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(202)
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "state": "ready", "result": map[string]any{"kind": "file", "size": 100}}})
		} else {
			w.Write([]byte("partial"))
		}
	}))
	defer server.Close()
	_, err := New(server.URL, time.Second).YoutubeDownload(context.Background(), "https://youtu.be/test", "", false)
	if err == nil || !strings.Contains(err.Error(), "tidak lengkap") {
		t.Fatalf("expected truncated file error, got %v", err)
	}
}
