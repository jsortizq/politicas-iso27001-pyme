package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

const otherTenant = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

func setupAll() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	NewSoAHandler(NewMemorySoAStore(), logger).Register(mux)
	NewAssetHandler(NewMemoryAssetStore(), logger).Register(mux)
	NewRiskHandler(NewMemoryRiskStore(), logger).Register(mux)
	return tenant.Middleware(mux)
}

func doAs(h http.Handler, tenantID, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Tenant-ID", tenantID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decode[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

// ── Assets ──

func TestAsset_CRUD(t *testing.T) {
	h := setupAll()

	rr := doAs(h, testTenant, http.MethodPost, "/v1/assets",
		`{"name":" Base de datos de clientes ","asset_type":"information","classification":"confidential","confidentiality":5,"integrity":4,"availability":3,"owner":"Gerencia comercial"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	a := decode[Asset](t, rr)
	if a.ID == "" || a.Name != "Base de datos de clientes" || a.Criticality != 5 || a.Status != "active" {
		t.Fatalf("unexpected asset: %+v", a)
	}

	rr = doAs(h, testTenant, http.MethodPut, "/v1/assets/"+a.ID,
		`{"name":"Base de datos de clientes","asset_type":"information","confidentiality":2,"integrity":2,"availability":2,"status":"retired"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	a = decode[Asset](t, rr)
	if a.Criticality != 2 || a.Status != "retired" || a.Classification != "internal" {
		t.Errorf("unexpected updated asset: %+v", a)
	}

	if rr = doAs(h, testTenant, http.MethodDelete, "/v1/assets/"+a.ID, ""); rr.Code != http.StatusNoContent {
		t.Errorf("delete: expected 204, got %d", rr.Code)
	}
	if rr = doAs(h, testTenant, http.MethodGet, "/v1/assets/"+a.ID, ""); rr.Code != http.StatusNotFound {
		t.Errorf("get after delete: expected 404, got %d", rr.Code)
	}
}

func TestAsset_Validation(t *testing.T) {
	h := setupAll()
	cases := map[string]string{
		"missing name":       `{"asset_type":"hardware"}`,
		"bad type":           `{"name":"x","asset_type":"spaceship"}`,
		"bad classification": `{"name":"x","asset_type":"hardware","classification":"secret"}`,
		"cia out of range":   `{"name":"x","asset_type":"hardware","integrity":6}`,
		"bad status":         `{"name":"x","asset_type":"hardware","status":"lost"}`,
	}
	for name, body := range cases {
		if rr := doAs(h, testTenant, http.MethodPost, "/v1/assets", body); rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: expected 422, got %d", name, rr.Code)
		}
	}
	if rr := doAs(h, testTenant, http.MethodPost, "/v1/assets", `{not json`); rr.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON: expected 400, got %d", rr.Code)
	}
}

func TestAsset_DuplicateNameAndOrdering(t *testing.T) {
	h := setupAll()
	doAs(h, testTenant, http.MethodPost, "/v1/assets", `{"name":"Portátiles","asset_type":"hardware","availability":2}`)
	doAs(h, testTenant, http.MethodPost, "/v1/assets", `{"name":"ERP","asset_type":"software","availability":5}`)

	if rr := doAs(h, testTenant, http.MethodPost, "/v1/assets", `{"name":"ERP","asset_type":"service"}`); rr.Code != http.StatusConflict {
		t.Errorf("duplicate name: expected 409, got %d", rr.Code)
	}

	list := decode[struct{ Data []Asset }](t, doAs(h, testTenant, http.MethodGet, "/v1/assets", ""))
	if len(list.Data) != 2 || list.Data[0].Name != "ERP" {
		t.Errorf("expected ERP (most critical) first, got %+v", list.Data)
	}
}

// ── Risks ──

func TestRisk_CreateComputesLevelAndCode(t *testing.T) {
	h := setupAll()

	rr := doAs(h, testTenant, http.MethodPost, "/v1/risks",
		`{"title":"Ransomware en servidores","likelihood":4,"impact":5,"threat":"Malware","vulnerability":"Parches atrasados"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	r := decode[Risk](t, rr)
	if r.Code != "R-001" || r.Level != 20 || r.Rating != "critical" || r.Treatment != "mitigate" || r.Status != "open" {
		t.Fatalf("unexpected risk: %+v", r)
	}
	if r.ResidualLevel != nil {
		t.Errorf("residual level should be absent, got %v", *r.ResidualLevel)
	}

	r2 := decode[Risk](t, doAs(h, testTenant, http.MethodPost, "/v1/risks", `{"title":"Fuga por correo","likelihood":2,"impact":3}`))
	if r2.Code != "R-002" || r2.Rating != "medium" {
		t.Errorf("unexpected second risk: %+v", r2)
	}

	// Another tenant starts its own numbering.
	r3 := decode[Risk](t, doAs(h, otherTenant, http.MethodPost, "/v1/risks", `{"title":"Otro","likelihood":1,"impact":1}`))
	if r3.Code != "R-001" || r3.Rating != "low" {
		t.Errorf("unexpected other-tenant risk: %+v", r3)
	}
}

func TestRisk_UpdateResidualAndKeepsCode(t *testing.T) {
	h := setupAll()
	r := decode[Risk](t, doAs(h, testTenant, http.MethodPost, "/v1/risks", `{"title":"Caída del proveedor cloud","likelihood":3,"impact":4}`))

	rr := doAs(h, testTenant, http.MethodPut, "/v1/risks/"+r.ID,
		`{"title":"Caída del proveedor cloud","likelihood":3,"impact":4,"treatment":"transfer","status":"in_treatment","residual_likelihood":2,"residual_impact":2,"review_date":"2027-03-31"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	u := decode[Risk](t, rr)
	if u.Code != r.Code || u.ResidualLevel == nil || *u.ResidualLevel != 4 || u.ResidualRating != "low" || u.Level != 12 || u.Rating != "high" {
		t.Errorf("unexpected updated risk: %+v", u)
	}
}

func TestRisk_Validation(t *testing.T) {
	h := setupAll()
	cases := map[string]string{
		"missing title":         `{"likelihood":3,"impact":3}`,
		"likelihood range":      `{"title":"x","likelihood":0,"impact":3}`,
		"impact range":          `{"title":"x","likelihood":3,"impact":9}`,
		"bad treatment":         `{"title":"x","likelihood":3,"impact":3,"treatment":"ignore"}`,
		"bad status":            `{"title":"x","likelihood":3,"impact":3,"status":"done"}`,
		"half residual":         `{"title":"x","likelihood":3,"impact":3,"residual_impact":2}`,
		"residual out of range": `{"title":"x","likelihood":3,"impact":3,"residual_likelihood":7,"residual_impact":2}`,
		"bad date":              `{"title":"x","likelihood":3,"impact":3,"review_date":"31/03/2027"}`,
	}
	for name, body := range cases {
		if rr := doAs(h, testTenant, http.MethodPost, "/v1/risks", body); rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: expected 422, got %d", name, rr.Code)
		}
	}
}

func TestRisk_DuplicateCodeAndOrdering(t *testing.T) {
	h := setupAll()
	doAs(h, testTenant, http.MethodPost, "/v1/risks", `{"code":"R-010","title":"Bajo","likelihood":1,"impact":2}`)
	doAs(h, testTenant, http.MethodPost, "/v1/risks", `{"title":"Alto","likelihood":5,"impact":4}`)

	if rr := doAs(h, testTenant, http.MethodPost, "/v1/risks", `{"code":"R-010","title":"dup","likelihood":1,"impact":1}`); rr.Code != http.StatusConflict {
		t.Errorf("duplicate code: expected 409, got %d", rr.Code)
	}

	list := decode[struct{ Data []Risk }](t, doAs(h, testTenant, http.MethodGet, "/v1/risks", ""))
	if len(list.Data) != 2 || list.Data[0].Title != "Alto" || list.Data[0].Code != "R-011" {
		t.Errorf("expected highest risk first with code R-011, got %+v", list.Data)
	}
}

func TestRiskRating_Bands(t *testing.T) {
	for level, want := range map[int]string{1: "low", 4: "low", 5: "medium", 9: "medium", 10: "high", 15: "high", 16: "critical", 25: "critical"} {
		if got := RiskRating(level); got != want {
			t.Errorf("RiskRating(%d) = %s, want %s", level, got, want)
		}
	}
}

func TestRisk_ConcurrentCreatesGetDistinctCodes(t *testing.T) {
	store := NewMemoryRiskStore()
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := Risk{TenantID: testTenant, Title: "t", Likelihood: 1, Impact: 1}
			if err := store.Create(context.Background(), &r); err != nil {
				t.Errorf("create: %v", err)
			}
		}()
	}
	wg.Wait()

	risks, _ := store.List(context.Background(), testTenant)
	seen := map[string]bool{}
	for _, r := range risks {
		if seen[r.Code] {
			t.Errorf("duplicate code %s", r.Code)
		}
		seen[r.Code] = true
	}
	if len(risks) != n {
		t.Errorf("expected %d risks, got %d", n, len(risks))
	}
}

