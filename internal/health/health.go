// Package health provides liveness and readiness probe endpoints.
//
// - /healthz: liveness probe — is the process alive?
// - /readyz:  readiness probe — can the process serve traffic?
//
// The readiness probe checks external dependencies (database, etc.)
// and can be used by load balancers to stop sending traffic during
// startup or dependency outages.
package health

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Checker is a function that reports the health of a dependency.
// It returns nil if healthy, or an error describing the problem.
type Checker func() error

// Handler serves the health check endpoints.
type Handler struct {
	logger   *slog.Logger
	mu       sync.RWMutex
	checkers map[string]Checker
}

// NewHandler creates a health handler with the given logger.
func NewHandler(logger *slog.Logger) *Handler {
	return &Handler{
		logger:   logger,
		checkers: make(map[string]Checker),
	}
}

// Register adds a named readiness checker.
// Checkers are evaluated in undefined order during readiness probes.
func (h *Handler) Register(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = checker
}

// Response is the JSON body returned by health endpoints.
type Response struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// Liveness handles GET /healthz.
// It always returns 200 OK if the process is running.
func (h *Handler) Liveness(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

// Readiness handles GET /readyz.
// It returns 200 OK only if all registered checkers pass.
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	checkers := make(map[string]Checker, len(h.checkers))
	for k, v := range h.checkers {
		checkers[k] = v
	}
	h.mu.RUnlock()

	checks := make(map[string]string, len(checkers))
	allOK := true

	for name, check := range checkers {
		if err := check(); err != nil {
			checks[name] = err.Error()
			allOK = false
			h.logger.Warn("readiness check failed", "check", name, "error", err)
		} else {
			checks[name] = "ok"
		}
	}

	status := http.StatusOK
	resp := Response{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}

	if !allOK {
		status = http.StatusServiceUnavailable
		resp.Status = "degraded"
	}

	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
