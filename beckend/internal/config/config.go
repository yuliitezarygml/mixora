package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr     string
	Environment  string
	DatabaseURL  string
	RedisAddr    string
	PublicURL    string
	CookieSecure bool
	SessionTTL   time.Duration
	SMTPAddr     string
	SMTPFrom     string
	SMTPUsername string
	SMTPPassword string
}

func Load() (Config, error) {
	secure, err := strconv.ParseBool(value("MIXORA_COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("MIXORA_COOKIE_SECURE: %w", err)
	}
	ttl, err := time.ParseDuration(value("MIXORA_SESSION_TTL", "720h"))
	if err != nil || ttl < time.Hour {
		return Config{}, fmt.Errorf("MIXORA_SESSION_TTL must be a duration of at least one hour")
	}

	cfg := Config{
		HTTPAddr:     value("MIXORA_HTTP_ADDR", ":8080"),
		Environment:  value("MIXORA_ENV", "development"),
		DatabaseURL:  strings.TrimSpace(os.Getenv("MIXORA_DATABASE_URL")),
		RedisAddr:    value("MIXORA_REDIS_ADDR", "127.0.0.1:6379"),
		PublicURL:    strings.TrimRight(value("MIXORA_PUBLIC_URL", "http://127.0.0.1:5174"), "/"),
		CookieSecure: secure,
		SessionTTL:   ttl,
		SMTPAddr:     value("MIXORA_SMTP_ADDR", "127.0.0.1:1025"),
		SMTPFrom:     value("MIXORA_SMTP_FROM", "Mixora <noreply@mixora.local>"),
		SMTPUsername: strings.TrimSpace(os.Getenv("MIXORA_SMTP_USERNAME")),
		SMTPPassword: os.Getenv("MIXORA_SMTP_PASSWORD"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("MIXORA_DATABASE_URL is required")
	}
	return cfg, nil
}

func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
