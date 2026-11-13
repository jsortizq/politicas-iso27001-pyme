package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/catalog"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
)

// ── Internal audits (clause 9.2) and corrective action (clause 10.2) ──
//
// An audit is planned, carried out and completed:
//
//	planned ──start──▶ in_progress ──complete──▶ completed
//	   └───────────────────┴──────cancel──────▶ cancelled
//
// Findings are recorded while the audit is in progress. Once the audit is
// completed its content is fixed — what was found does not change — but the
// follow-up of each finding continues until it is closed:
//
//	open ──▶ in_progress ──▶ implemented ──close──▶ closed
//	                                         ◀──reopen──┘
//
// Closing a finding means someone verified that the action worked, so it
// is reserved to owners and administrators and requires a written note.
//
// (The "audit" in AuditHandler/AuditStore is the activity log; the types
// here are prefixed InternalAudit to keep the two apart.)

// Audit statuses.
const (
	AuditPlanned    = "planned"
	AuditInProgress = "in_progress"
	AuditCompleted  = "completed"
	AuditCancelled  = "cancelled"
)

// Finding statuses.
const (
	FindingOpen        = "open"
	FindingInProgress  = "in_progress"
	FindingImplemented = "implemented"
	FindingClosed      = "closed"
)

// Finding severities.
const (
	SeverityMajor       = "major_nc"    // nonconformity that puts the SGSI's results at risk
	SeverityMinor       = "minor_nc"    // isolated nonconformity
	SeverityObservation = "observation" // not a nonconformity yet, could become one
	SeverityOpportunity = "opportunity" // suggestion for improvement
)

var (
	auditTypes        = []string{"internal", "external", "surveillance"}
	findingSeverities = []string{SeverityMajor, SeverityMinor, SeverityObservation, SeverityOpportunity}
	// Statuses a member can set while working on the action plan; closing
	// and reopening have their own endpoints.
	findingWorkStatuses = []string{FindingOpen, FindingInProgress, FindingImplemented}
)

