package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
	"go.mau.fi/whatsmeow"
)

func TestParsePixivQuery(t *testing.T) {
	for _, tc := range []struct{ input, query, mode string }{
		{"blue sky", "blue sky", "all"}, {"safe all", "safe all", "all"},
		{"blue sky --mode safe", "blue sky", "safe"}, {"--mode=r18 blue sky", "blue sky", "r18"},
		{"https://www.pixiv.net/users/123 --mode all", "https://www.pixiv.net/users/123", "all"},
	} {
		q, m, err := parsePixivQuery(tc.input)
		if err != nil || q != tc.query || m != tc.mode {
			t.Errorf("%q: %q %q %v", tc.input, q, m, err)
		}
	}
	for _, input := range []string{"", "--mode safe", "cat --mode", "cat --mode=", "cat --mode unsafe", "cat --mode safe --mode all"} {
		if _, _, err := parsePixivQuery(input); err == nil {
			t.Errorf("accepted invalid input %q", input)
		}
	}
}

func TestProfileFormattingIsBoundedAndHandlesEmptyResults(t *testing.T) {
	pins := make([]api.PinterestSearchItem, 6)
	for i := range pins {
		pins[i] = api.PinterestSearchItem{Title: fmt.Sprint("pin", i), Link: fmt.Sprint("https://pinterest.com/pin/", i)}
	}
	zero := 0
	pin := formatPinterestProfile(&api.PinterestUserResponse{User: api.PinterestUserProfile{Username: "artist", FullName: "Artist", Bio: strings.Repeat("界", 1000), Followers: &zero}, Pins: pins})
	if !strings.Contains(pin, "Pengikut: 0") || strings.Contains(pin, "Mengikuti:") || strings.Contains(pin, "pin5") || strings.Contains(pin, strings.Repeat("界", 701)) {
		t.Fatal("profile fields or bounds incorrect")
	}
	pixiv := formatPixivProfile(&api.PixivUserResponse{User: api.PixivUserProfile{ID: "123", Name: "Artist"}}, "safe")
	if !strings.Contains(pixiv, "https://www.pixiv.net/users/123") || !strings.Contains(pixiv, "Tidak ada artwork") {
		t.Fatal(pixiv)
	}
}

func TestProfileCommandsDispatchToAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pinterest/user":
			if r.URL.Query().Get("username") != "artist" {
				t.Error("lost username")
			}
			fmt.Fprint(w, `{"data":{"user":{"username":"artist","fullName":"Pin Artist"},"pins":[]}}`)
		case "/api/pixiv/user":
			if r.URL.Query().Get("id") != "123" || r.URL.Query().Get("mode") != "safe" {
				t.Error("lost Pixiv arguments")
			}
			fmt.Fprint(w, `{"data":{"user":{"id":"123","name":"Pixiv Artist"},"artworks":[]}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	for _, tc := range []struct{ body, query, want string }{
		{".pinterestuser artist", "artist", "Pin Artist"},
		{".pxuser 123 --mode safe", "123 --mode safe", "Pixiv Artist"},
	} {
		var command *commands.Command
		for _, item := range commands.Commands() {
			if item.Name == "pinuser" && strings.Contains(tc.body, "pinterestuser") || item.Name == "pixivuser" && strings.Contains(tc.body, "pxuser") {
				copy := item
				command = &copy
				break
			}
		}
		if command == nil || !commands.CanHandle(tc.body, config.Config{Prefix: "."}) {
			t.Fatalf("command alias not registered: %s", tc.body)
		}
		reply := ""
		m := &message.Message{Query: tc.query, Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			reply = text
			return whatsmeow.SendResponse{}, nil
		}}
		command.Exec(context.Background(), nil, m, config.Config{BASEApiURL: server.URL})
		if !strings.Contains(reply, tc.want) {
			t.Fatalf("unexpected reply: %s", reply)
		}
	}
}

func TestPixivSearchCommandForwardsMode(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/pixiv/search" || r.URL.Query().Get("q") != "blue sky" || r.URL.Query().Get("mode") != "safe" {
			t.Errorf("unexpected request %s", r.URL)
		}
		fmt.Fprint(w, `{"data":{"results":[]}}`)
	}))
	defer server.Close()
	for _, command := range commands.Commands() {
		if command.Name != "pixivsearch" {
			continue
		}
		reply := ""
		m := &message.Message{Query: "blue sky --mode safe", Reply: func(_ context.Context, text string, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			reply = text
			return whatsmeow.SendResponse{}, nil
		}}
		command.Exec(context.Background(), nil, m, config.Config{BASEApiURL: server.URL})
		if calls != 1 {
			t.Fatal("search request not sent")
		}
		m.Query = "blue sky --mode bad"
		command.Exec(context.Background(), nil, m, config.Config{BASEApiURL: server.URL})
		if calls != 1 || !strings.Contains(reply, "mode Pixiv") {
			t.Fatal("invalid mode reached API")
		}
		return
	}
	t.Fatal("pixivsearch not registered")
}
