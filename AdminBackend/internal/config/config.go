// Package config centralizes runtime configuration for the StageX Admin API.
// It is a deliberately separate service from the participant API — different
// binary, different port, its own JWT secret — but points at the SAME database.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config is the resolved admin-service configuration.
type Config struct {
	AppEnv          string
	HTTPPort        string
	DatabaseURL     string
	JWTSecret       string
	JWTTTL          time.Duration
	CORSAllowOrigin string
	LogLevel        string
	// MediaRoot is the local directory where per-event media folders live. A
	// cloud object store can replace this later without touching callers.
	MediaRoot string
	// MediaPublicBaseURL is the base URL under which /media/... is served, used
	// to build absolute media links the participant app can load.
	MediaPublicBaseURL string
}

// Load resolves configuration from the environment with dev-friendly defaults.
func Load() Config {
	return Config{
		AppEnv:          getenv("APP_ENV", "development"),
		HTTPPort:        getenv("HTTP_PORT", "8081"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://stagex:stagex@localhost:5432/stagex?sslmode=disable"),
		JWTSecret:       getenv("JWT_SECRET", "dev-admin-insecure-change-me"),
		JWTTTL:          getenvDuration("JWT_TTL", 12*time.Hour),
		CORSAllowOrigin: getenv("CORS_ALLOW_ORIGIN", "http://localhost:5174"),
		LogLevel:        getenv("LOG_LEVEL", "info"),
		MediaRoot:          getenv("MEDIA_ROOT", "./media"),
		MediaPublicBaseURL: getenv("MEDIA_PUBLIC_BASE_URL", "http://localhost:8081"),
	}
}

// Validate refuses insecure defaults in production.
func (c Config) Validate() error {
	if c.AppEnv == "production" && c.JWTSecret == "dev-admin-insecure-change-me" {
		return fmt.Errorf("JWT_SECRET must be set in production")
	}
	return nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
