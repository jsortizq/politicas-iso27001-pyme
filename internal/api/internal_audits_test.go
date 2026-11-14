package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/database/dbtest"
)

func pgFileEnv(t *testing.T) *fileEnv {
	dbs := dbtest.New(t) // skips without TEST_DATABASE_URL
	return newFileEnv(t, &dbs)
}

func (e *fileEnv) newAudit(title string) InternalAudit {
	e.t.Helper()
	rr := e.as("editor", http.MethodPost, "/v1/audits",
		`{"title":"`+title+`","scope":"Procesos de TI","criteria":"ISO/IEC 27001:2022","lead_auditor":"Marta Ruiz (externa)","planned_date":"2026-12-01","controls":["A.5.2","A.5.1"]}`)
	mustStatus(e.t, rr, http.StatusCreated, "create audit")
	return decode[InternalAudit](e.t, rr)
}

// startedAudit returns an audit in progress with one major nonconformity
// (finding 1) and one observation (finding 2).
func (e *fileEnv) startedAudit() InternalAudit {
	e.t.Helper()
	a := e.newAudit("Auditoría interna 2026")
	base := "/v1/audits/" + a.ID
	mustStatus(e.t, e.as("editor", http.MethodPost, base+"/start", nil), http.StatusOK, "start")
	mustStatus(e.t, e.as("editor", http.MethodPost, base+"/findings",
		`{"severity":"major_nc","control_id":"A.5.1","description":"La política de seguridad no está aprobada por la dirección","evidence":"Documento en borrador desde 2024"}`),
		http.StatusCreated, "add finding 1")
	rr := e.as("editor", http.MethodPost, base+"/findings",
		`{"severity":"observation","clause":"7.5.3","description":"Algunos registros no indican fecha de revisión"}`)
	mustStatus(e.t, rr, http.StatusCreated, "add finding 2")
	return decode[InternalAudit](e.t, rr)
}

func findingStates(a InternalAudit) string {
	var parts []string
	for _, f := range a.Findings {
		parts = append(parts, fmt.Sprintf("H%d=%s", f.Number, f.Status))
	}
	return strings.Join(parts, " ")
}

