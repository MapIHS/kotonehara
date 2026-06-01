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
	DBURL                string
	Owners               []string
	Cooldown             time.Duration
	AdminTTL             time.Duration
	DisableContactImport bool
	BASEApiURL           string
	BASES3URL            string
	MemeHost             string
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

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))

	owners := parseCSV(os.Getenv("OWNER"))

	cd := parseDurationOrSeconds(os.Getenv("COOLDOWN"), 3*time.Second)
	adminTTL := parseDurationOrSeconds(os.Getenv("ADMIN_TTL"), 45*time.Second)
	disableContactImport := parseBoolDefault(os.Getenv("DISABLE_CONTACT_IMPORT"), true)

	baseurl := strings.TrimSpace(os.Getenv("BASEAPI_URL"))

	bases3url := strings.TrimSpace(os.Getenv("BASES3_URL"))
	memehost := strings.TrimSpace(os.Getenv("MEMEHOST_URL"))

	return Config{
		AppEnv:               env,
		Prefix:               prefix,
		DBURL:                dbURL,
		Owners:               owners,
		Cooldown:             cd,
		AdminTTL:             adminTTL,
		DisableContactImport: disableContactImport,
		BASEApiURL:           baseurl,
		BASES3URL:            bases3url,
		MemeHost:             memehost,
	}
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
