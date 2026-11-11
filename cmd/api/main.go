// Command api starts the SGSI platform HTTP server.
//
// Usage:
//
//	SERVER_PORT=8080 DB_HOST=localhost ./api
//
// See internal/config for all available environment variables.
package main

import (
	"log/slog"
	"os"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/config"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/server"
)

func main() {
	// Load configuration from environment
	cfg := config.Load()

	// Set up structured logger
	var handler slog.Handler
	if cfg.Log.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: parseLogLevel(cfg.Log.Level),
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: parseLogLevel(cfg.Log.Level),
		})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	logger.Info("SGSI API starting",
		"version", "0.1.0",
		"port", cfg.Server.Port,
		"log_level", cfg.Log.Level,
	)

	// Create and start the server
	srv := server.New(cfg, logger)
	if err := srv.Start(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
