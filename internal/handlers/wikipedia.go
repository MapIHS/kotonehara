package handlers

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/api"
)

var wikipediaHost = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.m)?\.wikipedia\.org$`)

func parseWikipediaQuery(query string) (string, error) {
	args := strings.Fields(query)
	if len(args) != 1 {
		return "", fmt.Errorf("gunakan wiki <url>, contoh: wiki https://id.wikipedia.org/wiki/Kucing")
	}
	u, err := url.Parse(args[0])
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" || !wikipediaHost.MatchString(strings.ToLower(u.Hostname())) {
		return "", fmt.Errorf("link Wikipedia tidak valid; gunakan URL artikel Wikipedia")
	}
	if !strings.HasPrefix(u.Path, "/wiki/") || strings.TrimSpace(strings.TrimPrefix(u.Path, "/wiki/")) == "" {
		return "", fmt.Errorf("gunakan URL Wikipedia dengan format /wiki/Judul_artikel")
	}
	u.Scheme = "https"
	u.Host = strings.Replace(strings.ToLower(u.Hostname()), ".m.wikipedia.org", ".wikipedia.org", 1)
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func wikiExcerpt(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return string(runes)
}

func handleWikipedia(ctx context.Context, m *message.Message, fetch func(context.Context, string) (*api.WikipediaArticle, error)) {
	target, err := parseWikipediaQuery(m.Query)
	if err != nil {
		m.Reply(ctx, err.Error())
		return
	}
	workCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	m.Reply(ctx, "Sedang mengambil artikel Wikipedia...")
	article, err := fetch(workCtx, target)
	if err != nil {
		m.Reply(ctx, "Gagal mengambil Wikipedia: "+err.Error())
		return
	}
	if article == nil || strings.TrimSpace(article.Title) == "" {
		m.Reply(ctx, "Artikel Wikipedia tidak ditemukan.")
		return
	}
	text := strings.TrimSpace(article.Summary)
	if text == "" {
		text = strings.TrimSpace(article.Content)
	}
	if text == "" {
		m.Reply(ctx, "Artikel Wikipedia tidak memiliki teks yang dapat dibaca.")
		return
	}
	source := target
	if canonical, err := parseWikipediaQuery(article.SourceURL); err == nil {
		source = canonical
	}
	title := wikiExcerpt(article.Title, 200)
	if language := wikiExcerpt(article.Language, 30); language != "" {
		title += " (" + language + ")"
	}
	m.Reply(ctx, title+"\n\n"+wikiExcerpt(text, 3500)+"\n\nBaca selengkapnya:\n"+source)
}

func init() {
	commands.Register(&commands.Command{
		Name: "wiki", As: []string{"wikipedia"}, Tags: "tools", Description: "Cari atau baca Wikipedia: <kata kunci atau url>", IsQuery: true, IsPrefix: true,
		Exec: func(ctx context.Context, _ *clients.Client, m *message.Message, cfg config.Config) {
			if strings.TrimSpace(cfg.BASEApiURL) == "" {
				m.Reply(ctx, "Fitur Wikipedia belum dikonfigurasi (BASEAPI_URL kosong).")
				return
			}
			ap := api.Shared(cfg.BASEApiURL, 60*time.Second)
			handleWikipediaInput(ctx, m, ap.Wikipedia, ap.WikipediaSearch)
		},
	})
}

var wikipediaURLInput = regexp.MustCompile(`(?i)^(?:[a-z][a-z0-9+.-]*:|//|www\.)`)

func handleWikipediaInput(ctx context.Context, m *message.Message,
	fetch func(context.Context, string) (*api.WikipediaArticle, error),
	search func(context.Context, string) (*api.WikipediaSearchResult, error),
) {
	query := strings.TrimSpace(m.Query)
	if wikipediaURLInput.MatchString(query) {
		handleWikipedia(ctx, m, fetch)
		return
	}
	if query == "" || len([]rune(query)) > 300 {
		m.Reply(ctx, "Gunakan wiki <kata kunci atau URL>, maksimal 300 karakter. Contoh: wiki kucing anggora")
		return
	}
	workCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	m.Reply(ctx, "Sedang mencari di Wikipedia...")
	result, err := search(workCtx, query)
	if err != nil {
		m.Reply(ctx, "Gagal mencari Wikipedia: "+err.Error())
		return
	}
	if result == nil || len(result.Results) == 0 {
		m.Reply(ctx, "Tidak ada artikel Wikipedia yang cocok. Coba kata kunci lain.")
		return
	}
	var entries []string
	for _, item := range result.Results {
		source, err := parseWikipediaQuery(item.URL)
		if err != nil || strings.TrimSpace(item.Title) == "" {
			continue
		}
		entries = append(entries, fmt.Sprintf("%d. %s\n%s\n%s", len(entries)+1, wikiExcerpt(item.Title, 150), wikiExcerpt(item.Snippet, 400), source))
		if len(entries) == 5 {
			break
		}
	}
	if len(entries) == 0 {
		m.Reply(ctx, "Hasil pencarian Wikipedia tidak memiliki tautan artikel yang valid.")
		return
	}
	m.Reply(ctx, "Hasil Wikipedia: "+wikiExcerpt(query, 150)+"\n\n"+strings.Join(entries, "\n\n")+"\n\nUntuk ringkasan artikel, gunakan perintah wiki diikuti salah satu URL di atas.")
}
