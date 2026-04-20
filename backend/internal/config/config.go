// Package config centralizes all environment variable reading for the Phaeton server.
// Call Load() once at startup; the returned Config is then passed to subsystem constructors
// via dependency injection. No other package should call os.Getenv for configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all environment-driven settings for the Phaeton server.
type Config struct {
	// Env is the GO_ENV value ("production" enables stricter defaults).
	Env    string
	IsProd bool

	// NoColor disables colored console output when set.
	NoColor bool

	// Addr is the HTTP listen address (default ":8080").
	Addr string

	// JWTSecret is the HMAC key for signing/verifying JWTs.
	// Required in production; defaults to a dev-only secret otherwise.
	JWTSecret string

	// AuthDisabled bypasses JWT validation (dev only, ignored in production).
	AuthDisabled bool

	// CORSOrigin is a comma-separated list of allowed origins.
	CORSOrigin string

	// WebhookSecret enables HMAC-SHA256 verification for incoming webhooks.
	WebhookSecret string

	DB       DBConfig
	AI       AIConfig
	SMTP     *SMTPConfig     // nil = email disabled
	SAML     *SAMLConfig     // nil = SAML disabled
	Amaranth *AmaranthConfig // nil = sync disabled
}

// DBConfig holds PostgreSQL connection pool settings.
type DBConfig struct {
	URL                string
	MaxConns           int
	MinConns           int
	StatementTimeoutMS int
}

// AIConfig holds vLLM client settings.
type AIConfig struct {
	BaseURL string
	Model   string // empty = auto-detect from vLLM
}

// SMTPConfig holds email delivery settings.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// SAMLConfig holds SAML 2.0 SP settings.
type SAMLConfig struct {
	EntityID       string
	RootURL        string
	CertPath       string
	KeyPath        string
	IdPMetadataURL string
}

// AmaranthConfig holds Amaranth HR sync settings.
type AmaranthConfig struct {
	BaseURL string
	APIKey  string
}

// Load reads all configuration from environment variables and returns a Config.
// Returns an error if required values are missing in production.
func Load() (*Config, error) {
	env := os.Getenv("GO_ENV")
	isProd := env == "production"

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		if isProd {
			return nil, fmt.Errorf("JWT_SECRET environment variable is required in production")
		}
		jwtSecret = "phaeton-dev-secret-change-in-production"
	}

	authDisabled := false
	if !isProd {
		authDisabled = strings.EqualFold(os.Getenv("AUTH_DISABLED"), "true")
	}

	cfg := &Config{
		Env:           env,
		IsProd:        isProd,
		NoColor:       os.Getenv("NO_COLOR") != "",
		Addr:          envOr("ADDR", ":8080"),
		JWTSecret:     jwtSecret,
		AuthDisabled:  authDisabled,
		CORSOrigin:    os.Getenv("CORS_ORIGIN"),
		WebhookSecret: os.Getenv("WEBHOOK_SECRET"),

		DB: DBConfig{
			URL:                envOr("DATABASE_URL", "postgres://phaeton:phaeton@localhost:5432/phaeton?sslmode=disable"),
			MaxConns:           envInt("DB_MAX_CONNS", 50),
			MinConns:           envInt("DB_MIN_CONNS", 5),
			StatementTimeoutMS: envInt("DB_STATEMENT_TIMEOUT_MS", 30000),
		},

		AI: AIConfig{
			BaseURL: envOr("AI_BASE_URL", "http://localhost:8000"),
			Model:   os.Getenv("AI_MODEL"),
		},
	}

	// SMTP (optional).
	if host := os.Getenv("SMTP_HOST"); host != "" {
		cfg.SMTP = &SMTPConfig{
			Host:     host,
			Port:     envOr("SMTP_PORT", "587"),
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("SMTP_FROM"),
		}
	}

	// SAML (optional).
	if idpURL := os.Getenv("SAML_IDP_METADATA_URL"); idpURL != "" {
		cfg.SAML = &SAMLConfig{
			EntityID:       envOr("SAML_ENTITY_ID", "phaeton"),
			RootURL:        envOr("SAML_ROOT_URL", "http://localhost:8080"),
			CertPath:       envOr("SAML_CERT_PATH", "saml/sp.crt"),
			KeyPath:        envOr("SAML_KEY_PATH", "saml/sp.key"),
			IdPMetadataURL: idpURL,
		}
	}

	// Amaranth HR sync (optional).
	if os.Getenv("AMARANTH_SYNC_ENABLED") == "true" {
		cfg.Amaranth = &AmaranthConfig{
			BaseURL: os.Getenv("AMARANTH_API_URL"),
			APIKey:  os.Getenv("AMARANTH_API_KEY"),
		}
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
