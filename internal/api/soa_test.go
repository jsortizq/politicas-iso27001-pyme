package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

const testTenant = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

func setupHandler() (*SoAHandler, *http.ServeMux) {
	store := NewMemorySoAStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewSoAHandler(store, logger)

	mux := http.NewServeMux()
	h.Register(mux)

	return h, mux
}

func withTenant(mux http.Handler) http.Handler {
	return tenant.Middleware(mux)
}

func doRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("X-Tenant-ID", testTenant)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestCreateSoAEntry(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	body := `{
		"control_id": "A.5.1",
		"applicable": true,
		"justification": "Required for SGSI framework",
		"implementation_pct": 80,
		"implementation_evidence": "Policy document published",
		"effectiveness_pct": 70,
		"effectiveness_evidence": "Annual review completed",
		"responsible_role": "CISO"
	}`

	rr := doRequest(handler, http.MethodPost, "/v1/soa", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /v1/soa: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var result SoAEntry
	json.NewDecoder(rr.Body).Decode(&result)

	if result.ID == "" {
		t.Error("expected non-empty ID")
	}
	if result.ControlID != "A.5.1" {
		t.Errorf("expected control_id A.5.1, got %s", result.ControlID)
	}
	if result.ImplementationPct != 80 {
		t.Errorf("expected impl 80, got %d", result.ImplementationPct)
	}
}

func TestListSoAEntries(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	// Create two entries
	doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.5.1","applicable":true,"implementation_pct":100,"effectiveness_pct":90}`)
	doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.5.2","applicable":false,"justification":"Fuera del alcance","implementation_pct":0,"effectiveness_pct":0}`)

	rr := doRequest(handler, http.MethodGet, "/v1/soa", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v1/soa: expected 200, got %d", rr.Code)
	}

	var result map[string]any
	json.NewDecoder(rr.Body).Decode(&result)

	count := int(result["count"].(float64))
	if count != 2 {
		t.Errorf("expected 2 entries, got %d", count)
	}
}

func TestGetSoAEntry(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	// Create an entry
	rr := doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.6.1","applicable":true,"implementation_pct":50,"effectiveness_pct":40}`)

	var created SoAEntry
	json.NewDecoder(rr.Body).Decode(&created)

	// Get it
	rr = doRequest(handler, http.MethodGet, "/v1/soa/"+created.ID, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v1/soa/%s: expected 200, got %d", created.ID, rr.Code)
	}

	var fetched SoAEntry
	json.NewDecoder(rr.Body).Decode(&fetched)

	if fetched.ControlID != "A.6.1" {
		t.Errorf("expected A.6.1, got %s", fetched.ControlID)
	}
}

func TestGetSoAEntry_NotFound(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	rr := doRequest(handler, http.MethodGet, "/v1/soa/nonexistent", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET nonexistent: expected 404, got %d", rr.Code)
	}
}

func TestUpdateSoAEntry(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	// Create
	rr := doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.7.1","applicable":true,"implementation_pct":30,"effectiveness_pct":20}`)

	var created SoAEntry
	json.NewDecoder(rr.Body).Decode(&created)

	// Update
	rr = doRequest(handler, http.MethodPut, "/v1/soa/"+created.ID,
		`{"control_id":"A.7.1","applicable":true,"implementation_pct":90,"effectiveness_pct":85,"justification":"Updated assessment"}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated SoAEntry
	json.NewDecoder(rr.Body).Decode(&updated)

	if updated.ImplementationPct != 90 {
		t.Errorf("expected impl 90, got %d", updated.ImplementationPct)
	}
	if updated.ID != created.ID {
		t.Errorf("ID should be preserved: expected %s, got %s", created.ID, updated.ID)
	}
}

func TestDeleteSoAEntry(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	// Create
	rr := doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.8.1","applicable":true,"implementation_pct":100,"effectiveness_pct":100}`)

	var created SoAEntry
	json.NewDecoder(rr.Body).Decode(&created)

	// Delete
	rr = doRequest(handler, http.MethodDelete, "/v1/soa/"+created.ID, "")

	if rr.Code != http.StatusNoContent {
		t.Errorf("DELETE: expected 204, got %d", rr.Code)
	}

	// Confirm gone
	rr = doRequest(handler, http.MethodGet, "/v1/soa/"+created.ID, "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET after delete: expected 404, got %d", rr.Code)
	}
}

func TestCreateSoAEntry_InvalidControlID(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	rr := doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"INVALID","applicable":true,"implementation_pct":50,"effectiveness_pct":50}`)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rr.Code)
	}
}

func TestCreateSoAEntry_InvalidPct(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	rr := doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.5.1","applicable":true,"implementation_pct":150,"effectiveness_pct":50}`)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for pct > 100, got %d", rr.Code)
	}
}

