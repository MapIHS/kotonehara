package clients

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestAlbumAssociatesMixedMediaAndCountsOnlyPreparedAttachments(t *testing.T) {
	failed := errors.New("download failed")
	medias := []AlbumMedia{{URL: "image"}, {URL: "bad"}, {URL: "video", IsVideo: true}}
	var calls []*waE2E.Message
	prepare := func(_ context.Context, item AlbumMedia) (*waE2E.Message, error) {
		if item.URL == "bad" {
			return nil, failed
		}
		if item.IsVideo {
			return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, nil
		}
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, nil
	}
	send := func(_ context.Context, _ types.JID, msg *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		calls = append(calls, proto.Clone(msg).(*waE2E.Message))
		return whatsmeow.SendResponse{ID: types.MessageID(fmt.Sprintf("id-%d", len(calls)))}, nil
	}
	chat := types.NewJID("123", types.GroupServer)
	quote := &waE2E.ContextInfo{StanzaID: proto.String("quoted")}
	responses, err := sendAlbum(context.Background(), chat, medias, quote, prepare, send)
	if !errors.Is(err, failed) || len(responses) != 3 || len(calls) != 3 {
		t.Fatalf("responses=%d calls=%d err=%v", len(responses), len(calls), err)
	}
	album := calls[0].GetAlbumMessage()
	if album.GetExpectedImageCount() != 1 || album.GetExpectedVideoCount() != 1 || album.GetContextInfo().GetStanzaID() != "quoted" {
		t.Fatalf("album=%v", album)
	}
	for _, msg := range calls[1:] {
		association := msg.GetMessageContextInfo().GetMessageAssociation()
		key := association.GetParentMessageKey()
		if association.GetAssociationType() != waE2E.MessageAssociation_MEDIA_ALBUM || key.GetID() != "id-1" || key.GetRemoteJID() != chat.String() || !key.GetFromMe() {
			t.Fatalf("association=%v", association)
		}
	}
}

