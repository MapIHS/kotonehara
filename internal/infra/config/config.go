package config

import (
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
	OpenAISystemPrompt   string
	OpenAITimeout        time.Duration
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
		OpenAISystemPrompt:   openAISystemPrompt,
		OpenAITimeout:        openAITimeout,
	}
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
