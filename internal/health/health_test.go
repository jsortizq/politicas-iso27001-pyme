package health

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLiveness_AlwaysOK(t *testing.T) {
	h := NewHandler(testLogger())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	h.Liveness(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("expected status ok in body, got %s", body)
	}
}

func TestReadiness_NoCheckers(t *testing.T) {
	h := NewHandler(testLogger())
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	h.Readiness(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReadiness_AllPass(t *testing.T) {
	h := NewHandler(testLogger())
	h.Register("db", func() error { return nil })
	h.Register("redis", func() error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	h.Readiness(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("expected status ok, got %s", body)
	}
}

func TestReadiness_OneFails(t *testing.T) {
	h := NewHandler(testLogger())
	h.Register("db", func() error { return fmt.Errorf("connection refused") })
	h.Register("redis", func() error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	h.Readiness(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"status":"degraded"`) {
		t.Errorf("expected status degraded, got %s", body)
	}
	if !strings.Contains(body, "connection refused") {
		t.Errorf("expected error message in checks, got %s", body)
	}
}

func TestRegister_AddsChecker(t *testing.T) {
	h := NewHandler(testLogger())
	if len(h.checkers) != 0 {
		t.Fatalf("expected 0 checkers, got %d", len(h.checkers))
	}

	h.Register("test", func() error { return nil })

	if len(h.checkers) != 1 {
		t.Errorf("expected 1 checker, got %d", len(h.checkers))
	}
}