func TestInternalAudit_Lifecycle(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		a := e.newAudit("Auditoría interna 2026")
		if a.Status != AuditPlanned || a.Type != "internal" || strings.Join(a.Controls, ",") != "A.5.1,A.5.2" ||
			a.CreatedByEmail != "editor@empresa.test" || len(a.Findings) != 0 {
			t.Fatalf("new audit: %+v", a)
		}
		base := "/v1/audits/" + a.ID

		// Findings belong to the audit work: not before it starts.
		mustStatus(t, e.as("editor", http.MethodPost, base+"/findings", `{"severity":"minor_nc","description":"x"}`), http.StatusConflict, "finding on a planned audit")
		mustStatus(t, e.as("editor", http.MethodPost, base+"/complete", `{"summary":"x"}`), http.StatusConflict, "completing a planned audit")

		a = decode[InternalAudit](t, e.as("editor", http.MethodPost, base+"/start", nil))
		if a.Status != AuditInProgress {
			t.Fatalf("after start: %s", a.Status)
		}
		mustStatus(t, e.as("editor", http.MethodPost, base+"/start", nil), http.StatusConflict, "starting twice")

		e.as("editor", http.MethodPost, base+"/findings", `{"severity":"major_nc","control_id":"A.5.1","description":" Política sin aprobar ","evidence":"Borrador"}`)
		a = decode[InternalAudit](t, e.as("editor", http.MethodPost, base+"/findings", `{"severity":"observation","clause":"7.5.3","description":"Registros sin fecha"}`))
		if len(a.Findings) != 2 || a.Findings[0].Number != 1 || a.Findings[1].Number != 2 ||
			a.Findings[0].Description != "Política sin aprobar" || a.Findings[0].Control != "A.5.1" ||
			a.Findings[1].Clause != "7.5.3" || a.OpenFindings != 2 || findingStates(a) != "H1=open H2=open" {
			t.Fatalf("findings: %+v", a.Findings)
		}
		h1, h2 := base+"/findings/"+a.Findings[0].ID, base+"/findings/"+a.Findings[1].ID

		// While auditing, what was found can be corrected or withdrawn.
		a = decode[InternalAudit](t, e.as("editor", http.MethodPut, h2, `{"severity":"minor_nc","clause":"7.5.3","description":"Registros sin fecha de revisión"}`))
		if a.Findings[1].Severity != "minor_nc" || a.Findings[1].Description != "Registros sin fecha de revisión" {
			t.Errorf("finding update: %+v", a.Findings[1])
		}

		// Completing needs conclusions and fixes the content.
		mustStatus(t, e.as("editor", http.MethodPost, base+"/complete", `{"summary":"  "}`), http.StatusUnprocessableEntity, "complete without conclusions")
		mustStatus(t, e.as("editor", http.MethodPost, base+"/complete", `{"summary":"x","completed_date":"2999-01-01"}`), http.StatusUnprocessableEntity, "complete in the future")
		a = decode[InternalAudit](t, e.as("editor", http.MethodPost, base+"/complete", `{"summary":"El SGSI opera con dos no conformidades."}`))
		if a.Status != AuditCompleted || a.CompletedDate != today() || a.Summary == "" {
			t.Fatalf("after complete: %+v", a)
		}
		for name, rr := range map[string]*httptest.ResponseRecorder{
			"new finding":    e.as("editor", http.MethodPost, base+"/findings", `{"severity":"minor_nc","description":"tardío"}`),
			"reword finding": e.as("editor", http.MethodPut, h1, `{"severity":"observation","description":"suavizado"}`),
			"delete finding": e.as("editor", http.MethodDelete, h1, nil),
			"edit audit":     e.as("editor", http.MethodPut, base, `{"title":"Otro título"}`),
			"complete again": e.as("editor", http.MethodPost, base+"/complete", `{"summary":"x"}`),
			"cancel":         e.as("admin", http.MethodPost, base+"/cancel", `{"reason":"x"}`),
			"delete audit":   e.as("admin", http.MethodDelete, base, nil),
		} {
			mustStatus(t, rr, http.StatusConflict, name+" after completion")
		}

		// The follow-up continues: plan, implement, verify, close.
		rr := e.as("editor", http.MethodPut, h1+"/action",
			`{"root_cause":"No había responsable asignado","corrective_action":"Aprobar la política en comité","responsible":"Gerencia","due_date":"2020-01-31","status":"in_progress"}`)
		mustStatus(t, rr, http.StatusOK, "plan")
		a = decode[InternalAudit](t, rr)
		if f := a.Findings[0]; f.Status != FindingInProgress || !f.Overdue || f.Responsible != "Gerencia" || a.OverdueCount != 1 || a.OpenFindings != 2 {
			t.Fatalf("after plan: %+v (overdue %d, open %d)", f, a.OverdueCount, a.OpenFindings)
		}

		mustStatus(t, e.as("editor", http.MethodPost, h1+"/close", `{"verification":"ok"}`), http.StatusForbidden, "editor closing")
		mustStatus(t, e.as("admin", http.MethodPost, h1+"/close", `{"verification":" "}`), http.StatusUnprocessableEntity, "close without verification")
		rr = e.as("admin", http.MethodPost, h1+"/close", `{"verification":"Acta del comité del 5 de febrero; política publicada"}`)
		mustStatus(t, rr, http.StatusOK, "close")
		a = decode[InternalAudit](t, rr)
		if f := a.Findings[0]; f.Status != FindingClosed || f.Overdue || f.ClosedByEmail != "admin@empresa.test" || f.ClosedAt == nil ||
			a.OpenFindings != 1 || a.OverdueCount != 0 {
			t.Fatalf("after close: %+v", f)
		}
		mustStatus(t, e.as("admin", http.MethodPost, h1+"/close", `{"verification":"otra vez"}`), http.StatusConflict, "closing twice")
		mustStatus(t, e.as("editor", http.MethodPut, h1+"/action", `{"corrective_action":"cambio","status":"open"}`), http.StatusConflict, "plan on a closed finding")

		// The fix did not hold: reopen, with a reason.
		mustStatus(t, e.as("editor", http.MethodPost, h1+"/reopen", `{"note":"x"}`), http.StatusForbidden, "editor reopening")
		mustStatus(t, e.as("owner", http.MethodPost, h1+"/reopen", `{"note":""}`), http.StatusUnprocessableEntity, "reopen without reason")
		a = decode[InternalAudit](t, e.as("owner", http.MethodPost, h1+"/reopen", `{"note":"La política volvió a quedar desactualizada"}`))
		if f := a.Findings[0]; f.Status != FindingInProgress || f.ClosedAt != nil || f.ClosedByEmail != "" || f.ReopenNote == "" || f.Verification == "" {
			t.Fatalf("after reopen: %+v", f)
		}
		mustStatus(t, e.as("owner", http.MethodPost, h1+"/reopen", `{"note":"x"}`), http.StatusConflict, "reopening an open finding")

		// What is stored matches what the operations returned.
		got := decode[InternalAudit](t, e.as("viewer", http.MethodGet, base, nil))
		if findingStates(got) != "H1=in_progress H2=open" || got.Status != AuditCompleted || got.Findings[0].RootCause != "No había responsable asignado" {
			t.Errorf("persisted audit: %s %s", got.Status, findingStates(got))
		}
	})
}

