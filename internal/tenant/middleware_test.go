package tenant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testTenantA = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	testTenantB = "b2c3d4e5-f6a7-8901-bcde-f12345678901"
)

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{testTenantA, true},
		{"00000000-0000-0000-0000-000000000000", true},
		{"A1B2C3D4-E5F6-7890-ABCD-EF1234567890", true}, // uppercase OK
		{"not-a-uuid", false},
		{"", false},
		{"a1b2c3d4e5f678900abcdef123456789", false}, // no dashes
		{"a1b2c3d4-e5f6-7890-abcd-ef123456789", false}, // too short
	}

	for _, tt := range tests {
		got := IsValidUUID(tt.input)
		if got != tt.valid {
			t.Errorf("IsValidUUID(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

func TestFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	id := FromContext(ctx)
	if id != "" {
		t.Errorf("expected empty for empty context, got %q", id)
	}
}

func TestWithTenantID_Roundtrip(t *testing.T) {
	ctx := WithTenantID(context.Background(), testTenantA)
	got := FromContext(ctx)
	if got != testTenantA {
		t.Errorf("expected %s, got %s", testTenantA, got)
	}
}

func TestMiddleware_ValidHeader(t *testing.T) {
	var captured string

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/controls", nil)
	req.Header.Set("X-Tenant-ID", testTenantA)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if captured != testTenantA {
		t.Errorf("expected tenant %s, got %s", testTenantA, captured)
	}
}

func TestMiddleware_MissingHeader(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without tenant")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/controls", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestMiddleware_InvalidUUID(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with invalid UUID")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/controls", nil)
	req.Header.Set("X-Tenant-ID", "not-a-uuid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestMiddleware_IsolationBetweenRequests(t *testing.T) {
	results := make([]string, 0, 2)

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results = append(results, FromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	// Request from tenant A
	reqA := httptest.NewRequest(http.MethodGet, "/api/controls", nil)
	reqA.Header.Set("X-Tenant-ID", testTenantA)
	handler.ServeHTTP(httptest.NewRecorder(), reqA)

	// Request from tenant B
	reqB := httptest.NewRequest(http.MethodGet, "/api/controls", nil)
	reqB.Header.Set("X-Tenant-ID", testTenantB)
	handler.ServeHTTP(httptest.NewRecorder(), reqB)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0] != testTenantA {
		t.Errorf("request A: expected %s, got %s", testTenantA, results[0])
	}
	if results[1] != testTenantB {
		t.Errorf("request B: expected %s, got %s", testTenantB, results[1])
	}
	if results[0] == results[1] {
		t.Error("tenant IDs should be different between requests")
	}
}

func TestSetOnTx_NoContext(t *testing.T) {
	ctx := context.Background()
	err := SetOnTx(ctx, nil)
	if err == nil {
		t.Error("expected error when no tenant_id in context")
	}
}

func TestMiddleware_UppercaseUUID(t *testing.T) {
	upper := "A1B2C3D4-E5F6-7890-ABCD-EF1234567890"
	var captured string

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/controls", nil)
	req.Header.Set("X-Tenant-ID", upper)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	// Should be normalized to lowercase
	if captured != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("expected lowercase UUID, got %s", captured)
	}
}
