// Package config loads application configuration from environment variables.
//
// Every setting has a sensible default for local development.
// In production, values come from environment variables prefixed with the
// relevant section (DB_, REDIS_, S3_, etc.).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	S3       S3Config
	Auth     AuthConfig
	AI       AIConfig
	Log      LogConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	CORSOrigins     string
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int
	URL      string // full DSN override

	// MigrateURL is the administrator DSN used only by cmd/migrate.
	MigrateURL string
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL string
}

// S3Config holds object storage settings.
type S3Config struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
}

// AuthConfig holds OIDC authentication settings.
type AuthConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	SessionKey   string

	// DevMode enables the development login, which accepts any credentials.
	// It must be requested explicitly (AUTH_DEV_MODE=true) and is never
	// inferred from missing OIDC settings.
	DevMode bool
}

// AIConfig holds AI gateway settings.
type AIConfig struct {
	Provider string // "anthropic", "openai", "ollama"
	APIKey   string
	Model    string
	BaseURL  string
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string // "debug", "info", "warn", "error"
	Format string // "json", "text"
}

// Load reads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            env("SERVER_HOST", "0.0.0.0"),
			Port:            envInt("SERVER_PORT", 8080),
			ReadTimeout:     envDuration("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    envDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			ShutdownTimeout: envDuration("SERVER_SHUTDOWN_TIMEOUT", 10*time.Second),
			CORSOrigins:     env("CORS_ORIGINS", "http://localhost:3000"),
		},
		Database: DatabaseConfig{
			Host:     env("DB_HOST", "localhost"),
			Port:     envInt("DB_PORT", 5432),
			User:     env("DB_USER", "sgsi"),
			Password: env("DB_PASSWORD", "sgsi_dev"),
			DBName:   env("DB_NAME", "sgsi"),
			SSLMode:  env("DB_SSLMODE", "disable"),
			MaxConns: envInt("DB_MAX_CONNS", 20),
			URL:      env("DATABASE_URL", ""),

			MigrateURL: env("DATABASE_MIGRATE_URL", ""),
		},
		Redis: RedisConfig{
			URL: env("REDIS_URL", "redis://localhost:6379/0"),
		},
		S3: S3Config{
			Endpoint:  env("S3_ENDPOINT", "http://localhost:9000"),
			Bucket:    env("S3_BUCKET", "sgsi-documents"),
			Region:    env("S3_REGION", "us-east-1"),
			AccessKey: env("S3_ACCESS_KEY", ""),
			SecretKey: env("S3_SECRET_KEY", ""),
		},
		Auth: AuthConfig{
			Issuer:       env("OIDC_ISSUER", ""),
			ClientID:     env("OIDC_CLIENT_ID", ""),
			ClientSecret: env("OIDC_CLIENT_SECRET", ""),
			RedirectURL:  env("OIDC_REDIRECT_URL", "http://localhost:8080/auth/callback"),
			SessionKey:   env("SESSION_KEY", "dev-session-key-change-in-production"),
			DevMode:      envBool("AUTH_DEV_MODE", false),
		},
		AI: AIConfig{
			Provider: env("AI_PROVIDER", "anthropic"),
			APIKey:   env("AI_API_KEY", ""),
			Model:    env("AI_MODEL", "claude-sonnet-4-20250514"),
			BaseURL:  env("AI_BASE_URL", ""),
		},
		Log: LogConfig{
			Level:  env("LOG_LEVEL", "info"),
			Format: env("LOG_FORMAT", "json"),
		},
	}
}

// DSN returns the PostgreSQL connection string.
// If DATABASE_URL is set, it takes precedence.
func (d *DatabaseConfig) DSN() string {
	if d.URL != "" {
		return d.URL
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// Addr returns the server listen address as "host:port".
func (s *ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// ── helpers ──

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