func TestFinding_ClosingRules(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		a := e.startedAudit()
		base := "/v1/audits/" + a.ID
		nc, obs := base+"/findings/"+a.Findings[0].ID, base+"/findings/"+a.Findings[1].ID

		// A nonconformity needs cause and action on record (clause 10.2).
		mustStatus(t, e.as("admin", http.MethodPost, nc+"/close", `{"verification":"revisado"}`), http.StatusUnprocessableEntity, "closing an NC without cause and action")
		e.as("editor", http.MethodPut, nc+"/action", `{"root_cause":"Sin responsable","status":"in_progress"}`)
		mustStatus(t, e.as("admin", http.MethodPost, nc+"/close", `{"verification":"revisado"}`), http.StatusUnprocessableEntity, "closing an NC without action")
		// "Implemented" claims an action was taken, so one must be described.
		mustStatus(t, e.as("editor", http.MethodPut, nc+"/action", `{"root_cause":"Sin responsable","status":"implemented"}`), http.StatusUnprocessableEntity, "implemented without action")
		mustStatus(t, e.as("editor", http.MethodPut, nc+"/action", `{"root_cause":"Sin responsable","corrective_action":"Política aprobada","status":"implemented"}`), http.StatusOK, "implemented")
		mustStatus(t, e.as("admin", http.MethodPost, nc+"/close", `{"verification":"Política publicada y comunicada"}`), http.StatusOK, "closing the NC")

		// An observation can be closed with just the note.
		rr := e.as("owner", http.MethodPost, obs+"/close", `{"verification":"Se tendrá en cuenta en la próxima revisión"}`)
		mustStatus(t, rr, http.StatusOK, "closing an observation")
		if got := decode[InternalAudit](t, rr); got.OpenFindings != 0 || findingStates(got) != "H1=closed H2=closed" {
			t.Errorf("after closing both: %s", findingStates(got))
		}

		for name, body := range map[string]string{
			"closing through the plan": `{"status":"closed"}`,
			"unknown status":           `{"status":"done"}`,
			"bad due date":             `{"due_date":"mañana"}`,
		} {
			e.as("owner", http.MethodPost, obs+"/reopen", `{"note":"prueba"}`)
			mustStatus(t, e.as("editor", http.MethodPut, obs+"/action", body), http.StatusUnprocessableEntity, name)
		}
	})
}

