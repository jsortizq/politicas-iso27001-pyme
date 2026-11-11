package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	cfg := Load()

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("expected default db host localhost, got %s", cfg.Database.Host)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("expected default db port 5432, got %d", cfg.Database.Port)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level info, got %s", cfg.Log.Level)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("DB_HOST", "db.example.com")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg := Load()

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("expected db host db.example.com, got %s", cfg.Database.Host)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Log.Level)
	}
}

func TestDSN_Default(t *testing.T) {
	cfg := Load()
	dsn := cfg.Database.DSN()

	expected := "host=localhost port=5432 user=sgsi password=sgsi_dev dbname=sgsi sslmode=disable"
	if dsn != expected {
		t.Errorf("expected DSN %q, got %q", expected, dsn)
	}
}

func TestDSN_URLOverride(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")
	defer os.Unsetenv("DATABASE_URL")

	cfg := Load()
	dsn := cfg.Database.DSN()

	if dsn != "postgres://user:pass@host:5432/db" {
		t.Errorf("expected DATABASE_URL override, got %q", dsn)
	}
}

func TestAddr(t *testing.T) {
	cfg := Load()
	addr := cfg.Server.Addr()

	if addr != "0.0.0.0:8080" {
		t.Errorf("expected 0.0.0.0:8080, got %s", addr)
	}
}

func TestEnvDuration(t *testing.T) {
	os.Setenv("SERVER_READ_TIMEOUT", "30s")
	defer os.Unsetenv("SERVER_READ_TIMEOUT")

	cfg := Load()

	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected 30s, got %v", cfg.Server.ReadTimeout)
	}
}

func TestEnvInt_InvalidFallback(t *testing.T) {
	os.Setenv("SERVER_PORT", "not_a_number")
	defer os.Unsetenv("SERVER_PORT")

	cfg := Load()

	if cfg.Server.Port != 8080 {
		t.Errorf("expected fallback 8080 for invalid int, got %d", cfg.Server.Port)
	}
}
