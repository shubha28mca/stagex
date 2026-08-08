// Package config centralizes all runtime configuration for the StageX backend.
//
// Configuration is read from environment variables so the same binary can be
// deployed to any environment (local Docker, staging, production) without code
// changes. Sensible defaults are provided for local development.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the fully-resolved application configuration.
type Config struct {
	// AppEnv is one of: development, staging, production.
	AppEnv string
	// HTTPPort is the port the API server listens on.
	HTTPPort string
	// DatabaseURL is the full Postgres connection string (pgx format).
	DatabaseURL string
	// JWTSecret signs and verifies access tokens. MUST be overridden in prod.
	JWTSecret string
	// AadhaarKey is the passphrase used to derive the AES key that encrypts
	// Aadhaar numbers at rest. MUST be overridden in prod.
	AadhaarKey string
	// JWTTTL is how long an issued access token stays valid.
	JWTTTL time.Duration
	// OTPTTL is how long a generated OTP stays valid.
	OTPTTL time.Duration
	// CORSAllowOrigin is the single frontend origin allowed to call the API.
	CORSAllowOrigin string
	// LogLevel is one of: debug, info, warn, error.
	LogLevel string
}

// Load resolves configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		AppEnv:          getenv("APP_ENV", "development"),
		HTTPPort:        getenv("HTTP_PORT", "8080"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://stagex:stagex@localhost:5432/stagex?sslmode=disable"),
		JWTSecret:       getenv("JWT_SECRET", "dev-insecure-change-me"),
		AadhaarKey:      getenv("AADHAAR_KEY", "dev-insecure-aadhaar-key"),
		JWTTTL:          getenvDuration("JWT_TTL", 30*24*time.Hour),
		OTPTTL:          getenvDuration("OTP_TTL", 5*time.Minute),
		CORSAllowOrigin: getenv("CORS_ALLOW_ORIGIN", "http://localhost:5173"),
		LogLevel:        getenv("LOG_LEVEL", "info"),
	}
}

// Validate returns an error if the configuration is unsafe for the target env.
func (c Config) Validate() error {
	if c.AppEnv == "production" && c.JWTSecret == "dev-insecure-change-me" {
		return fmt.Errorf("JWT_SECRET must be set in production")
	}
	if c.AppEnv == "production" && c.AadhaarKey == "dev-insecure-aadhaar-key" {
		return fmt.Errorf("AADHAAR_KEY must be set in production")
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
		// Accept either a Go duration ("5m") or an integer number of seconds.
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return fallback
}