func TestCreateSoAEntry_InvalidJSON(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	rr := doRequest(handler, http.MethodPost, "/v1/soa", "not json")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSoASummary(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	// Create mix of entries
	doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.5.1","applicable":true,"implementation_pct":100,"effectiveness_pct":90}`)
	doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.5.2","applicable":true,"implementation_pct":50,"effectiveness_pct":40}`)
	doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.5.3","applicable":false,"justification":"Fuera del alcance","implementation_pct":0,"effectiveness_pct":0}`)

	rr := doRequest(handler, http.MethodGet, "/v1/soa/summary", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v1/soa/summary: expected 200, got %d", rr.Code)
	}

	var summary SoASummary
	json.NewDecoder(rr.Body).Decode(&summary)

	if summary.TotalControls != 3 {
		t.Errorf("expected 3 total, got %d", summary.TotalControls)
	}
	if summary.Applicable != 2 {
		t.Errorf("expected 2 applicable, got %d", summary.Applicable)
	}
	if summary.NotApplicable != 1 {
		t.Errorf("expected 1 not applicable, got %d", summary.NotApplicable)
	}
	if summary.Implemented != 1 {
		t.Errorf("expected 1 implemented, got %d", summary.Implemented)
	}
	if summary.Partial != 1 {
		t.Errorf("expected 1 partial, got %d", summary.Partial)
	}
}

func TestSoASummary_Empty(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	rr := doRequest(handler, http.MethodGet, "/v1/soa/summary", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var summary SoASummary
	json.NewDecoder(rr.Body).Decode(&summary)

	if summary.TotalControls != 0 {
		t.Errorf("expected 0 total, got %d", summary.TotalControls)
	}
}

func TestTenantIsolation(t *testing.T) {
	_, mux := setupHandler()

	tenantA := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	tenantB := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	doRequestAs := func(tid, method, path, body string) *httptest.ResponseRecorder {
		var bodyReader io.Reader
		if body != "" {
			bodyReader = bytes.NewBufferString(body)
		}
		req := httptest.NewRequest(method, path, bodyReader)
		req.Header.Set("X-Tenant-ID", tid)
		rr := httptest.NewRecorder()
		tenant.Middleware(mux).ServeHTTP(rr, req)
		return rr
	}

	// Create entry for tenant A
	doRequestAs(tenantA, http.MethodPost, "/v1/soa",
		`{"control_id":"A.5.1","applicable":true,"implementation_pct":100,"effectiveness_pct":100}`)

	// Tenant B should see nothing
	rr := doRequestAs(tenantB, http.MethodGet, "/v1/soa", "")
	var result map[string]any
	json.NewDecoder(rr.Body).Decode(&result)

	count := int(result["count"].(float64))
	if count != 0 {
		t.Errorf("tenant B should see 0 entries, got %d", count)
	}

	// Tenant A should see 1
	rr = doRequestAs(tenantA, http.MethodGet, "/v1/soa", "")
	json.NewDecoder(rr.Body).Decode(&result)

	count = int(result["count"].(float64))
	if count != 1 {
		t.Errorf("tenant A should see 1 entry, got %d", count)
	}
}

func TestValidateControlID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"A.5.1", true},
		{"A.8.13", true},
		{"A.12.1", false}, // well-formed but not an Annex A:2022 control
		{"B.5.1", false},
		{"A.5", false},
		{"5.1", false},
		{"", false},
	}

	for _, tt := range tests {
		e := &SoAEntry{ControlID: tt.id, Applicable: true, ImplementationPct: 50, EffectivenessPct: 50}
		err := validateSoAEntry(e)
		if tt.valid && err != nil {
			t.Errorf("validateSoAEntry(%q) unexpected error: %v", tt.id, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("validateSoAEntry(%q) expected error", tt.id)
		}
	}
}

func TestCreateSoAEntry_DuplicateControl(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)
	body := `{"control_id":"A.5.1","applicable":true}`

	if rr := doRequest(handler, http.MethodPost, "/v1/soa", body); rr.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", rr.Code)
	}
	if rr := doRequest(handler, http.MethodPost, "/v1/soa", body); rr.Code != http.StatusConflict {
		t.Errorf("duplicate control: expected 409, got %d", rr.Code)
	}
}

func TestCreateSoAEntry_NotApplicableNeedsJustification(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	rr := doRequest(handler, http.MethodPost, "/v1/soa", `{"control_id":"A.7.1","applicable":false}`)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rr.Code)
	}
}

func TestInitializeSoA(t *testing.T) {
	_, mux := setupHandler()
	handler := withTenant(mux)

	// One control assessed beforehand must survive initialization untouched.
	doRequest(handler, http.MethodPost, "/v1/soa",
		`{"control_id":"A.5.1","applicable":true,"implementation_pct":70}`)

	rr := doRequest(handler, http.MethodPost, "/v1/soa/initialize", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("initialize: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var res map[string]int
	json.NewDecoder(rr.Body).Decode(&res)
	if res["created"] != 92 || res["total"] != 93 {
		t.Errorf("expected 92 created / 93 total, got %v", res)
	}

	// Idempotent
	rr = doRequest(handler, http.MethodPost, "/v1/soa/initialize", "")
	json.NewDecoder(rr.Body).Decode(&res)
	if res["created"] != 0 || res["total"] != 93 {
		t.Errorf("second run: expected 0 created / 93 total, got %v", res)
	}

	rr = doRequest(handler, http.MethodGet, "/v1/soa", "")
	var list struct {
		Data []SoAEntry `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Data) != 93 {
		t.Fatalf("expected 93 entries, got %d", len(list.Data))
	}
	first := list.Data[0]
	if first.ControlID != "A.5.1" || first.ImplementationPct != 70 || first.ControlTitle == "" {
		t.Errorf("first entry should be the pre-assessed A.5.1 with a title, got %+v", first)
	}
	// Natural order: A.5.9 comes before A.5.10
	if list.Data[8].ControlID != "A.5.9" || list.Data[9].ControlID != "A.5.10" {
		t.Errorf("unexpected order: %s, %s", list.Data[8].ControlID, list.Data[9].ControlID)
	}
}
