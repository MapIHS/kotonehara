package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchBudgetAndDateContext(t *testing.T) {
	t.Setenv("OPENAI_WEB_SEARCH", "1")
	t.Setenv("OPENAI_IMAGE_MODEL", "")
	chats, searches := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/search" {
			searches++
			if r.Header.Get("Authorization") != "Bearer fixture" {
				t.Error("missing key")
			}
			w.Write([]byte(`{"results":[{"title":"Fixture","url":"https://example.com/rate","snippet":"Rate dated today"}]}`))
			return
		}
		chats++
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[0].Content, time.Now().UTC().Add(7*time.Hour).Format("2006-01-02")) {
			t.Error("missing current date")
		}
		if chats < 3 {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"reasoning_content":"provider metadata","tool_calls":[{"id":"call","type":"function","function":{"name":"web_search","arguments":"{\"query\":\"dated rate\"}"}}]}}]}`))
			return
		}
		if req.ToolChoice != "none" || len(req.Tools) == 0 {
			t.Error("final synthesis must disable tool calls explicitly")
		}
		found := false
		for _, m := range req.Messages {
			if m.ReasoningContent == "provider metadata" {
				found = true
			}
		}
		if !found {
			t.Error("lost provider reasoning metadata")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"Verified source https://example.com/rate"}}]}`))
	}))
	defer server.Close()
	reply, err := New(server.URL+"/v1", "fixture", "model", time.Second).Chat(context.Background(), []Message{{Role: "user", Content: "Rate today?"}})
	if err != nil || reply.Text == "" || searches != 2 || chats != 3 {
		t.Fatalf("chats=%d searches=%d err=%v", chats, searches, err)
	}
}

func TestImageToolReturnsUsableImage(t *testing.T) {
	t.Setenv("OPENAI_WEB_SEARCH", "")
	t.Setenv("OPENAI_IMAGE_MODEL", "image-model")
	var encoded bytes.Buffer
	png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"image","type":"function","function":{"name":"generate_image","arguments":"{\"prompt\":\"cat\"}"}}]}}]}`))
			return
		}
		if r.URL.Path != "/v1/images/generations" || r.URL.Query().Get("response_format") != "binary" {
			t.Error("wrong image endpoint")
		}
		var payload struct {
			Model  string
			Prompt string
		}
		json.NewDecoder(r.Body).Decode(&payload)
		if payload.Model != "image-model" || payload.Prompt != "cat" {
			t.Error("wrong image parameters")
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(encoded.Bytes())
	}))
	defer server.Close()
	reply, err := New(server.URL+"/v1", "fixture", "chat-model", time.Second).Chat(context.Background(), []Message{{Role: "user", Content: "Draw cat"}})
	if err != nil || len(reply.Images) != 1 || !strings.HasPrefix(reply.Images[0], "data:image/png;base64,") {
		t.Fatalf("image missing, err=%v", err)
	}
}
