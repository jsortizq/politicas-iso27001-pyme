package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/config"
)

func testServer() *Server {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(cfg, logger)
}

func TestHealthz(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /healthz: expected 200, got %d", rr.Code)
	}
}

func TestReadyz(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /readyz: expected 200, got %d", rr.Code)
	}
}

func TestRoot(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /: expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "sgsi-api") {
		t.Errorf("expected sgsi-api in body, got %s", body)
	}
}

func TestAPIInfo_WithTenant(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	req.Header.Set("X-Tenant-ID", "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /api/v1/info: expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "a1b2c3d4-e5f6-7890-abcd-ef1234567890") {
		t.Errorf("expected tenant ID in body, got %s", body)
	}
}

func TestAPIInfo_WithoutTenant(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("GET /api/v1/info without tenant: expected 403, got %d", rr.Code)
	}
}

func TestCORS_Preflight(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/info", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("OPTIONS: expected 204, got %d", rr.Code)
	}
	if h := rr.Header().Get("Access-Control-Allow-Methods"); h == "" {
		t.Error("expected CORS headers in preflight response")
	}
}

func TestRecoverMiddleware(t *testing.T) {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{cfg: cfg, logger: logger}

	// Handler that panics
	panicker := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := s.recoverMiddleware(panicker)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", rr.Code)
	}
}
