package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database/dbtest"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

const (
	pgTenantA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	pgTenantB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

// setupPg wires the real handlers to PostgreSQL stores using the
// RLS-restricted application connection.
func setupPg(t *testing.T) (http.Handler, dbtest.DBs) {
	dbs := dbtest.New(t)
	dbtest.Tenant(t, dbs.Admin, pgTenantA, "Empresa A")
	dbtest.Tenant(t, dbs.Admin, pgTenantB, "Empresa B")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	NewSoAHandler(NewPgSoAStore(dbs.App), logger).Register(mux)
	NewAssetHandler(NewPgAssetStore(dbs.App), logger).Register(mux)
	NewRiskHandler(NewPgRiskStore(dbs.App), logger).Register(mux)

	// Simulate the authenticated session that auth.Middleware would inject.
	withUser := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithUser(r.Context(), &auth.User{Email: "ciso@empresa.test", Role: "admin"})
		tenant.Middleware(mux).ServeHTTP(w, r.WithContext(ctx))
	})
	return withUser, dbs
}

func TestPg_SoALifecycle(t *testing.T) {
	h, dbs := setupPg(t)

	res := decode[map[string]int](t, doAs(h, pgTenantA, http.MethodPost, "/v1/soa/initialize", ""))
	if res["created"] != 93 {
		t.Fatalf("initialize: expected 93 created, got %v", res)
	}
	list := decode[struct{ Data []SoAEntry }](t, doAs(h, pgTenantA, http.MethodGet, "/v1/soa", ""))
	if len(list.Data) != 93 || list.Data[0].ControlID != "A.5.1" || list.Data[0].ControlTitle == "" {
		t.Fatalf("unexpected list: %d entries, first %+v", len(list.Data), list.Data[0])
	}
	id := list.Data[0].ID

	rr := doAs(h, pgTenantA, http.MethodPut, "/v1/soa/"+id,
		`{"control_id":"A.5.1","applicable":true,"implementation_pct":100,"implementation_evidence":"Política aprobada","effectiveness_pct":85,"responsible_role":"CISO","review_date":"2027-01-15T00:00:00Z"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	got := decode[SoAEntry](t, doAs(h, pgTenantA, http.MethodGet, "/v1/soa/"+id, ""))
	if got.ImplementationPct != 100 || got.EffectivenessPct != 85 || got.ImplementationEv != "Política aprobada" ||
		got.ResponsibleRole != "CISO" || got.ReviewDate == nil || got.ReviewDate.Format("2006-01-02") != "2027-01-15" {
		t.Errorf("update not persisted: %+v", got)
	}

	// Categorical statuses (ADR-004) are kept in sync with the percentages.
	var impl, eff string
	if err := dbs.Admin.QueryRow(`SELECT implementation_status, effectiveness FROM soa_entries WHERE id = $1`, id).Scan(&impl, &eff); err != nil {
		t.Fatal(err)
	}
	if impl != "implemented" || eff != "effective" {
		t.Errorf("derived statuses: %s / %s", impl, eff)
	}

	sum := decode[SoASummary](t, doAs(h, pgTenantA, http.MethodGet, "/v1/soa/summary", ""))
	if sum.TotalControls != 93 || sum.Implemented != 1 || sum.NotImplemented != 92 {
		t.Errorf("unexpected summary: %+v", sum)
	}

	if rr := doAs(h, pgTenantA, http.MethodPost, "/v1/soa", `{"control_id":"A.5.1","applicable":true}`); rr.Code != http.StatusConflict {
		t.Errorf("duplicate control: expected 409, got %d", rr.Code)
	}
	if rr := doAs(h, pgTenantA, http.MethodGet, "/v1/soa/not-a-uuid", ""); rr.Code != http.StatusNotFound {
		t.Errorf("malformed id: expected 404, got %d", rr.Code)
	}
	if rr := doAs(h, pgTenantA, http.MethodDelete, "/v1/soa/"+id, ""); rr.Code != http.StatusNoContent {
		t.Errorf("delete: expected 204, got %d", rr.Code)
	}
	if rr := doAs(h, pgTenantA, http.MethodDelete, "/v1/soa/"+id, ""); rr.Code != http.StatusNotFound {
		t.Errorf("second delete: expected 404, got %d", rr.Code)
	}
}

func TestPg_AssetsAndRisks(t *testing.T) {
	h, _ := setupPg(t)

	a := decode[Asset](t, doAs(h, pgTenantA, http.MethodPost, "/v1/assets",
		`{"name":"ERP","asset_type":"software","classification":"confidential","confidentiality":4,"integrity":5,"availability":3}`))
	if a.ID == "" || a.Criticality != 5 {
		t.Fatalf("unexpected asset: %+v", a)
	}
	if rr := doAs(h, pgTenantA, http.MethodPost, "/v1/assets", `{"name":"ERP","asset_type":"software"}`); rr.Code != http.StatusConflict {
		t.Errorf("duplicate asset: expected 409, got %d", rr.Code)
	}
	// The same name is free for another organization.
	if rr := doAs(h, pgTenantB, http.MethodPost, "/v1/assets", `{"name":"ERP","asset_type":"software"}`); rr.Code != http.StatusCreated {
		t.Errorf("same name, other tenant: expected 201, got %d", rr.Code)
	}
	a = decode[Asset](t, doAs(h, pgTenantA, http.MethodPut, "/v1/assets/"+a.ID,
		`{"name":"ERP","asset_type":"software","confidentiality":2,"integrity":2,"availability":2}`))
	if a.Criticality != 2 {
		t.Errorf("criticality not recomputed: %+v", a)
	}

	r := decode[Risk](t, doAs(h, pgTenantA, http.MethodPost, "/v1/risks",
		`{"title":"Ransomware","likelihood":4,"impact":5,"review_date":"2027-03-31"}`))
	if r.Code != "R-001" || r.Level != 20 || r.ReviewDate != "2027-03-31" {
		t.Fatalf("unexpected risk: %+v", r)
	}
	r = decode[Risk](t, doAs(h, pgTenantA, http.MethodPut, "/v1/risks/"+r.ID,
		`{"title":"Ransomware","likelihood":4,"impact":5,"status":"in_treatment","residual_likelihood":2,"residual_impact":3}`))
	got := decode[Risk](t, doAs(h, pgTenantA, http.MethodGet, "/v1/risks/"+r.ID, ""))
	if got.Code != "R-001" || got.Status != "in_treatment" || got.ResidualLevel == nil || *got.ResidualLevel != 6 ||
		got.ResidualRating != "medium" || got.ReviewDate != "" {
		t.Errorf("unexpected stored risk: %+v", got)
	}
	if rr := doAs(h, pgTenantA, http.MethodDelete, "/v1/risks/"+r.ID, ""); rr.Code != http.StatusNoContent {
		t.Errorf("delete risk: expected 204, got %d", rr.Code)
	}
}

func TestPg_CrossTenantAccessIsDenied(t *testing.T) {
	h, _ := setupPg(t)

	soa := decode[SoAEntry](t, doAs(h, pgTenantA, http.MethodPost, "/v1/soa", `{"control_id":"A.5.1","applicable":true}`))
	asset := decode[Asset](t, doAs(h, pgTenantA, http.MethodPost, "/v1/assets", `{"name":"ERP","asset_type":"software"}`))
	risk := decode[Risk](t, doAs(h, pgTenantA, http.MethodPost, "/v1/risks", `{"title":"x","likelihood":2,"impact":2}`))

	paths := map[string]string{
		"/v1/soa/" + soa.ID:      `{"control_id":"A.5.1","applicable":true,"implementation_pct":100}`,
		"/v1/assets/" + asset.ID: `{"name":"hijacked","asset_type":"software"}`,
		"/v1/risks/" + risk.ID:   `{"title":"hijacked","likelihood":1,"impact":1}`,
	}
	for path, body := range paths {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			if rr := doAs(h, pgTenantB, method, path, body); rr.Code != http.StatusNotFound {
				t.Errorf("%s %s as other tenant: expected 404, got %d", method, path, rr.Code)
			}
		}
		if rr := doAs(h, pgTenantA, http.MethodGet, path, ""); rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "hijacked") {
			t.Errorf("GET %s as owner: got %d %s", path, rr.Code, rr.Body.String())
		}
	}
	for _, list := range []string{"/v1/soa", "/v1/assets", "/v1/risks"} {
		if res := decode[struct{ Count int }](t, doAs(h, pgTenantB, http.MethodGet, list, "")); res.Count != 0 {
			t.Errorf("GET %s as other tenant: expected 0 records, got %d", list, res.Count)
		}
	}
}

func TestPg_AuditTrail(t *testing.T) {
	h, dbs := setupPg(t)

	r := decode[Risk](t, doAs(h, pgTenantA, http.MethodPost, "/v1/risks", `{"title":"x","likelihood":2,"impact":2}`))
	doAs(h, pgTenantA, http.MethodPut, "/v1/risks/"+r.ID, `{"title":"x","likelihood":5,"impact":5,"status":"accepted","treatment":"accept"}`)
	doAs(h, pgTenantA, http.MethodDelete, "/v1/risks/"+r.ID, "")
	// A rejected request must leave no audit record.
	doAs(h, pgTenantA, http.MethodPost, "/v1/risks", `{"title":"","likelihood":2,"impact":2}`)

	rows, err := dbs.Admin.Query(
		`SELECT action, detail->>'actor', COALESCE(detail->'before'->>'risk_level', ''), COALESCE(detail->'after'->>'risk_level', '')
		 FROM audit_log WHERE tenant_id = $1 AND entity_id = $2 ORDER BY id`, pgTenantA, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var action, actor, before, after string
		if err := rows.Scan(&action, &actor, &before, &after); err != nil {
			t.Fatal(err)
		}
		if actor != "ciso@empresa.test" {
			t.Errorf("%s: actor = %q", action, actor)
		}
		got = append(got, action+":"+before+">"+after)
	}
	want := "risk.create:>,risk.update:4>25,risk.delete:>"
	if strings.Join(got, ",") != want {
		t.Errorf("audit trail = %v, want %s", got, want)
	}

	var total int
	if err := dbs.Admin.QueryRow(`SELECT count(*) FROM audit_log`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("expected exactly 3 audit records, got %d", total)
	}
}

func TestPg_ConcurrentRiskCodes(t *testing.T) {
	_, dbs := setupPg(t)
	store := NewPgRiskStore(dbs.App)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := Risk{TenantID: pgTenantA, Title: "t", Likelihood: 1, Impact: 1, Treatment: "accept", Status: "open"}
			errs <- store.Create(ctx, &r)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent create: %v", err)
		}
	}

	risks, err := store.List(ctx, pgTenantA)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range risks {
		seen[r.Code] = true
	}
	if len(risks) != n || len(seen) != n {
		t.Errorf("expected %d risks with distinct codes, got %d risks / %d codes", n, len(risks), len(seen))
	}
}

func TestPg_AuditLogEndpoint(t *testing.T) {
	dbs := dbtest.New(t)
	dbtest.Tenant(t, dbs.Admin, pgTenantA, "Empresa A")
	dbtest.Tenant(t, dbs.Admin, pgTenantB, "Empresa B")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	NewRiskHandler(NewPgRiskStore(dbs.App), logger).Register(mux)
	NewAuditHandler(NewPgAuditStore(dbs.App), logger).Register(mux)
	as := func(tenantID, role, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		ctx := auth.WithUser(tenant.WithTenantID(req.Context(), tenantID),
			&auth.User{Email: role + "@empresa.test", Role: role, TenantID: tenantID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req.WithContext(ctx))
		return rr
	}

	for i := 0; i < 5; i++ {
		as(pgTenantA, "admin", http.MethodPost, "/v1/risks", `{"title":"Riesgo","likelihood":2,"impact":2}`)
	}
	as(pgTenantB, "admin", http.MethodPost, "/v1/risks", `{"title":"Ajeno","likelihood":1,"impact":1}`)

	type page struct {
		Data       []AuditEvent `json:"data"`
		NextBefore int64        `json:"next_before"`
		Recorded   bool         `json:"recorded"`
	}
	first := decode[page](t, as(pgTenantA, "auditor", http.MethodGet, "/v1/audit-log?limit=3", ""))
	if !first.Recorded || len(first.Data) != 3 || first.NextBefore == 0 {
		t.Fatalf("first page: %+v", first)
	}
	if first.Data[0].ID <= first.Data[1].ID || first.Data[0].Action != "risk.create" || first.Data[0].Actor != "admin@empresa.test" {
		t.Errorf("events should be newest first with their actor: %+v", first.Data[0])
	}
	second := decode[page](t, as(pgTenantA, "auditor", http.MethodGet,
		"/v1/audit-log?limit=3&before="+strconv.FormatInt(first.NextBefore, 10), ""))
	if len(second.Data) != 2 || second.NextBefore != 0 {
		t.Errorf("second page: %+v", second)
	}
	// Only this organization's events, across both pages.
	for _, e := range append(first.Data, second.Data...) {
		if strings.Contains(string(e.Detail), "Ajeno") {
			t.Errorf("event of another organization leaked: %+v", e)
		}
	}
	if other := decode[page](t, as(pgTenantB, "owner", http.MethodGet, "/v1/audit-log", "")); len(other.Data) != 1 {
		t.Errorf("tenant B should see exactly its own event: %+v", other.Data)
	}

	for _, role := range []string{"editor", "viewer"} {
		if rr := as(pgTenantA, role, http.MethodGet, "/v1/audit-log", ""); rr.Code != http.StatusForbidden {
			t.Errorf("%s reading the audit trail: expected 403, got %d", role, rr.Code)
		}
	}
	for _, q := range []string{"limit=0", "limit=x", "before=-1"} {
		if rr := as(pgTenantA, "owner", http.MethodGet, "/v1/audit-log?"+q, ""); rr.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", q, rr.Code)
		}
	}
}
