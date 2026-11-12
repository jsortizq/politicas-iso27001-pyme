// Package server provides the HTTP server and routing for the SGSI API.
//
// It wires together all HTTP handlers, middleware, and graceful shutdown.
// The router uses Go 1.22+ net/http routing with method patterns.
package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/api"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/config"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/directory"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/health"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// Server wraps the HTTP server with configuration and dependencies.
type Server struct {
	cfg    *config.Config
	logger *slog.Logger
	db     *sql.DB // nil: in-memory stores (development without a database)
	http   *http.Server
}

// New creates a Server with the given configuration and logger.
// With a nil db the server keeps data in memory, which is lost on restart;
// with a db every tenant-scoped query goes through Row-Level Security.
func New(cfg *config.Config, logger *slog.Logger, db *sql.DB) *Server {
	s := &Server{
		cfg:    cfg,
		logger: logger,
		db:     db,
	}

	mux := s.routes()

	s.http = &http.Server{
		Addr:         cfg.Server.Addr(),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	return s
}

// routes builds the HTTP router.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// ── Public routes (no tenant context required) ──
	healthHandler := health.NewHandler(s.logger)
	mux.HandleFunc("GET /healthz", healthHandler.Liveness)
	mux.HandleFunc("GET /readyz", healthHandler.Readiness)

	if s.db != nil {
		healthHandler.Register("database", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return s.db.PingContext(ctx)
		})
	}

	// ── Stores: PostgreSQL when configured, otherwise in memory ──
	var (
		members    directory.Store = directory.NewMemoryStore()
		soaStore   api.SoAStore    = api.NewMemorySoAStore()
		assetStore api.AssetStore  = api.NewMemoryAssetStore()
		riskStore  api.RiskStore   = api.NewMemoryRiskStore()
		auditStore api.AuditStore  // the audit trail is only recorded in PostgreSQL
	)
	if s.db != nil {
		members = directory.NewPgStore(s.db)
		soaStore = api.NewPgSoAStore(s.db)
		assetStore = api.NewPgAssetStore(s.db)
		riskStore = api.NewPgRiskStore(s.db)
		auditStore = api.NewPgAuditStore(s.db)
	}

	// ── Auth routes (no tenant context) ──
	sessions := auth.NewSessionStore(8 * time.Hour)
	authHandler := auth.NewHandler(sessions, members, auth.Options{
		DevMode: s.cfg.Auth.DevMode,
		OIDC: auth.OIDCConfig{
			Issuer:       s.cfg.Auth.Issuer,
			ClientID:     s.cfg.Auth.ClientID,
			ClientSecret: s.cfg.Auth.ClientSecret,
			RedirectURL:  s.cfg.Auth.RedirectURL,
			Scopes:       s.cfg.Auth.Scopes,
		},
		AppURL:        s.cfg.Auth.AppURL,
		SecureCookies: strings.HasPrefix(s.cfg.Auth.AppURL, "https://"),
		AllowSignup:   s.cfg.Auth.AllowSignup,
		Logger:        s.logger,
	})
	authHandler.Register(mux)

	// ── API v1 routes (authenticated, tenant-scoped) ──
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /v1/info", s.handleInfo)

	api.NewSoAHandler(soaStore, s.logger).Register(apiMux)
	api.NewAssetHandler(assetStore, s.logger).Register(apiMux)
	api.NewRiskHandler(riskStore, s.logger).Register(apiMux)
	api.NewAuditHandler(auditStore, s.logger).Register(apiMux)
	directory.NewHandler(members, s.logger).Register(apiMux)

	// Order matters: the session decides the tenant (auth overwrites the
	// X-Tenant-ID header), then the role decides whether writes are allowed.
	protected := authHandler.Middleware(tenant.Middleware(requireWriteRole(apiMux)))
	apiHandler := http.StripPrefix("/api", protected)
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.Handle(method+" /api/", apiHandler)
	}

	// ── Root handler ──
	mux.HandleFunc("GET /", s.handleRoot)

	// Apply global middleware
	var handler http.Handler = mux
	handler = s.recoverMiddleware(handler)
	handler = s.originMiddleware(handler)
	handler = s.corsMiddleware(handler)
	handler = s.requestLogMiddleware(handler)

	return handler
}

// Start runs the HTTP server and blocks until a shutdown signal is received.
func (s *Server) Start() error {
	// Channel for shutdown signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("server starting",
			"addr", s.http.Addr,
			"read_timeout", s.cfg.Server.ReadTimeout,
			"write_timeout", s.cfg.Server.WriteTimeout,
		)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		s.logger.Info("shutdown signal received", "signal", sig.String())
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	s.logger.Info("server stopped gracefully")
	return nil
}

// ── Handlers ──

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"service":"sgsi-api","version":"0.1.0","docs":"/api/v1/info"}`)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	tid := tenant.FromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"api":"v1","tenant":"%s","status":"ok"}`, tid)
}

// ── Middleware ──

// requireWriteRole rejects state-changing requests from read-only roles
// (auditor, viewer). It must run after auth.Middleware.
func requireWriteRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !auth.FromContext(r.Context()).CanWrite() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"your role does not allow changes"}`)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// originMiddleware rejects state-changing requests that a browser sends
// from another site. The session cookie is SameSite=Lax, which already
// keeps it off cross-site POST/PUT/DELETE; this is a second, independent
// check. Requests without an Origin header (CLI tools, server-to-server)
// are not affected.
func (s *Server) originMiddleware(next http.Handler) http.Handler {
	allowed := map[string]bool{strings.TrimRight(s.cfg.Auth.AppURL, "/"): true}
	for _, origin := range strings.Split(s.cfg.Server.CORSOrigins, ",") {
		if origin = strings.TrimRight(strings.TrimSpace(origin), "/"); origin != "" {
			allowed[origin] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin := r.Header.Get("Origin"); origin != "" && !allowed[strings.TrimRight(origin, "/")] {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"request origin not allowed"}`)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.cfg.Server.CORSOrigins)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Tenant-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.Error("panic recovered",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
				)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