func TestAlbumSingleMediaKeepsQuoteWithoutContainer(t *testing.T) {
	for _, video := range []bool{false, true} {
		t.Run(fmt.Sprint(video), func(t *testing.T) {
			calls := 0
			quote := &waE2E.ContextInfo{StanzaID: proto.String("quoted")}
			prepare := func(context.Context, AlbumMedia) (*waE2E.Message, error) {
				if video {
					return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, nil
				}
				return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, nil
			}
			send := func(_ context.Context, _ types.JID, msg *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				calls++
				if msg.GetAlbumMessage() != nil || msg.GetMessageContextInfo() != nil {
					t.Fatal("single media wrapped as album")
				}
				ci := msg.GetImageMessage().GetContextInfo()
				if video {
					ci = msg.GetVideoMessage().GetContextInfo()
				}
				if ci.GetStanzaID() != "quoted" {
					t.Fatal("quote missing")
				}
				return whatsmeow.SendResponse{ID: "single"}, nil
			}
			responses, err := sendAlbum(context.Background(), types.EmptyJID, []AlbumMedia{{}}, quote, prepare, send)
			if err != nil || calls != 1 || len(responses) != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestAlbumFailureAndCancellationDoNotSendUnexpectedMessages(t *testing.T) {
	for _, mode := range []string{"prepare", "container", "cancel", "count"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sentinel := errors.New("failure")
			calls := 0
			medias := []AlbumMedia{{}, {}}
			if mode == "count" {
				medias = make([]AlbumMedia, MaxAlbumItems+1)
			}
			prepare := func(context.Context, AlbumMedia) (*waE2E.Message, error) {
				if mode == "prepare" {
					return nil, sentinel
				}
				if mode == "cancel" {
					cancel()
				}
				return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, nil
			}
			send := func(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				calls++
				return whatsmeow.SendResponse{}, sentinel
			}
			_, err := sendAlbum(ctx, types.EmptyJID, medias, nil, prepare, send)
			want := 0
			if mode == "container" {
				want = 1
			}
			if err == nil || calls != want {
				t.Fatalf("calls=%d want=%d err=%v", calls, want, err)
			}
		})
	}
}

type albumRoundTrip func(*http.Request) (*http.Response, error)

func (f albumRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAlbumLazyDownloadsAreBoundedAndKeepHeadersAndCancellation(t *testing.T) {
	calls := 0
	c := &Client{HTTP: &http.Client{Transport: albumRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Referer") != "https://source.example/" {
			t.Fatal("missing required source header")
		}
		length := int64(-1)
		if strings.Contains(r.URL.Path, "declared") {
			length = 100
		}
		return &http.Response{StatusCode: 200, ContentLength: length, Body: io.NopCloser(strings.NewReader("0123456789")), Header: make(http.Header)}, nil
	})}}
	item := NewAlbumImageURL("https://cdn.example/image")
	item.Headers = map[string]string{"Referer": "https://source.example/"}
	if calls != 0 || len(item.Data) != 0 {
		t.Fatal("descriptor fetched media eagerly")
	}
	if _, err := c.loadAlbumMedia(context.Background(), item, 5); !errors.Is(err, ErrAlbumLimit) {
		t.Fatalf("stream limit: %v", err)
	}
	item.URL = "https://cdn.example/declared"
	if _, err := c.loadAlbumMedia(context.Background(), item, 5); !errors.Is(err, ErrAlbumLimit) {
		t.Fatalf("declared limit: %v", err)
	}
	item.URL = "https://cdn.example/image"
	data, err := c.loadAlbumMedia(context.Background(), item, 10)
	if err != nil || len(data) != 10 {
		t.Fatalf("valid media: len=%d err=%v", len(data), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := calls
	if _, err := c.loadAlbumMedia(ctx, item, 10); !errors.Is(err, context.Canceled) || calls != before {
		t.Fatalf("cancel: calls=%d err=%v", calls, err)
	}
	item = AlbumMedia{Fetch: func(_ context.Context, limit int64) ([]byte, error) {
		if limit != 5 {
			t.Fatal("incorrect remaining budget")
		}
		return make([]byte, 6), nil
	}}
	if _, err := c.loadAlbumMedia(context.Background(), item, 5); !errors.Is(err, ErrAlbumLimit) {
		t.Fatalf("custom loader limit: %v", err)
	}
}

func TestAlbumChildFailureReturnsPartialResultAndContinues(t *testing.T) {
	failed := errors.New("child failed")
	calls := 0
	prepare := func(context.Context, AlbumMedia) (*waE2E.Message, error) {
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, nil
	}
	send := func(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		calls++
		if calls == 2 {
			return whatsmeow.SendResponse{}, failed
		}
		return whatsmeow.SendResponse{ID: types.MessageID(fmt.Sprint(calls))}, nil
	}
	responses, err := sendAlbum(context.Background(), types.EmptyJID, []AlbumMedia{{}, {}}, nil, prepare, send)
	if !errors.Is(err, failed) || calls != 3 || len(responses) != 2 {
		t.Fatalf("calls=%d responses=%d err=%v", calls, len(responses), err)
	}
}

func TestAlbumRejectsOversizedGIFCanvasBeforeConversionOrUpload(t *testing.T) {
	// GIF logical screen is 5000x5000; no pixel buffer is allocated by this test.
	data := append([]byte("GIF89a"), 0x88, 0x13, 0x88, 0x13, 0, 0, 0)
	_, err := (&Client{}).prepareAlbumVideo(context.Background(), NewAlbumVideo(data, true))
	if !errors.Is(err, ErrAlbumLimit) {
		t.Fatalf("canvas limit: %v", err)
	}
}

func TestAlbumPartialDeliveryCountsAttachmentsWithoutContainer(t *testing.T) {
	for _, valid := range []int{1, 4} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			index, calls := 0, 0
			cause := &AlbumHTTPError{StatusCode: 403}
			prepare := func(context.Context, AlbumMedia) (*waE2E.Message, error) {
				index++
				if index > valid {
					return nil, cause
				}
				return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, nil
			}
			send := func(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
				calls++
				return whatsmeow.SendResponse{ID: types.MessageID(fmt.Sprint(calls))}, nil
			}
			responses, err := sendAlbum(context.Background(), types.EmptyJID, make([]AlbumMedia, 5), nil, prepare, send)
			var partial *PartialAlbumError
			if !errors.As(err, &partial) || partial.Sent != valid || partial.Total != 5 || !errors.Is(err, cause) {
				t.Fatalf("partial=%+v err=%v", partial, err)
			}
			expected := valid
			if valid > 1 {
				expected++
			}
			if len(responses) != expected {
				t.Fatalf("responses=%d expected=%d", len(responses), expected)
			}
			var status *AlbumHTTPError
			if !errors.As(err, &status) || status.StatusCode != 403 {
				t.Fatalf("underlying status lost: %v", err)
			}
		})
	}
}

func TestAlbumContainerAcknowledgementAloneIsNotPartialDelivery(t *testing.T) {
	calls := 0
	cause := errors.New("child failed")
	prepare := func(context.Context, AlbumMedia) (*waE2E.Message, error) {
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, nil
	}
	send := func(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		calls++
		if calls > 1 {
			return whatsmeow.SendResponse{}, cause
		}
		return whatsmeow.SendResponse{ID: "container"}, nil
	}
	responses, err := sendAlbum(context.Background(), types.EmptyJID, []AlbumMedia{{}, {}}, nil, prepare, send)
	var partial *PartialAlbumError
	if len(responses) != 1 || !errors.Is(err, cause) || errors.As(err, &partial) {
		t.Fatalf("responses=%d partial=%v err=%v", len(responses), partial, err)
	}
}
