package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr            string
	Environment         string
	DatabaseURL         string
	RedisAddr           string
	PublicURL           string
	CookieSecure        bool
	SessionTTL          time.Duration
	SMTPAddr            string
	SMTPFrom            string
	SMTPUsername        string
	SMTPPassword        string
	GorseURL            string
	GorseAPIKey         string
	GorseTimeout        time.Duration
	EmbeddingURL        string
	EmbeddingModel      string
	EmbeddingVersion    string
	EmbeddingDimensions int
	EmbeddingTimeout    time.Duration
	EmbeddingBatchSize  int
	YTDLPTimeout        time.Duration
	YTDLPMaxConcurrent  int
	YTDLPCookiesFile    string
	// Shared browser cookies are unsafe for the multi-user HTTP API: one
	// account could otherwise resolve media using another account's provider
	// session. They are allowed only for an explicitly opted-in, single-user
	// local development runtime.
	AllowSharedYTDLPCookies bool
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
	gorseTimeout, err := time.ParseDuration(value("MIXORA_GORSE_TIMEOUT", "3s"))
	if err != nil || gorseTimeout <= 0 || gorseTimeout > 30*time.Second {
		return Config{}, fmt.Errorf("MIXORA_GORSE_TIMEOUT must be between 1ns and 30s")
	}
	embeddingTimeout, err := time.ParseDuration(value("MIXORA_EMBEDDINGS_TIMEOUT", "2m"))
	if err != nil || embeddingTimeout <= 0 || embeddingTimeout > 10*time.Minute {
		return Config{}, fmt.Errorf("MIXORA_EMBEDDINGS_TIMEOUT must be between 1ns and 10m")
	}
	embeddingDimensions, err := strconv.Atoi(value("MIXORA_EMBEDDINGS_DIMENSIONS", "768"))
	if err != nil || embeddingDimensions != 768 {
		return Config{}, fmt.Errorf("MIXORA_EMBEDDINGS_DIMENSIONS must be 768")
	}
	embeddingBatchSize, err := strconv.Atoi(value("MIXORA_EMBEDDINGS_BATCH_SIZE", "16"))
	if err != nil || embeddingBatchSize < 1 || embeddingBatchSize > 128 {
		return Config{}, fmt.Errorf("MIXORA_EMBEDDINGS_BATCH_SIZE must be between 1 and 128")
	}
	ytdlpTimeout, err := time.ParseDuration(value("MIXORA_YTDLP_TIMEOUT", "30s"))
	if err != nil || ytdlpTimeout < time.Second || ytdlpTimeout > 2*time.Minute {
		return Config{}, fmt.Errorf("MIXORA_YTDLP_TIMEOUT must be between 1s and 2m")
	}
	ytdlpMaxConcurrent, err := strconv.Atoi(value("MIXORA_YTDLP_MAX_CONCURRENT", "2"))
	if err != nil || ytdlpMaxConcurrent < 1 || ytdlpMaxConcurrent > 8 {
		return Config{}, fmt.Errorf("MIXORA_YTDLP_MAX_CONCURRENT must be between 1 and 8")
	}
	allowSharedYTDLPCookies, err := strconv.ParseBool(value("MIXORA_ALLOW_SHARED_YTDLP_COOKIES", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("MIXORA_ALLOW_SHARED_YTDLP_COOKIES: %w", err)
	}

	cfg := Config{
		HTTPAddr:                value("MIXORA_HTTP_ADDR", ":8080"),
		Environment:             value("MIXORA_ENV", "development"),
		DatabaseURL:             strings.TrimSpace(os.Getenv("MIXORA_DATABASE_URL")),
		RedisAddr:               value("MIXORA_REDIS_ADDR", "127.0.0.1:6379"),
		PublicURL:               strings.TrimRight(value("MIXORA_PUBLIC_URL", "http://127.0.0.1:5174"), "/"),
		CookieSecure:            secure,
		SessionTTL:              ttl,
		SMTPAddr:                value("MIXORA_SMTP_ADDR", "127.0.0.1:1025"),
		SMTPFrom:                value("MIXORA_SMTP_FROM", "Mixora <noreply@mixora.local>"),
		SMTPUsername:            strings.TrimSpace(os.Getenv("MIXORA_SMTP_USERNAME")),
		SMTPPassword:            os.Getenv("MIXORA_SMTP_PASSWORD"),
		GorseURL:                strings.TrimRight(strings.TrimSpace(os.Getenv("MIXORA_GORSE_URL")), "/"),
		GorseAPIKey:             strings.TrimSpace(os.Getenv("MIXORA_GORSE_API_KEY")),
		GorseTimeout:            gorseTimeout,
		EmbeddingURL:            strings.TrimRight(strings.TrimSpace(os.Getenv("MIXORA_EMBEDDINGS_URL")), "/"),
		EmbeddingModel:          value("MIXORA_EMBEDDINGS_MODEL", "embeddinggemma:300m-qat-q4_0"),
		EmbeddingVersion:        value("MIXORA_EMBEDDINGS_VERSION", "embeddinggemma-q4-768-doc-v1"),
		EmbeddingDimensions:     embeddingDimensions,
		EmbeddingTimeout:        embeddingTimeout,
		EmbeddingBatchSize:      embeddingBatchSize,
		YTDLPTimeout:            ytdlpTimeout,
		YTDLPMaxConcurrent:      ytdlpMaxConcurrent,
		YTDLPCookiesFile:        strings.TrimSpace(os.Getenv("MIXORA_YTDLP_COOKIES_FILE")),
		AllowSharedYTDLPCookies: allowSharedYTDLPCookies,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("MIXORA_DATABASE_URL is required")
	}
	if cfg.YTDLPCookiesFile != "" &&
		(!cfg.AllowSharedYTDLPCookies || !strings.EqualFold(cfg.Environment, "development")) {
		return Config{}, fmt.Errorf("MIXORA_YTDLP_COOKIES_FILE is allowed only with MIXORA_ENV=development and MIXORA_ALLOW_SHARED_YTDLP_COOKIES=true; never share provider cookies in a multi-user API")
	}
	return cfg, nil
}

func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
