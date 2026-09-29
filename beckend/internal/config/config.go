package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	SoundCloudClientID, SoundCloudClientSecret, TokenEncryptionKey string
	Addr, DatabaseURL, MediaDir, AllowedOrigin                     string
	SecureCookies                                                  bool
}

func Load() (Config, error) {
	c := Config{Addr: env("HTTP_ADDR", "127.0.0.1:8080"), DatabaseURL: os.Getenv("DATABASE_URL"), MediaDir: env("MEDIA_DIR", "storage"), AllowedOrigin: env("ALLOWED_ORIGIN", "http://localhost:5173")}
	c.SoundCloudClientID = os.Getenv("SOUNDCLOUD_CLIENT_ID")
	c.SoundCloudClientSecret = os.Getenv("SOUNDCLOUD_CLIENT_SECRET")
	c.TokenEncryptionKey = os.Getenv("TOKEN_ENCRYPTION_KEY")
	if (c.SoundCloudClientID == "") != (c.SoundCloudClientSecret == "") {
		return c, fmt.Errorf("set both SOUNDCLOUD_CLIENT_ID and SOUNDCLOUD_CLIENT_SECRET")
	}
	var err error
	c.SecureCookies, err = strconv.ParseBool(env("COOKIE_SECURE", "false"))
	if err != nil {
		return c, fmt.Errorf("COOKIE_SECURE must be true or false")
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	return c, nil
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