func TestInternalAudit_CancelAndDelete(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		// Planned: can be deleted (it left no history) or cancelled.
		planned := e.newAudit("Auditoría que no se hará")
		mustStatus(t, e.as("editor", http.MethodDelete, "/v1/audits/"+planned.ID, nil), http.StatusForbidden, "editor deleting")
		mustStatus(t, e.as("admin", http.MethodDelete, "/v1/audits/"+planned.ID, nil), http.StatusNoContent, "delete planned")
		mustStatus(t, e.as("admin", http.MethodGet, "/v1/audits/"+planned.ID, nil), http.StatusNotFound, "get deleted")

		// In progress: cancelled with a reason, kept on record, frozen.
		a := e.startedAudit()
		base := "/v1/audits/" + a.ID
		mustStatus(t, e.as("admin", http.MethodDelete, base, nil), http.StatusConflict, "deleting a started audit")
		mustStatus(t, e.as("editor", http.MethodPost, base+"/cancel", `{"reason":"x"}`), http.StatusForbidden, "editor cancelling")
		mustStatus(t, e.as("admin", http.MethodPost, base+"/cancel", `{"reason":""}`), http.StatusUnprocessableEntity, "cancel without reason")
		rr := e.as("admin", http.MethodPost, base+"/cancel", `{"reason":"El auditor externo no estuvo disponible"}`)
		mustStatus(t, rr, http.StatusOK, "cancel")
		cancelled := decode[InternalAudit](t, rr)
		if cancelled.Status != AuditCancelled || cancelled.CancelReason == "" || len(cancelled.Findings) != 2 {
			t.Fatalf("after cancel: %+v", cancelled)
		}
		finding := base + "/findings/" + a.Findings[0].ID
		for name, rr := range map[string]*httptest.ResponseRecorder{
			"start":       e.as("editor", http.MethodPost, base+"/start", nil),
			"complete":    e.as("editor", http.MethodPost, base+"/complete", `{"summary":"x"}`),
			"cancel":      e.as("admin", http.MethodPost, base+"/cancel", `{"reason":"x"}`),
			"edit":        e.as("editor", http.MethodPut, base, `{"title":"x"}`),
			"new finding": e.as("editor", http.MethodPost, base+"/findings", `{"severity":"minor_nc","description":"x"}`),
			"plan":        e.as("editor", http.MethodPut, finding+"/action", `{"status":"in_progress"}`),
			"close":       e.as("admin", http.MethodPost, finding+"/close", `{"verification":"x"}`),
			"delete":      e.as("admin", http.MethodDelete, base, nil),
		} {
			mustStatus(t, rr, http.StatusConflict, name+" on a cancelled audit")
		}
	})
}

// Deleting and starting the same audit at once must never both succeed:
// a started audit is history, and history is not deleted.
func TestInternalAudit_DeleteRacesWithStart(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		for i := 0; i < 20; i++ {
			a := e.newAudit("Auditoría en disputa")
			base := "/v1/audits/" + a.ID
			var deleted, started int
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				deleted = e.as("admin", http.MethodDelete, base, nil).Code
			}()
			go func() {
				defer wg.Done()
				started = e.as("editor", http.MethodPost, base+"/start", nil).Code
			}()
			wg.Wait()

			switch {
			case deleted == http.StatusNoContent && started == http.StatusNotFound:
				mustStatus(t, e.as("admin", http.MethodGet, base, nil), http.StatusNotFound, "get after delete won")
			case deleted == http.StatusConflict && started == http.StatusOK:
				mustStatus(t, e.as("admin", http.MethodGet, base, nil), http.StatusOK, "get after start won")
			default:
				t.Fatalf("round %d: delete=%d start=%d; exactly one must win", i, deleted, started)
			}
		}
	})
}

