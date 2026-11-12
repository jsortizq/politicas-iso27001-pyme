package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/catalog"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// ── SoA Entry domain model ──

// SoAEntry represents a Statement of Applicability entry with
// three-dimension assessment (ADR-004).
type SoAEntry struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id,omitempty"`
	ControlID         string     `json:"control_id"`
	ControlTitle      string     `json:"control_title,omitempty"`
	Applicable        bool       `json:"applicable"`
	Justification     string     `json:"justification"`
	ImplementationPct int        `json:"implementation_pct"`
	ImplementationEv  string     `json:"implementation_evidence"`
	EffectivenessPct  int        `json:"effectiveness_pct"`
	EffectivenessEv   string     `json:"effectiveness_evidence"`
	ResponsibleRole   string     `json:"responsible_role"`
	ReviewDate        *time.Time `json:"review_date,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// SoAStore is the interface for SoA persistence.
// Every method is scoped to one tenant; implementations must never return
// or modify another tenant's entries. A control appears at most once per
// tenant (ErrConflict otherwise).
type SoAStore interface {
	List(ctx context.Context, tenantID string) ([]SoAEntry, error)
	Get(ctx context.Context, tenantID, id string) (*SoAEntry, error)
	Create(ctx context.Context, entry *SoAEntry) error // assigns entry.ID
	Update(ctx context.Context, entry *SoAEntry) error
	Delete(ctx context.Context, tenantID, id string) error
}

// SoAHandler handles SoA REST API requests.
type SoAHandler struct {
	store  SoAStore
	logger *slog.Logger
}

// NewSoAHandler creates a handler with the given store and logger.
func NewSoAHandler(store SoAStore, logger *slog.Logger) *SoAHandler {
	return &SoAHandler{store: store, logger: logger}
}

// Register wires the SoA routes into the given mux.
// All routes expect tenant context from middleware.
func (h *SoAHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/catalog/controls", h.handleCatalog)
	mux.HandleFunc("GET /v1/soa", h.handleList)
	mux.HandleFunc("POST /v1/soa", h.handleCreate)
	mux.HandleFunc("POST /v1/soa/initialize", h.handleInitialize)
	mux.HandleFunc("GET /v1/soa/summary", h.handleSummary)
	mux.HandleFunc("GET /v1/soa/{id}", h.handleGet)
	mux.HandleFunc("PUT /v1/soa/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /v1/soa/{id}", h.handleDelete)
}

// ── Handlers ──

func (h *SoAHandler) handleCatalog(w http.ResponseWriter, r *http.Request) {
	controls, err := catalog.Controls()
	if err != nil {
		h.logger.Error("catalog load failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"standard": catalog.Standard,
		"data":     controls,
		"count":    len(controls),
	})
}

func (h *SoAHandler) handleList(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	entries, err := h.store.List(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("soa list failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to list SoA entries")
		return
	}
	decorate(entries)

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  entries,
		"count": len(entries),
	})
}

func (h *SoAHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	entry, err := h.store.Get(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		if writeStoreError(w, err, "SoA entry") {
			h.logger.Error("soa get failed", "error", err, "tenant", tenantID)
		}
		return
	}
	entry.ControlTitle = controlTitle(entry.ControlID)

	writeJSON(w, http.StatusOK, entry)
}

func (h *SoAHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	var input SoAEntry
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateSoAEntry(&input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	input.ID = ""
	input.TenantID = tenantID
	now := time.Now().UTC()
	input.CreatedAt = now
	input.UpdatedAt = now

	if err := h.store.Create(r.Context(), &input); err != nil {
		if writeStoreError(w, err, "SoA entry for this control") {
			h.logger.Error("soa create failed", "error", err, "tenant", tenantID)
		}
		return
	}
	input.ControlTitle = controlTitle(input.ControlID)

	writeJSON(w, http.StatusCreated, input)
}

// handleInitialize creates one entry per catalog control that the tenant
// does not have yet. It is idempotent: existing assessments are untouched.
func (h *SoAHandler) handleInitialize(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	controls, err := catalog.Controls()
	if err != nil {
		h.logger.Error("catalog load failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	existing, err := h.store.List(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("soa initialize failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to initialize SoA")
		return
	}
	have := make(map[string]bool, len(existing))
	for _, e := range existing {
		have[e.ControlID] = true
	}

	created := 0
	now := time.Now().UTC()
	for _, c := range controls {
		if have[c.ID] {
			continue
		}
		entry := SoAEntry{
			TenantID:   tenantID,
			ControlID:  c.ID,
			Applicable: true,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		err := h.store.Create(r.Context(), &entry)
		if errors.Is(err, ErrConflict) {
			continue // created concurrently
		}
		if err != nil {
			h.logger.Error("soa initialize failed", "error", err, "tenant", tenantID, "control", c.ID)
			writeError(w, http.StatusInternalServerError, "failed to initialize SoA")
			return
		}
		created++
	}

	writeJSON(w, http.StatusOK, map[string]int{
		"created": created,
		"total":   len(existing) + created,
	})
}

func (h *SoAHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	existing, err := h.store.Get(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		if writeStoreError(w, err, "SoA entry") {
			h.logger.Error("soa get failed", "error", err, "tenant", tenantID)
		}
		return
	}

	var input SoAEntry
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateSoAEntry(&input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Preserve immutable fields
	input.ID = existing.ID
	input.TenantID = existing.TenantID
	input.CreatedAt = existing.CreatedAt
	input.UpdatedAt = time.Now().UTC()

	if err := h.store.Update(r.Context(), &input); err != nil {
		if writeStoreError(w, err, "SoA entry") {
			h.logger.Error("soa update failed", "error", err, "tenant", tenantID)
		}
		return
	}
	input.ControlTitle = controlTitle(input.ControlID)

	writeJSON(w, http.StatusOK, input)
}

func (h *SoAHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	if err := h.store.Delete(r.Context(), tenantID, r.PathValue("id")); err != nil {
		if writeStoreError(w, err, "SoA entry") {
			h.logger.Error("soa delete failed", "error", err, "tenant", tenantID)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SoAHandler) handleSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	entries, err := h.store.List(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("soa summary failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to compute summary")
		return
	}

	writeJSON(w, http.StatusOK, computeSummary(entries))
}

// ── Presentation helpers ──

func controlTitle(id string) string {
	if c, ok := catalog.Lookup(id); ok {
		return c.ClauseES
	}
	return ""
}

// decorate fills control titles from the catalog and sorts entries in
// natural control order (A.5.2 before A.5.10).
func decorate(entries []SoAEntry) {
	for i := range entries {
		entries[i].ControlTitle = controlTitle(entries[i].ControlID)
	}
	sort.Slice(entries, func(i, j int) bool {
		return controlLess(entries[i].ControlID, entries[j].ControlID)
	})
}

func controlLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 1; i < len(pa) && i < len(pb); i++ {
		na, _ := strconv.Atoi(pa[i])
		nb, _ := strconv.Atoi(pb[i])
		if na != nb {
			return na < nb
		}
	}
	return a < b
}

// ── Summary computation ──

// SoASummary aggregates SoA metrics. Averages consider applicable controls only.
type SoASummary struct {
	TotalControls   int     `json:"total_controls"`
	Applicable      int     `json:"applicable"`
	NotApplicable   int     `json:"not_applicable"`
	Implemented     int     `json:"implemented"`
	Partial         int     `json:"partial"`
	NotImplemented  int     `json:"not_implemented"`
	AvgImplement    float64 `json:"avg_implementation_pct"`
	AvgEffective    float64 `json:"avg_effectiveness_pct"`
	ComplianceScore float64 `json:"compliance_score_pct"`
}

func computeSummary(entries []SoAEntry) SoASummary {
	s := SoASummary{TotalControls: len(entries)}

	var sumImpl, sumEff int
	for _, e := range entries {
		if !e.Applicable {
			s.NotApplicable++
			continue
		}
		s.Applicable++
		sumImpl += e.ImplementationPct
		sumEff += e.EffectivenessPct

		switch {
		case e.ImplementationPct >= 100:
			s.Implemented++
		case e.ImplementationPct > 0:
			s.Partial++
		default:
			s.NotImplemented++
		}
	}

	if s.Applicable > 0 {
		s.AvgImplement = float64(sumImpl) / float64(s.Applicable)
		s.AvgEffective = float64(sumEff) / float64(s.Applicable)
		s.ComplianceScore = (s.AvgImplement + s.AvgEffective) / 2
	}
	return s
}

// ── Validation ──

var controlIDPattern = regexp.MustCompile(`^A\.\d+\.\d+$`)

func validateSoAEntry(e *SoAEntry) error {
	if e.ControlID == "" {
		return invalid("control_id is required")
	}
	if !controlIDPattern.MatchString(e.ControlID) {
		return invalid("control_id must match pattern A.X.Y (e.g. A.5.1)")
	}
	if _, ok := catalog.Lookup(e.ControlID); !ok {
		return invalid("control_id %s is not in the ISO/IEC 27001:2022 Annex A catalog", e.ControlID)
	}
	if e.ImplementationPct < 0 || e.ImplementationPct > 100 {
		return invalid("implementation_pct must be 0-100")
	}
	if e.EffectivenessPct < 0 || e.EffectivenessPct > 100 {
		return invalid("effectiveness_pct must be 0-100")
	}
	if !e.Applicable && strings.TrimSpace(e.Justification) == "" {
		return invalid("justification is required when a control is not applicable")
	}
	return nil
}

// ── In-Memory Store (development) ──

// MemorySoAStore is a thread-safe in-memory SoA store for development.
type MemorySoAStore struct {
	t *memTable[SoAEntry]
}

// NewMemorySoAStore creates an empty in-memory store.
func NewMemorySoAStore() *MemorySoAStore {
	return &MemorySoAStore{t: newMemTable[SoAEntry]()}
}

func (m *MemorySoAStore) List(_ context.Context, tenantID string) ([]SoAEntry, error) {
	return m.t.list(tenantID), nil
}

func (m *MemorySoAStore) Get(_ context.Context, tenantID, id string) (*SoAEntry, error) {
	entry, ok := m.t.get(tenantID, id)
	if !ok {
		return nil, ErrNotFound
	}
	return &entry, nil
}

func (m *MemorySoAStore) Create(_ context.Context, entry *SoAEntry) error {
	entry.ID = newUUID()
	return m.t.insert(entry.TenantID, entry.ID, *entry, func(existing SoAEntry) bool {
		return existing.ControlID == entry.ControlID
	})
}

func (m *MemorySoAStore) Update(_ context.Context, entry *SoAEntry) error {
	return m.t.replace(entry.TenantID, entry.ID, *entry, func(_ string, other SoAEntry) bool {
		return other.ControlID == entry.ControlID
	})
}

func (m *MemorySoAStore) Delete(_ context.Context, tenantID, id string) error {
	return m.t.remove(tenantID, id)
}