// Finding is something an audit found, plus the follow-up of its treatment.
type Finding struct {
	ID       string `json:"id"`
	Number   int    `json:"number"` // 1, 2, 3… within its audit
	Severity string `json:"severity"`
	Control  string `json:"control_id,omitempty"` // Annex A control, if it relates to one
	Clause   string `json:"clause"`               // e.g. "7.5.3" for requirements of clauses 4–10

	Description string `json:"description"`
	Evidence    string `json:"evidence"` // objective evidence observed

	// Corrective action (10.2)
	RootCause        string `json:"root_cause"`
	CorrectiveAction string `json:"corrective_action"`
	Responsible      string `json:"responsible"`
	DueDate          string `json:"due_date,omitempty"` // YYYY-MM-DD
	Status           string `json:"status"`
	Overdue          bool   `json:"overdue"`

	Verification   string     `json:"verification"` // how effectiveness was verified at closing
	ClosedByEmail  string     `json:"closed_by,omitempty"`
	ClosedAt       *time.Time `json:"closed_at,omitempty"`
	ReopenNote     string     `json:"reopen_note,omitempty"`
	CreatedByEmail string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// IsNonconformity reports whether the finding requires corrective action.
func (f *Finding) IsNonconformity() bool {
	return f.Severity == SeverityMajor || f.Severity == SeverityMinor
}

// InternalAudit is an audit with its findings, in the order they were recorded.
type InternalAudit struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id,omitempty"`
	Title          string    `json:"title"`
	Type           string    `json:"audit_type"`
	Status         string    `json:"status"`
	Scope          string    `json:"scope"`
	Criteria       string    `json:"criteria"`
	LeadAuditor    string    `json:"lead_auditor"`
	PlannedDate    string    `json:"planned_date,omitempty"`   // YYYY-MM-DD
	CompletedDate  string    `json:"completed_date,omitempty"` // YYYY-MM-DD
	Summary        string    `json:"summary"`                  // conclusions, required to complete
	CancelReason   string    `json:"cancel_reason,omitempty"`
	Controls       []string  `json:"controls"`
	Findings       []Finding `json:"findings"`
	OpenFindings   int       `json:"open_findings"`
	OverdueCount   int       `json:"overdue_findings"`
	CreatedByEmail string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// refresh recomputes derived fields and orders the findings.
func (a *InternalAudit) refresh() {
	sort.Slice(a.Findings, func(i, j int) bool { return a.Findings[i].Number < a.Findings[j].Number })
	a.OpenFindings, a.OverdueCount = 0, 0
	now := today()
	for i := range a.Findings {
		f := &a.Findings[i]
		f.Overdue = f.Status != FindingClosed && f.DueDate != "" && f.DueDate < now
		if f.Status != FindingClosed {
			a.OpenFindings++
		}
		if f.Overdue {
			a.OverdueCount++
		}
	}
	if a.Controls == nil {
		a.Controls = []string{}
	}
	if a.Findings == nil {
		a.Findings = []Finding{}
	}
}

func (a *InternalAudit) finding(id string) *Finding {
	for i := range a.Findings {
		if a.Findings[i].ID == id {
			return &a.Findings[i]
		}
	}
	return nil
}

// ── Audit transitions ──

func (a *InternalAudit) start() error {
	if a.Status != AuditPlanned {
		return noTransition("only a planned audit can be started (this one is %s)", a.Status)
	}
	a.Status = AuditInProgress
	return nil
}

func (a *InternalAudit) complete(summary, completedDate string) error {
	if a.Status != AuditInProgress {
		return noTransition("only an audit in progress can be completed (this one is %s)", a.Status)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return invalid("write the audit conclusions before completing it")
	}
	if len(summary) > 8000 {
		return invalid("summary must be at most 8000 characters")
	}
	date, err := parseDate("completed_date", completedDate)
	if err != nil {
		return err
	}
	if date == "" {
		date = today()
	}
	if date > today() {
		return invalid("completed_date cannot be in the future")
	}
	a.Status, a.Summary, a.CompletedDate = AuditCompleted, summary, date
	return nil
}

func (a *InternalAudit) cancel(reason string) error {
	if a.Status != AuditPlanned && a.Status != AuditInProgress {
		return noTransition("a %s audit cannot be cancelled", a.Status)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return invalid("explain why the audit is cancelled")
	}
	if len(reason) > 2000 {
		return invalid("reason must be at most 2000 characters")
	}
	a.Status, a.CancelReason = AuditCancelled, reason
	return nil
}

// editable reports whether the audit's own data may still change.
func (a *InternalAudit) editable() bool {
	return a.Status == AuditPlanned || a.Status == AuditInProgress
}

// ── Finding operations ──

// addFinding records a new finding; only possible while auditing.
func (a *InternalAudit) addFinding(f Finding) (*Finding, error) {
	if a.Status != AuditInProgress {
		return nil, noTransition("findings are recorded while the audit is in progress (this one is %s)", a.Status)
	}
	next := 1
	for _, existing := range a.Findings {
		if existing.Number >= next {
			next = existing.Number + 1
		}
	}
	f.Number = next
	f.Status = FindingOpen
	a.Findings = append(a.Findings, f)
	return &a.Findings[len(a.Findings)-1], nil
}

// contentChangeable guards what the audit found: fixed once it is completed.
func (a *InternalAudit) contentChangeable() error {
	if a.Status != AuditInProgress {
		return noTransition("what was found cannot change once the audit is %s; record the follow-up in the action plan", a.Status)
	}
	return nil
}

func (a *InternalAudit) removeFinding(id string) error {
	if err := a.contentChangeable(); err != nil {
		return err
	}
	for i := range a.Findings {
		if a.Findings[i].ID == id {
			a.Findings = append(a.Findings[:i], a.Findings[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// followUp guards the action plan: possible until the finding is closed,
// and not for audits that were cancelled.
func (a *InternalAudit) followUp(id string) (*Finding, error) {
	if a.Status == AuditCancelled {
		return nil, noTransition("the audit was cancelled")
	}
	f := a.finding(id)
	if f == nil {
		return nil, ErrNotFound
	}
	return f, nil
}

// findingContent is what the auditor observed.
type findingContent struct {
	Severity    string `json:"severity"`
	Control     string `json:"control_id"`
	Clause      string `json:"clause"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
}

func (c *findingContent) normalize() error {
	c.Description = strings.TrimSpace(c.Description)
	c.Evidence = strings.TrimSpace(c.Evidence)
	c.Clause = strings.TrimSpace(c.Clause)
	c.Control = strings.TrimSpace(c.Control)
	if c.Description == "" {
		return invalid("description is required")
	}
	if len(c.Description) > 4000 || len(c.Evidence) > 4000 {
		return invalid("description and evidence must be at most 4000 characters")
	}
	if len(c.Clause) > 100 {
		return invalid("clause must be at most 100 characters")
	}
	if !oneOf(c.Severity, findingSeverities) {
		return invalid("severity must be one of: %s", strings.Join(findingSeverities, ", "))
	}
	if c.Control != "" {
		if _, ok := catalog.Lookup(c.Control); !ok {
			return invalid("control %s is not in the ISO/IEC 27001:2022 Annex A catalog", c.Control)
		}
	}
	return nil
}

func (c *findingContent) applyTo(f *Finding) {
	f.Severity, f.Control, f.Clause = c.Severity, c.Control, c.Clause
	f.Description, f.Evidence = c.Description, c.Evidence
}

// findingPlan is the treatment of a finding.
type findingPlan struct {
	RootCause        string `json:"root_cause"`
	CorrectiveAction string `json:"corrective_action"`
	Responsible      string `json:"responsible"`
	DueDate          string `json:"due_date"`
	Status           string `json:"status"`
}

func (p *findingPlan) applyTo(f *Finding) error {
	if f.Status == FindingClosed {
		return noTransition("the finding is closed; reopen it to change its action plan")
	}
	p.RootCause = strings.TrimSpace(p.RootCause)
	p.CorrectiveAction = strings.TrimSpace(p.CorrectiveAction)
	p.Responsible = strings.TrimSpace(p.Responsible)
	if len(p.RootCause) > 4000 || len(p.CorrectiveAction) > 4000 {
		return invalid("root_cause and corrective_action must be at most 4000 characters")
	}
	if len(p.Responsible) > 200 {
		return invalid("responsible must be at most 200 characters")
	}
	due, err := parseDate("due_date", p.DueDate)
	if err != nil {
		return err
	}
	if p.Status == "" {
		p.Status = f.Status
	}
	if !oneOf(p.Status, findingWorkStatuses) {
		return invalid("status must be one of: %s (closing has its own action)", strings.Join(findingWorkStatuses, ", "))
	}
	if p.Status == FindingImplemented && p.CorrectiveAction == "" {
		return invalid("describe the action taken before marking it as implemented")
	}
	f.RootCause, f.CorrectiveAction, f.Responsible = p.RootCause, p.CorrectiveAction, p.Responsible
	f.DueDate, f.Status = due, p.Status
	return nil
}

// closeFinding records that the treatment was verified.
func closeFinding(f *Finding, verification string, by *auth.User, now time.Time) error {
	if f.Status == FindingClosed {
		return noTransition("the finding is already closed")
	}
	verification = strings.TrimSpace(verification)
	if verification == "" {
		return invalid("describe how the result was verified before closing")
	}
	if len(verification) > 4000 {
		return invalid("verification must be at most 4000 characters")
	}
	// A nonconformity cannot be closed without its cause and its action:
	// that is what clause 10.2 asks to keep on record.
	if f.IsNonconformity() && (f.RootCause == "" || f.CorrectiveAction == "") {
		return invalid("a nonconformity needs its root cause and corrective action before it can be closed")
	}
	f.Status, f.Verification = FindingClosed, verification
	f.ClosedByEmail, f.ClosedAt = by.Email, &now
	return nil
}

// reopenFinding puts a closed finding back in progress.
func reopenFinding(f *Finding, note string) error {
	if f.Status != FindingClosed {
		return noTransition("only a closed finding can be reopened")
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return invalid("explain why the finding is reopened")
	}
	if len(note) > 2000 {
		return invalid("note must be at most 2000 characters")
	}
	f.Status, f.ReopenNote = FindingInProgress, note
	f.ClosedByEmail, f.ClosedAt = "", nil
	return nil
}

// ── Store ──

// InternalAuditStore persists audits with their findings, scoped to one tenant.
type InternalAuditStore interface {
	List(ctx context.Context, tenantID string) ([]InternalAudit, error)
	Get(ctx context.Context, tenantID, id string) (*InternalAudit, error)
	Create(ctx context.Context, a *InternalAudit) error // assigns a.ID
	// Mutate loads the audit, applies change and saves the audit, its
	// control links and its findings atomically, with the audit note
	// returned by change. If change fails nothing is saved.
	Mutate(ctx context.Context, tenantID, id string, change func(a *InternalAudit) (AuditNote, error)) (*InternalAudit, error)
	Delete(ctx context.Context, tenantID, id string) error
}

// ── Handler ──

// InternalAuditHandler serves audits and findings.
//
// Everyone in the organization can read. Owners, admins and editors plan
// and run audits, record findings and work on action plans. Only owners
// and admins cancel or delete audits and close or reopen findings.
type InternalAuditHandler struct {
	store  InternalAuditStore
	logger *slog.Logger
}

// NewInternalAuditHandler creates the handler.
func NewInternalAuditHandler(store InternalAuditStore, logger *slog.Logger) *InternalAuditHandler {
	return &InternalAuditHandler{store: store, logger: logger}
}

// Register wires the audit routes into the given mux.
func (h *InternalAuditHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/audits", h.handleList)
	mux.HandleFunc("POST /v1/audits", h.handleCreate)
	mux.HandleFunc("GET /v1/audits/{id}", h.handleGet)
	mux.HandleFunc("PUT /v1/audits/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /v1/audits/{id}", h.handleDelete)
	mux.HandleFunc("POST /v1/audits/{id}/start", h.handleStart)
	mux.HandleFunc("POST /v1/audits/{id}/complete", h.handleComplete)
	mux.HandleFunc("POST /v1/audits/{id}/cancel", h.handleCancel)
	mux.HandleFunc("POST /v1/audits/{id}/findings", h.handleAddFinding)
	mux.HandleFunc("PUT /v1/audits/{id}/findings/{finding}", h.handleUpdateFinding)
	mux.HandleFunc("DELETE /v1/audits/{id}/findings/{finding}", h.handleDeleteFinding)
	mux.HandleFunc("PUT /v1/audits/{id}/findings/{finding}/action", h.handlePlan)
	mux.HandleFunc("POST /v1/audits/{id}/findings/{finding}/close", h.handleClose)
	mux.HandleFunc("POST /v1/audits/{id}/findings/{finding}/reopen", h.handleReopen)
}

func (h *InternalAuditHandler) fail(w http.ResponseWriter, err error, what string) {
	var te *errTransition
	var ve *validationError
	switch {
	case errors.As(err, &te):
		writeError(w, http.StatusConflict, te.msg)
	case errors.As(err, &ve):
		writeError(w, http.StatusUnprocessableEntity, ve.msg)
	default:
		if writeStoreError(w, err, "audit or finding") {
			h.logger.Error("audit: "+what+" failed", "error", err)
		}
	}
}

// writer returns the user if they may change audits, else answers 403.
func (h *InternalAuditHandler) writer(w http.ResponseWriter, r *http.Request) *auth.User {
	user := auth.FromContext(r.Context())
	if !user.CanWrite() {
		writeError(w, http.StatusForbidden, "your role does not allow changes")
		return nil
	}
	return user
}

// manager returns the user if they are an owner or admin, else answers 403.
func (h *InternalAuditHandler) manager(w http.ResponseWriter, r *http.Request, what string) *auth.User {
	user := auth.FromContext(r.Context())
	if !user.CanManageMembers() {
		writeError(w, http.StatusForbidden, "only owners and administrators can "+what)
		return nil
	}
	return user
}

// mutate applies change to the audit in the path and writes the result.
func (h *InternalAuditHandler) mutate(w http.ResponseWriter, r *http.Request, user *auth.User, status int,
	change func(a *InternalAudit) (AuditNote, error)) {
	updated, err := h.store.Mutate(r.Context(), user.TenantID, r.PathValue("id"), change)
	if err != nil {
		h.fail(w, err, "change")
		return
	}
	writeJSON(w, status, updated)
}

func (h *InternalAuditHandler) handleList(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	audits, err := h.store.List(r.Context(), user.TenantID)
	if err != nil {
		h.fail(w, err, "list")
		return
	}
	// Most recent first; audits without a date (still being planned) on top.
	sort.Slice(audits, func(i, j int) bool {
		if audits[i].PlannedDate != audits[j].PlannedDate {
			if audits[i].PlannedDate == "" || audits[j].PlannedDate == "" {
				return audits[i].PlannedDate == ""
			}
			return audits[i].PlannedDate > audits[j].PlannedDate
		}
		return audits[i].CreatedAt.After(audits[j].CreatedAt)
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": audits, "count": len(audits)})
}

func (h *InternalAuditHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	a, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// auditFields is what describes an audit before it is completed.
type auditFields struct {
	Title       string   `json:"title"`
	Type        string   `json:"audit_type"`
	Scope       string   `json:"scope"`
	Criteria    string   `json:"criteria"`
	LeadAuditor string   `json:"lead_auditor"`
	PlannedDate string   `json:"planned_date"`
	Controls    []string `json:"controls"`
}

func (f *auditFields) applyTo(a *InternalAudit) error {
	title := strings.TrimSpace(f.Title)
	if title == "" {
		return invalid("title is required")
	}
	if len(title) > 300 {
		return invalid("title must be at most 300 characters")
	}
	if f.Type == "" {
		f.Type = "internal"
	}
	if !oneOf(f.Type, auditTypes) {
		return invalid("audit_type must be one of: %s", strings.Join(auditTypes, ", "))
	}
	if len(f.Scope) > 4000 || len(f.Criteria) > 4000 {
		return invalid("scope and criteria must be at most 4000 characters")
	}
	if len(f.LeadAuditor) > 200 {
		return invalid("lead_auditor must be at most 200 characters")
	}
	planned, err := parseDate("planned_date", f.PlannedDate)
	if err != nil {
		return err
	}
	controls, err := normalizeControls(f.Controls)
	if err != nil {
		return err
	}
	a.Title, a.Type = title, f.Type
	a.Scope, a.Criteria = strings.TrimSpace(f.Scope), strings.TrimSpace(f.Criteria)
	a.LeadAuditor, a.PlannedDate, a.Controls = strings.TrimSpace(f.LeadAuditor), planned, controls
	return nil
}

func (h *InternalAuditHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	user := h.writer(w, r)
	if user == nil {
		return
	}
	var input auditFields
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	now := time.Now().UTC()
	a := InternalAudit{TenantID: user.TenantID, Status: AuditPlanned, CreatedByEmail: user.Email, CreatedAt: now, UpdatedAt: now}
	if err := input.applyTo(&a); err != nil {
		h.fail(w, err, "create")
		return
	}
	if err := h.store.Create(r.Context(), &a); err != nil {
		h.fail(w, err, "create")
		return
	}
	a.refresh()
	writeJSON(w, http.StatusCreated, a)
}

func (h *InternalAuditHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	user := h.writer(w, r)
	if user == nil {
		return
	}
	var input auditFields
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		if !a.editable() {
			return AuditNote{}, noTransition("a %s audit cannot be edited", a.Status)
		}
		return AuditNote{"audit.update", map[string]any{"title": a.Title}}, input.applyTo(a)
	})
}

func (h *InternalAuditHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	user := h.manager(w, r, "delete audits")
	if user == nil {
		return
	}
	existing, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	// Only an audit that never started leaves no history worth keeping.
	if existing.Status != AuditPlanned {
		writeError(w, http.StatusConflict, "only a planned audit can be deleted; cancel it to keep its record")
		return
	}
	if err := h.store.Delete(r.Context(), user.TenantID, existing.ID); err != nil {
		h.fail(w, err, "delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *InternalAuditHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	user := h.writer(w, r)
	if user == nil {
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		return AuditNote{"audit.start", map[string]any{"title": a.Title}}, a.start()
	})
}

func (h *InternalAuditHandler) handleComplete(w http.ResponseWriter, r *http.Request) {
	user := h.writer(w, r)
	if user == nil {
		return
	}
	var input struct {
		Summary       string `json:"summary"`
		CompletedDate string `json:"completed_date"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		err := a.complete(input.Summary, input.CompletedDate)
		return AuditNote{"audit.complete", map[string]any{"title": a.Title, "findings": len(a.Findings)}}, err
	})
}

func (h *InternalAuditHandler) handleCancel(w http.ResponseWriter, r *http.Request) {
	user := h.manager(w, r, "cancel audits")
	if user == nil {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		err := a.cancel(input.Reason)
		return AuditNote{"audit.cancel", map[string]any{"title": a.Title, "reason": a.CancelReason}}, err
	})
}

func findingRef(a *InternalAudit, f *Finding) map[string]any {
	return map[string]any{"audit": a.Title, "finding": f.Number, "severity": f.Severity}
}

func (h *InternalAuditHandler) handleAddFinding(w http.ResponseWriter, r *http.Request) {
	user := h.writer(w, r)
	if user == nil {
		return
	}
	var input findingContent
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := input.normalize(); err != nil {
		h.fail(w, err, "add finding")
		return
	}
	now := time.Now().UTC()
	h.mutate(w, r, user, http.StatusCreated, func(a *InternalAudit) (AuditNote, error) {
		f := Finding{ID: newUUID(), CreatedByEmail: user.Email, CreatedAt: now, UpdatedAt: now}
		input.applyTo(&f)
		added, err := a.addFinding(f)
		if err != nil {
			return AuditNote{}, err
		}
		return AuditNote{"finding.create", findingRef(a, added)}, nil
	})
}

func (h *InternalAuditHandler) handleUpdateFinding(w http.ResponseWriter, r *http.Request) {
	user := h.writer(w, r)
	if user == nil {
		return
	}
	var input findingContent
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := input.normalize(); err != nil {
		h.fail(w, err, "update finding")
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		if err := a.contentChangeable(); err != nil {
			return AuditNote{}, err
		}
		f := a.finding(r.PathValue("finding"))
		if f == nil {
			return AuditNote{}, ErrNotFound
		}
		input.applyTo(f)
		f.UpdatedAt = time.Now().UTC()
		return AuditNote{"finding.update", findingRef(a, f)}, nil
	})
}

func (h *InternalAuditHandler) handleDeleteFinding(w http.ResponseWriter, r *http.Request) {
	user := h.writer(w, r)
	if user == nil {
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		f := a.finding(r.PathValue("finding"))
		if f == nil {
			return AuditNote{}, ErrNotFound
		}
		note := AuditNote{"finding.delete", findingRef(a, f)}
		note.Detail["description"] = f.Description
		return note, a.removeFinding(f.ID)
	})
}

func (h *InternalAuditHandler) handlePlan(w http.ResponseWriter, r *http.Request) {
	user := h.writer(w, r)
	if user == nil {
		return
	}
	var input findingPlan
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		f, err := a.followUp(r.PathValue("finding"))
		if err != nil {
			return AuditNote{}, err
		}
		before := f.Status
		if err := input.applyTo(f); err != nil {
			return AuditNote{}, err
		}
		f.UpdatedAt = time.Now().UTC()
		note := AuditNote{"finding.plan", findingRef(a, f)}
		note.Detail["before"], note.Detail["after"] = before, f.Status
		note.Detail["due_date"], note.Detail["responsible"] = f.DueDate, f.Responsible
		return note, nil
	})
}