// ── Cross-tenant isolation (every resource) ──

func TestCrossTenantAccessIsDenied(t *testing.T) {
	h := setupAll()

	soa := decode[SoAEntry](t, doAs(h, testTenant, http.MethodPost, "/v1/soa", `{"control_id":"A.5.1","applicable":true}`))
	asset := decode[Asset](t, doAs(h, testTenant, http.MethodPost, "/v1/assets", `{"name":"ERP","asset_type":"software"}`))
	risk := decode[Risk](t, doAs(h, testTenant, http.MethodPost, "/v1/risks", `{"title":"x","likelihood":2,"impact":2}`))

	paths := map[string]string{
		"/v1/soa/" + soa.ID:      `{"control_id":"A.5.1","applicable":true,"implementation_pct":100}`,
		"/v1/assets/" + asset.ID: `{"name":"hijacked","asset_type":"software"}`,
		"/v1/risks/" + risk.ID:   `{"title":"hijacked","likelihood":1,"impact":1}`,
	}
	for path, body := range paths {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			if rr := doAs(h, otherTenant, method, path, body); rr.Code != http.StatusNotFound {
				t.Errorf("%s %s as other tenant: expected 404, got %d", method, path, rr.Code)
			}
		}
		// The owner still sees the record, unmodified.
		if rr := doAs(h, testTenant, http.MethodGet, path, ""); rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "hijacked") {
			t.Errorf("GET %s as owner: got %d %s", path, rr.Code, rr.Body.String())
		}
	}
	for _, list := range []string{"/v1/soa", "/v1/assets", "/v1/risks"} {
		res := decode[struct{ Count int }](t, doAs(h, otherTenant, http.MethodGet, list, ""))
		if res.Count != 0 {
			t.Errorf("GET %s as other tenant: expected 0 records, got %d", list, res.Count)
		}
	}
}
