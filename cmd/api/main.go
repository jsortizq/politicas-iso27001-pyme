// Command api starts the SGSI platform HTTP server.
//
// Usage:
//
//	SERVER_PORT=8080 DB_HOST=localhost ./api
//
// See internal/config for all available environment variables.
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/config"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/server"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/storage"
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

	if cfg.Auth.DevMode {
		logger.Warn("AUTH_DEV_MODE is on: login accepts any credentials; never use in production")
	}

	// Connect to PostgreSQL when configured; otherwise keep data in memory.
	var db *sql.DB
	if cfg.Database.URL != "" {
		var err error
		db, err = database.Open(context.Background(), cfg.Database.URL, cfg.Database.MaxConns)
		if err != nil {
			logger.Error("database connection failed", "error", err)
			os.Exit(1)
		}
		defer db.Close()
		// Refuse to run with a role that would silently disable tenant isolation.
		if err := database.CheckRLSEnforced(context.Background(), db); err != nil {
			logger.Error("unsafe database role", "error", err)
			os.Exit(1)
		}
		logger.Info("using PostgreSQL storage with row-level security")
	} else {
		logger.Warn("DATABASE_URL not set: using in-memory storage, data is lost on restart")
	}

	// Uploaded files: S3-compatible storage when configured, local disk otherwise.
	var files storage.Store
	if cfg.S3.AccessKey != "" {
		s3, err := storage.NewS3(storage.S3Config{
			Endpoint:  cfg.S3.Endpoint,
			Region:    cfg.S3.Region,
			Bucket:    cfg.S3.Bucket,
			AccessKey: cfg.S3.AccessKey,
			SecretKey: cfg.S3.SecretKey,
			PathStyle: cfg.S3.PathStyle,
		})
		if err != nil {
			logger.Error("invalid S3 configuration", "error", err)
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err = s3.EnsureBucket(ctx)
		cancel()
		if err != nil {
			logger.Error("object storage is not usable", "error", err)
			os.Exit(1)
		}
		logger.Info("storing files in S3-compatible storage", "endpoint", cfg.S3.Endpoint, "bucket", cfg.S3.Bucket)
		files = s3
	} else {
		dir, err := storage.NewDir(cfg.Storage.Dir)
		if err != nil {
			logger.Error("cannot use the local storage directory", "error", err)
			os.Exit(1)
		}
		logger.Warn("S3_ACCESS_KEY not set: storing files on local disk", "dir", cfg.Storage.Dir)
		files = dir
	}

	// Create and start the server
	srv := server.New(cfg, logger, db, files)
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
