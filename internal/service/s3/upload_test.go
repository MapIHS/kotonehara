package s3

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUploadPresignThenPut(t *testing.T) {
	payload := []byte("hello world")
	var stored []byte
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/upload/presign":
			var req struct {
				Filename string `json:"filename"`
				Size     int    `json:"size"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Filename != "a.txt" || req.Size != len(payload) {
				t.Errorf("presign request %+v", req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"key": "k1", "upload_url": srv.URL + "/put/k1", "method": "PUT",
				"headers": map[string][]string{"X-Sig": {"abc"}},
			}})
		case r.Method == http.MethodPut && r.URL.Path == "/put/k1":
			if r.Header.Get("X-Sig") != "abc" {
				t.Error("signed header not forwarded")
			}
			stored, _ = io.ReadAll(r.Body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	url, err := New(srv.URL, 5*time.Second).Upload("dir/a.txt", payload)
	if err != nil {
		t.Fatal(err)
	}
	if url != srv.URL+"/file/k1" || string(stored) != string(payload) {
		t.Fatalf("url=%q stored=%q", url, stored)
	}
}

func TestUploadRejectsBadPresign(t *testing.T) {
	for name, body := range map[string]string{
		"api error":    `{"error":"quota"}`,
		"wrong method": `{"data":{"key":"k","upload_url":"http://x/y","method":"POST"}}`,
		"bad scheme":   `{"data":{"key":"k","upload_url":"file:///etc/passwd","method":"PUT"}}`,
		"length lie":   `{"data":{"key":"k","upload_url":"http://127.0.0.1:1/p","method":"PUT","headers":{"Content-Length":["999"]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			if _, err := New(srv.URL, 5*time.Second).Upload("a", []byte("data")); err == nil {
				t.Fatal("bad presign accepted")
			}
		})
	}
}

func TestUploadDoesNotFollowRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"data":{"key":"k","upload_url":"`+srv.URL+`/put","method":"PUT"}}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/put") {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		t.Error("redirect was followed")
	}))
	defer srv.Close()
	if _, err := New(srv.URL, 5*time.Second).Upload("a", []byte("data")); err == nil {
		t.Fatal("redirected upload reported success")
	}
}

func TestUploadErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, 5*time.Second).Upload("a", []byte("x")); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v", err)
	}
}