func TestInternalAudit_ValidationAndRoles(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		for name, body := range map[string]string{
			"no title":        `{"title":" "}`,
			"bad type":        `{"title":"x","audit_type":"sorpresa"}`,
			"bad date":        `{"title":"x","planned_date":"diciembre"}`,
			"unknown control": `{"title":"x","controls":["A.99.1"]}`,
		} {
			mustStatus(t, e.as("editor", http.MethodPost, "/v1/audits", body), http.StatusUnprocessableEntity, name)
		}
		mustStatus(t, e.as("editor", http.MethodPost, "/v1/audits", `{`), http.StatusBadRequest, "invalid JSON")

		a := e.startedAudit()
		base := "/v1/audits/" + a.ID
		finding := base + "/findings/" + a.Findings[0].ID
		for name, body := range map[string]string{
			"no description":  `{"severity":"minor_nc","description":""}`,
			"bad severity":    `{"severity":"grave","description":"x"}`,
			"unknown control": `{"severity":"minor_nc","description":"x","control_id":"A.1.1"}`,
		} {
			mustStatus(t, e.as("editor", http.MethodPost, base+"/findings", body), http.StatusUnprocessableEntity, name)
			mustStatus(t, e.as("editor", http.MethodPut, finding, body), http.StatusUnprocessableEntity, name+" (update)")
		}
		for _, path := range []string{base + "/findings/not-a-uuid", base + "/findings/99999999-9999-4999-8999-999999999999"} {
			mustStatus(t, e.as("editor", http.MethodPut, path, `{"severity":"minor_nc","description":"x"}`), http.StatusNotFound, "unknown finding")
			mustStatus(t, e.as("editor", http.MethodPut, path+"/action", `{"status":"open"}`), http.StatusNotFound, "plan for an unknown finding")
			mustStatus(t, e.as("admin", http.MethodPost, path+"/close", `{"verification":"x"}`), http.StatusNotFound, "closing an unknown finding")
		}
		mustStatus(t, e.as("editor", http.MethodGet, "/v1/audits/not-a-uuid", nil), http.StatusNotFound, "malformed audit id")

		// Withdrawing a finding recorded by mistake, while still auditing.
		rr := e.as("editor", http.MethodDelete, base+"/findings/"+a.Findings[1].ID, nil)
		mustStatus(t, rr, http.StatusOK, "delete finding")
		if got := decode[InternalAudit](t, rr); len(got.Findings) != 1 || got.Findings[0].Number != 1 {
			t.Errorf("after deleting finding 2: %+v", got.Findings)
		}
		// Numbers are not reused within an audit… unless the last one was withdrawn.
		next := decode[InternalAudit](t, e.as("editor", http.MethodPost, base+"/findings", `{"severity":"opportunity","description":"Automatizar"}`))
		if len(next.Findings) != 2 || next.Findings[1].Number != 2 {
			t.Errorf("numbering after a deletion: %+v", next.Findings)
		}

		// Read-only roles see everything and change nothing.
		for _, role := range []string{"viewer", "auditor"} {
			mustStatus(t, e.as(role, http.MethodGet, "/v1/audits", nil), http.StatusOK, role+" list")
			mustStatus(t, e.as(role, http.MethodGet, base, nil), http.StatusOK, role+" get")
			for name, rr := range map[string]*httptest.ResponseRecorder{
				"create":       e.as(role, http.MethodPost, "/v1/audits", `{"title":"x"}`),
				"edit":         e.as(role, http.MethodPut, base, `{"title":"x"}`),
				"complete":     e.as(role, http.MethodPost, base+"/complete", `{"summary":"x"}`),
				"cancel":       e.as(role, http.MethodPost, base+"/cancel", `{"reason":"x"}`),
				"delete":       e.as(role, http.MethodDelete, base, nil),
				"new finding":  e.as(role, http.MethodPost, base+"/findings", `{"severity":"minor_nc","description":"x"}`),
				"edit finding": e.as(role, http.MethodPut, finding, `{"severity":"minor_nc","description":"x"}`),
				"drop finding": e.as(role, http.MethodDelete, finding, nil),
				"plan":         e.as(role, http.MethodPut, finding+"/action", `{"status":"in_progress"}`),
				"close":        e.as(role, http.MethodPost, finding+"/close", `{"verification":"x"}`),
				"reopen":       e.as(role, http.MethodPost, finding+"/reopen", `{"note":"x"}`),
			} {
				mustStatus(t, rr, http.StatusForbidden, role+" "+name)
			}
		}
	})
}

func TestInternalAudit_ListOrderAndCrossTenant(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		e.as("editor", http.MethodPost, "/v1/audits", `{"title":"Antigua","planned_date":"2025-03-01"}`)
		e.as("editor", http.MethodPost, "/v1/audits", `{"title":"Sin fecha"}`)
		a := e.startedAudit() // planned 2026-12-01

		list := decode[struct {
			Count int
			Data  []InternalAudit
		}](t, e.as("viewer", http.MethodGet, "/v1/audits", nil))
		if list.Count != 3 || list.Data[0].Title != "Sin fecha" || list.Data[1].ID != a.ID || list.Data[2].Title != "Antigua" ||
			list.Data[1].OpenFindings != 2 || len(list.Data[1].Findings) != 2 {
			t.Errorf("list: %d audits, order %q / %q / %q", list.Count, list.Data[0].Title, list.Data[1].Title, list.Data[2].Title)
		}

		base := "/v1/audits/" + a.ID
		finding := base + "/findings/" + a.Findings[0].ID
		attempts := []struct{ method, path, body string }{
			{http.MethodGet, base, ""}, {http.MethodPut, base, `{"title":"hijacked"}`},
			{http.MethodPost, base + "/complete", `{"summary":"hijacked"}`}, {http.MethodPost, base + "/cancel", `{"reason":"hijacked"}`},
			{http.MethodDelete, base, ""},
			{http.MethodPost, base + "/findings", `{"severity":"minor_nc","description":"hijacked"}`},
			{http.MethodPut, finding, `{"severity":"opportunity","description":"hijacked"}`},
			{http.MethodDelete, finding, ""},
			{http.MethodPut, finding + "/action", `{"status":"implemented","corrective_action":"hijacked"}`},
			{http.MethodPost, finding + "/close", `{"verification":"hijacked"}`},
		}
		for _, at := range attempts {
			var body any
			if at.body != "" {
				body = at.body
			}
			if rr := e.as("outsider", at.method, at.path, body); rr.Code != http.StatusNotFound {
				t.Errorf("%s %s from another organization: expected 404, got %d", at.method, at.path, rr.Code)
			}
		}
		if got := decode[struct{ Count int }](t, e.as("outsider", http.MethodGet, "/v1/audits", nil)); got.Count != 0 {
			t.Errorf("another organization lists %d audits", got.Count)
		}
		rr := e.as("owner", http.MethodGet, base, nil)
		got := decode[InternalAudit](t, rr)
		if strings.Contains(rr.Body.String(), "hijacked") || got.Status != AuditInProgress || findingStates(got) != "H1=open H2=open" {
			t.Errorf("audit was altered from outside: %s %s", got.Status, findingStates(got))
		}
	})
}