func (h *InternalAuditHandler) handleClose(w http.ResponseWriter, r *http.Request) {
	user := h.manager(w, r, "close findings")
	if user == nil {
		return
	}
	var input struct {
		Verification string `json:"verification"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		f, err := a.followUp(r.PathValue("finding"))
		if err != nil {
			return AuditNote{}, err
		}
		now := time.Now().UTC()
		if err := closeFinding(f, input.Verification, user, now); err != nil {
			return AuditNote{}, err
		}
		f.UpdatedAt = now
		note := AuditNote{"finding.close", findingRef(a, f)}
		note.Detail["verification"] = f.Verification
		return note, nil
	})
}

func (h *InternalAuditHandler) handleReopen(w http.ResponseWriter, r *http.Request) {
	user := h.manager(w, r, "reopen findings")
	if user == nil {
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.mutate(w, r, user, http.StatusOK, func(a *InternalAudit) (AuditNote, error) {
		f, err := a.followUp(r.PathValue("finding"))
		if err != nil {
			return AuditNote{}, err
		}
		if err := reopenFinding(f, input.Note); err != nil {
			return AuditNote{}, err
		}
		f.UpdatedAt = time.Now().UTC()
		note := AuditNote{"finding.reopen", findingRef(a, f)}
		note.Detail["note"] = f.ReopenNote
		return note, nil
	})
}

// ── In-Memory Store (development) ──

// MemoryInternalAuditStore is a thread-safe in-memory audit store.
type MemoryInternalAuditStore struct {
	t *memTable[InternalAudit]
}

// NewMemoryInternalAuditStore creates an empty in-memory store.
func NewMemoryInternalAuditStore() *MemoryInternalAuditStore {
	return &MemoryInternalAuditStore{t: newMemTable[InternalAudit]()}
}

// cloneAudit copies the slices so callers cannot alter stored data.
func cloneAudit(a InternalAudit) InternalAudit {
	a.Findings = append([]Finding(nil), a.Findings...)
	a.Controls = append([]string(nil), a.Controls...)
	a.refresh()
	return a
}

func (m *MemoryInternalAuditStore) List(_ context.Context, tenantID string) ([]InternalAudit, error) {
	audits := m.t.list(tenantID)
	for i := range audits {
		audits[i] = cloneAudit(audits[i])
	}
	return audits, nil
}

func (m *MemoryInternalAuditStore) Get(_ context.Context, tenantID, id string) (*InternalAudit, error) {
	a, ok := m.t.get(tenantID, id)
	if !ok {
		return nil, ErrNotFound
	}
	a = cloneAudit(a)
	return &a, nil
}

func (m *MemoryInternalAuditStore) Create(_ context.Context, a *InternalAudit) error {
	a.ID = newUUID()
	return m.t.insert(a.TenantID, a.ID, cloneAudit(*a), nil)
}

func (m *MemoryInternalAuditStore) Mutate(_ context.Context, tenantID, id string, change func(a *InternalAudit) (AuditNote, error)) (*InternalAudit, error) {
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	stored, ok := m.t.rows[tenantID][id]
	if !ok {
		return nil, ErrNotFound
	}
	working := cloneAudit(stored)
	if _, err := change(&working); err != nil {
		return nil, err
	}
	working.UpdatedAt = time.Now().UTC()
	working.refresh()
	m.t.rows[tenantID][id] = cloneAudit(working)
	return &working, nil
}

func (m *MemoryInternalAuditStore) Delete(_ context.Context, tenantID, id string) error {
	return m.t.remove(tenantID, id)
}
