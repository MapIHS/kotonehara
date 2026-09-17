package handlers

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	bilimedia "github.com/MapIHS/kotonehara/internal/media/bilibili"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
	"go.mau.fi/whatsmeow"
)

func TestBilibiliOpusDispatch(t *testing.T) {
	for _, fails := range []bool{false, true} {
		var replies []string
		m := &message.Message{Query: "https://www.bilibili.com/opus/1247969693541597185"}
		m.Reply = func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			replies = append(replies, text)
			return whatsmeow.SendResponse{}, nil
		}
		calls := 0
		handleBilibili(context.Background(), m,
			func(context.Context, string, string) (*api.BilibiliResult, error) {
				return &api.BilibiliResult{ID: "1247969693541597185", Format: "images"}, nil
			},
			func(context.Context, bilimedia.Selection, map[string]string) ([]byte, error) {
				t.Fatal("Opus passed to video merger")
				return nil, nil
			},
			func(context.Context, []byte, string) error { t.Fatal("Opus passed to video sender"); return nil },
			func(ctx context.Context, result *api.BilibiliResult) error {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Error("missing deadline")
				}
				if fails {
					return errors.New("partial failure")
				}
				return nil
			},
		)
		if calls != 1 {
			t.Fatal("Opus not dispatched")
		}
		if fails && !strings.Contains(replies[len(replies)-1], "partial failure") {
			t.Fatal("Opus failure hidden")
		}
	}
}

func TestSendBilibiliImages(t *testing.T) {
	for _, failedStage := range []string{"none", "fetch", "send"} {
		t.Run(failedStage, func(t *testing.T) {
			result := &api.BilibiliResult{Format: "images", Description: "Hey？", Media: []api.BilibiliMedia{
				{Type: "image", URL: "first"}, {Type: "gif", URL: "second"}, {Type: "image", URL: "third"},
			}}
			result.Author.Name = "Author"
			var order, captions []string
			var gifFlags []bool
			budget := int64(256 << 20)
			err := sendBilibiliImages(context.Background(), result,
				func(ctx context.Context, item api.BilibiliMedia, remaining int64) ([]byte, error) {
					if remaining != budget {
						t.Error("incorrect byte budget")
					}
					if item.URL == "first" && failedStage == "fetch" {
						return nil, errors.New("fetch failed")
					}
					data := []byte(item.URL)
					if item.Type == "gif" {
						data = []byte("GIF89a" + item.URL)
					}
					budget -= int64(len(data))
					return data, nil
				},
				func(ctx context.Context, data []byte, gif bool, caption string) error {
					if string(data) == "first" && failedStage == "send" {
						return errors.New("send failed")
					}
					order = append(order, string(data))
					captions = append(captions, caption)
					gifFlags = append(gifFlags, gif)
					return nil
				},
			)
			want := []string{"first", "GIF89asecond", "third"}
			flags := []bool{false, true, false}
			if failedStage != "none" {
				want = want[1:]
				flags = flags[1:]
				if err == nil || !strings.Contains(err.Error(), "2 dari 3") {
					t.Fatalf("partial failure hidden: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(order, want) || !reflect.DeepEqual(gifFlags, flags) {
				t.Fatalf("wrong media order/type: %v %v", order, gifFlags)
			}
			if captions[0] != "Author\n\nHey？" {
				t.Fatalf("missing caption on first successful send: %v", captions)
			}
			for _, caption := range captions[1:] {
				if caption != "" {
					t.Fatal("caption repeated")
				}
			}
		})
	}
}

func TestSendBilibiliImagesRejectsInvalidMedia(t *testing.T) {
	for _, items := range [][]api.BilibiliMedia{nil, {{Type: "video"}}, make([]api.BilibiliMedia, 21)} {
		err := sendBilibiliImages(context.Background(), &api.BilibiliResult{Media: items},
			func(context.Context, api.BilibiliMedia, int64) ([]byte, error) {
				t.Fatal("invalid gallery fetched")
				return nil, nil
			},
			func(context.Context, []byte, bool, string) error { t.Fatal("invalid gallery sent"); return nil })
		if err == nil {
			t.Fatal("invalid gallery accepted")
		}
	}
}
