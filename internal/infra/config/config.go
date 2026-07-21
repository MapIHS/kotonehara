package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv               string
	Prefix               string
	LoginMethod          string
	PairingPhoneNumber   string
	DBDriver             string
	DBURL                string
	Owners               []string
	Cooldown             time.Duration
	AdminTTL             time.Duration
	DisableContactImport bool
	BASEApiURL           string
	BASES3URL            string
	RemoveBGURL          string
	OpenAIBaseURL        string
	OpenAIAPIKey         string
	OpenAIModel          string
	OpenAIProviders      []OpenAIProvider
	OpenAIProvidersError string
	OpenAISystemPrompt   string
	OpenAITimeout        time.Duration
}

// OpenAIProvider describes one OpenAI-compatible upstream. Models can contain
// more than one model; each is included in the router rotation.
type OpenAIProvider struct {
	Name    string
	BaseURL string
	APIKey  string
	Models  []string
	Enabled bool
}

func Load() Config {
	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		env = "dev"
	}

	prefix := strings.TrimSpace(os.Getenv("PREFIX"))
	if prefix == "" {
		prefix = "."
	}

	loginMethod := normalizeLoginMethod(os.Getenv("LOGIN_METHOD"))
	pairingPhoneNumber := sanitizePhoneNumber(os.Getenv("PAIRING_PHONE_NUMBER"))

	dbDriver := normalizeDBDriver(os.Getenv("DB_DRIVER"))
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))

	owners := parseCSV(os.Getenv("OWNER"))

	cd := parseDurationOrSeconds(os.Getenv("COOLDOWN"), 3*time.Second)
	adminTTL := parseDurationOrSeconds(os.Getenv("ADMIN_TTL"), 45*time.Second)
	disableContactImport := parseBoolDefault(os.Getenv("DISABLE_CONTACT_IMPORT"), true)

	baseurl := strings.TrimSpace(os.Getenv("BASEAPI_URL"))

	bases3url := strings.TrimSpace(os.Getenv("BASES3_URL"))
	removeBGURL := strings.TrimRight(strings.TrimSpace(os.Getenv("REMOVEBG_URL")), "/")
	openAIBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), "/")
	if openAIBaseURL == "" {
		openAIBaseURL = "https://api.openai.com/v1"
	}
	openAIAPIKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	openAIModel := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	openAIProviders, openAIProvidersErr := loadOpenAIProviders()
	openAISystemPrompt := strings.TrimSpace(os.Getenv("OPENAI_SYSTEM_PROMPT"))
	if openAISystemPrompt == "" {
		openAISystemPrompt = "Kamu adalah Kotonehara, asisten WhatsApp yang membantu dengan jawaban jelas, ringkas, dan ramah dalam Bahasa Indonesia."
	}
	openAITimeout := parseDurationOrSeconds(os.Getenv("OPENAI_TIMEOUT"), 90*time.Second)

	return Config{
		AppEnv:               env,
		Prefix:               prefix,
		LoginMethod:          loginMethod,
		PairingPhoneNumber:   pairingPhoneNumber,
		DBDriver:             dbDriver,
		DBURL:                dbURL,
		Owners:               owners,
		Cooldown:             cd,
		AdminTTL:             adminTTL,
		DisableContactImport: disableContactImport,
		BASEApiURL:           baseurl,
		BASES3URL:            bases3url,
		RemoveBGURL:          removeBGURL,
		OpenAIBaseURL:        openAIBaseURL,
		OpenAIAPIKey:         openAIAPIKey,
		OpenAIModel:          openAIModel,
		OpenAIProviders:      openAIProviders,
		OpenAIProvidersError: openAIProvidersErr,
		OpenAISystemPrompt:   openAISystemPrompt,
		OpenAITimeout:        openAITimeout,
	}
}

// loadOpenAIProviders accepts either OPENAI_PROVIDERS (a JSON array) or
// OPENAI_PROVIDERS_FILE (a path to a JSON file). The format mirrors AiRouter:
// [{"name":"...","base_url":"...","api_key":"...","model":"...","enabled":true}].
// "model" may also be an array to rotate models on the same upstream.
func loadOpenAIProviders() ([]OpenAIProvider, string) {
	raw := strings.TrimSpace(os.Getenv("OPENAI_PROVIDERS"))
	if raw == "" {
		file := strings.TrimSpace(os.Getenv("OPENAI_PROVIDERS_FILE"))
		if file == "" {
			return nil, ""
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Sprintf("tidak dapat membaca OPENAI_PROVIDERS_FILE: %v", err)
		}
		raw = string(data)
	}

	var items []struct {
		Name    string          `json:"name"`
		BaseURL string          `json:"base_url"`
		APIKey  string          `json:"api_key"`
		Model   json.RawMessage `json:"model"`
		Enabled *bool           `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Sprintf("OPENAI_PROVIDERS harus berupa JSON array: %v", err)
	}

	providers := make([]OpenAIProvider, 0, len(items))
	for i, item := range items {
		models, err := parseProviderModels(item.Model)
		if err != nil {
			return nil, fmt.Sprintf("provider AI #%d: %v", i+1, err)
		}
		provider := OpenAIProvider{
			Name:    strings.TrimSpace(item.Name),
			BaseURL: strings.TrimRight(strings.TrimSpace(item.BaseURL), "/"),
			APIKey:  strings.TrimSpace(item.APIKey),
			Models:  models,
			Enabled: item.Enabled != nil && *item.Enabled,
		}
		if provider.Name == "" || provider.BaseURL == "" || provider.APIKey == "" || len(provider.Models) == 0 {
			return nil, fmt.Sprintf("provider AI #%d wajib memiliki name, base_url, api_key, model, dan enabled", i+1)
		}
		providers = append(providers, provider)
	}
	return providers, ""
}

func parseProviderModels(raw json.RawMessage) ([]string, error) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		if one = strings.TrimSpace(one); one != "" {
			return []string{one}, nil
		}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		out := make([]string, 0, len(many))
		for _, model := range many {
			if model = strings.TrimSpace(model); model != "" {
				out = append(out, model)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("model harus berupa string atau array string yang tidak kosong")
}

// normalizeDBDriver maps common aliases to the drivers actually wired up in
// cmd/bot ("postgres" via lib/pq, "sqlite" via modernc.org/sqlite). Defaults to
// "postgres" for an empty or unrecognized value.
func normalizeDBDriver(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "":
		return "postgres"
	case strings.HasPrefix(s, "postgres") || s == "pg" || s == "pgx":
		return "postgres"
	case strings.HasPrefix(s, "sqlite") || s == "lite":
		return "sqlite"
	default:
		return "postgres"
	}
}

// normalizeLoginMethod maps common aliases to the two login flows supported by
// cmd/bot: "qr" (scan a QR code, default) or "pairing" (enter a code on the
// phone). Defaults to "qr" for an empty or unrecognized value.
func normalizeLoginMethod(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "":
		return "qr"
	case strings.HasPrefix(s, "pair") || s == "phone" || s == "code":
		return "pairing"
	case strings.HasPrefix(s, "qr"):
		return "qr"
	default:
		return "qr"
	}
}

// sanitizePhoneNumber strips everything but digits, so users can supply the
// number with or without "+", spaces, or dashes.
func sanitizePhoneNumber(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func parseCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseDurationOrSeconds(s string, def time.Duration) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	return def
}

func parseBool(s string) bool {
	return parseBoolDefault(s, false)
}

func parseBoolDefault(s string, def bool) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return def
	}
	switch s {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}
