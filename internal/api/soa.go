// Package api provides HTTP handlers for the SGSI REST API.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

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
// In-memory implementation is provided for development;
// PostgreSQL implementation will come with database integration.
type SoAStore interface {
	List(tenantID string) ([]SoAEntry, error)
	Get(tenantID, id string) (*SoAEntry, error)
	Create(entry *SoAEntry) error
	Update(entry *SoAEntry) error
	Delete(tenantID, id string) error
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
	mux.HandleFunc("GET /v1/soa", h.handleList)
	mux.HandleFunc("POST /v1/soa", h.handleCreate)
	mux.HandleFunc("GET /v1/soa/{id}", h.handleGet)
	mux.HandleFunc("PUT /v1/soa/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /v1/soa/{id}", h.handleDelete)
	mux.HandleFunc("GET /v1/soa/summary", h.handleSummary)
}

// ── Handlers ──

func (h *SoAHandler) handleList(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	entries, err := h.store.List(tenantID)
	if err != nil {
		h.logger.Error("soa list failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to list SoA entries")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  entries,
		"count": len(entries),
	})
}

func (h *SoAHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())
	id := r.PathValue("id")

	entry, err := h.store.Get(tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "SoA entry not found")
		return
	}

	writeJSON(w, http.StatusOK, entry)
}

func (h *SoAHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	var input SoAEntry
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := validateSoAEntry(&input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	input.TenantID = tenantID
	now := time.Now().UTC()
	input.CreatedAt = now
	input.UpdatedAt = now

	if err := h.store.Create(&input); err != nil {
		h.logger.Error("soa create failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to create SoA entry")
		return
	}

	writeJSON(w, http.StatusCreated, input)
}

func (h *SoAHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())
	id := r.PathValue("id")

	existing, err := h.store.Get(tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "SoA entry not found")
		return
	}

	var input SoAEntry
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
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

	if err := h.store.Update(&input); err != nil {
		h.logger.Error("soa update failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to update SoA entry")
		return
	}

	writeJSON(w, http.StatusOK, input)
}

func (h *SoAHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())
	id := r.PathValue("id")

	if err := h.store.Delete(tenantID, id); err != nil {
		writeError(w, http.StatusNotFound, "SoA entry not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SoAHandler) handleSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	entries, err := h.store.List(tenantID)
	if err != nil {
		h.logger.Error("soa summary failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to compute summary")
		return
	}

	summary := computeSummary(entries)
	writeJSON(w, http.StatusOK, summary)
}

// ── Summary computation ──

// SoASummary aggregates SoA metrics.
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
	if len(entries) == 0 {
		return s
	}

	var sumImpl, sumEff int
	applicableCount := 0

	for _, e := range entries {
		if e.Applicable {
			s.Applicable++
			applicableCount++
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
		} else {
			s.NotApplicable++
		}
	}

	if applicableCount > 0 {
		s.AvgImplement = float64(sumImpl) / float64(applicableCount)
		s.AvgEffective = float64(sumEff) / float64(applicableCount)
		s.ComplianceScore = (s.AvgImplement + s.AvgEffective) / 2
	}

	return s
}

// ── Validation ──

var controlIDPattern = regexp.MustCompile(`^A\.\d+\.\d+$`)

func validateSoAEntry(e *SoAEntry) error {
	if e.ControlID == "" {
		return &validationError{"control_id is required"}
	}
	if !controlIDPattern.MatchString(e.ControlID) {
		return &validationError{"control_id must match pattern A.X.Y (e.g. A.5.1)"}
	}
	if e.ImplementationPct < 0 || e.ImplementationPct > 100 {
		return &validationError{"implementation_pct must be 0-100"}
	}
	if e.EffectivenessPct < 0 || e.EffectivenessPct > 100 {
		return &validationError{"effectiveness_pct must be 0-100"}
	}
	return nil
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string {
	return e.msg
}

// ── JSON helpers ──

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ── In-Memory Store (development) ──

// MemorySoAStore is a thread-safe in-memory SoA store for development.
type MemorySoAStore struct {
	entries map[string]map[string]SoAEntry // tenant_id -> id -> entry
	nextSeq int
}

// NewMemorySoAStore creates an empty in-memory store.
func NewMemorySoAStore() *MemorySoAStore {
	return &MemorySoAStore{
		entries: make(map[string]map[string]SoAEntry),
	}
}

func (m *MemorySoAStore) List(tenantID string) ([]SoAEntry, error) {
	tenant, ok := m.entries[tenantID]
	if !ok {
		return []SoAEntry{}, nil
	}
	result := make([]SoAEntry, 0, len(tenant))
	for _, e := range tenant {
		result = append(result, e)
	}
	return result, nil
}

func (m *MemorySoAStore) Get(tenantID, id string) (*SoAEntry, error) {
	tenant, ok := m.entries[tenantID]
	if !ok {
		return nil, &notFoundError{id}
	}
	entry, ok := tenant[id]
	if !ok {
		return nil, &notFoundError{id}
	}
	return &entry, nil
}

func (m *MemorySoAStore) Create(entry *SoAEntry) error {
	if _, ok := m.entries[entry.TenantID]; !ok {
		m.entries[entry.TenantID] = make(map[string]SoAEntry)
	}
	m.nextSeq++
	entry.ID = generateID(m.nextSeq)
	m.entries[entry.TenantID][entry.ID] = *entry
	return nil
}

func (m *MemorySoAStore) Update(entry *SoAEntry) error {
	tenant, ok := m.entries[entry.TenantID]
	if !ok {
		return &notFoundError{entry.ID}
	}
	if _, ok := tenant[entry.ID]; !ok {
		return &notFoundError{entry.ID}
	}
	tenant[entry.ID] = *entry
	return nil
}

func (m *MemorySoAStore) Delete(tenantID, id string) error {
	tenant, ok := m.entries[tenantID]
	if !ok {
		return &notFoundError{id}
	}
	if _, ok := tenant[id]; !ok {
		return &notFoundError{id}
	}
	delete(tenant, id)
	return nil
}

type notFoundError struct {
	id string
}

func (e *notFoundError) Error() string {
	return "entry not found: " + e.id
}

func generateID(seq int) string {
	return strings.ToLower(
		strings.ReplaceAll(
			time.Now().Format("20060102")+"-"+padInt(seq),
			" ", "",
		),
	)
}

func padInt(n int) string {
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if s == "" {
		return "0"
	}
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}