// ── PostgreSQL only ──

func TestPg_InternalAuditTrailAndIsolation(t *testing.T) {
	e := pgFileEnv(t)
	a := e.startedAudit()
	base := "/v1/audits/" + a.ID
	h1 := base + "/findings/" + a.Findings[0].ID
	e.as("editor", http.MethodPut, h1+"/action", `{"root_cause":"causa","corrective_action":"acción","status":"implemented"}`)
	e.as("editor", http.MethodPost, base+"/complete", `{"summary":"Conclusiones"}`)
	e.as("admin", http.MethodPost, h1+"/close", `{"verification":"verificado"}`)
	e.as("owner", http.MethodPost, h1+"/reopen", `{"note":"recayó"}`)
	// Refused actions leave no record.
	e.as("editor", http.MethodPost, h1+"/close", `{"verification":"x"}`)
	e.as("editor", http.MethodPost, base+"/findings", `{"severity":"minor_nc","description":"tardío"}`)

	rows, err := e.dbs.Admin.Query(`SELECT action, detail->>'actor', COALESCE(detail->>'finding', '') FROM audit_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var action, actor, finding string
		rows.Scan(&action, &actor, &finding)
		entry := action + " by " + strings.TrimSuffix(actor, "@empresa.test")
		if finding != "" {
			entry += " #" + finding
		}
		got = append(got, entry)
	}
	want := []string{
		"audit.create by editor", "audit.start by editor", "finding.create by editor #1", "finding.create by editor #2",
		"finding.plan by editor #1", "audit.complete by editor", "finding.close by admin #1", "finding.reopen by owner #1",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit trail:\n got: %v\nwant: %v", got, want)
	}

	// Row-level security, without the stores' own WHERE clauses.
	count := func(tenantID, table string) (n int) {
		t.Helper()
		tx, _ := e.dbs.App.Begin()
		defer tx.Rollback()
		tx.Exec(`SELECT set_config('app.tenant_id', $1, true)`, tenantID)
		if err := tx.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"audits", "audit_findings", "audit_controls"} {
		if n := count(pgTenantA, table); n == 0 {
			t.Errorf("%s: the owner sees no rows", table)
		}
		if n := count(pgTenantB, table); n != 0 {
			t.Errorf("%s: another organization sees %d rows", table, n)
		}
	}

	// The database itself refuses a "closed" finding without a closing time.
	if _, err := e.dbs.Admin.Exec(`UPDATE audit_findings SET status = 'closed' WHERE id = $1`, a.Findings[1].ID); err == nil {
		t.Error("a finding was marked closed without closed_at")
	}
}

func TestPg_ConcurrentFindingsGetDistinctNumbers(t *testing.T) {
	e := pgFileEnv(t)
	a := e.newAudit("Auditoría concurrida")
	base := "/v1/audits/" + a.ID
	e.as("editor", http.MethodPost, base+"/start", nil)

	const n = 12
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = e.as("editor", http.MethodPost, base+"/findings",
				fmt.Sprintf(`{"severity":"observation","description":"hallazgo %d"}`, i)).Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusCreated {
			t.Errorf("finding %d: %d", i, code)
		}
	}
	got := decode[InternalAudit](t, e.as("viewer", http.MethodGet, base, nil))
	if len(got.Findings) != n {
		t.Fatalf("expected %d findings, got %d", n, len(got.Findings))
	}
	for i, f := range got.Findings {
		if f.Number != i+1 {
			t.Errorf("finding at position %d has number %d", i, f.Number)
		}
	}
}
