package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// ── Risk domain model ──

// Risk is an entry of the risk register (clauses 6.1.2 and 6.1.3).
// The inherent level is likelihood × impact (1–25). The residual level is
// computed the same way once both residual values are recorded.
type Risk struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id,omitempty"`
	Code               string    `json:"code"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Asset              string    `json:"asset"`
	Threat             string    `json:"threat"`
	Vulnerability      string    `json:"vulnerability"`
	Likelihood         int       `json:"likelihood"`
	Impact             int       `json:"impact"`
	Level              int       `json:"risk_level"`
	Rating             string    `json:"risk_rating"`
	Treatment          string    `json:"treatment"`
	TreatmentPlan      string    `json:"treatment_plan"`
	ResidualLikelihood *int      `json:"residual_likelihood,omitempty"`
	ResidualImpact     *int      `json:"residual_impact,omitempty"`
	ResidualLevel      *int      `json:"residual_level,omitempty"`
	ResidualRating     string    `json:"residual_rating,omitempty"`
	Status             string    `json:"status"`
	ReviewDate         string    `json:"review_date,omitempty"` // YYYY-MM-DD
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

var (
	riskTreatments = []string{"accept", "mitigate", "transfer", "avoid"}
	riskStatuses   = []string{"open", "in_treatment", "accepted", "closed"}
	riskCodeRe     = regexp.MustCompile(`^R-(\d+)$`)
)

// RiskRating maps a 1–25 level onto the bands of the 5×5 matrix.
func RiskRating(level int) string {
	switch {
	case level >= 16:
		return "critical"
	case level >= 10:
		return "high"
	case level >= 5:
		return "medium"
	default:
		return "low"
	}
}

// RiskStore is the persistence interface for the risk register.
// Codes are unique per tenant (ErrConflict otherwise). When Create receives
// an empty code, the store assigns the next free "R-NNN".
type RiskStore interface {
	List(ctx context.Context, tenantID string) ([]Risk, error)
	Get(ctx context.Context, tenantID, id string) (*Risk, error)
	Create(ctx context.Context, risk *Risk) error // assigns risk.ID and, if empty, risk.Code
	Update(ctx context.Context, risk *Risk) error
	Delete(ctx context.Context, tenantID, id string) error
}

// RiskHandler handles risk register requests.
type RiskHandler struct {
	store  RiskStore
	logger *slog.Logger
}

// NewRiskHandler creates a handler with the given store and logger.
func NewRiskHandler(store RiskStore, logger *slog.Logger) *RiskHandler {
	return &RiskHandler{store: store, logger: logger}
}

// Register wires the risk routes into the given mux.
func (h *RiskHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/risks", h.handleList)
	mux.HandleFunc("POST /v1/risks", h.handleCreate)
	mux.HandleFunc("GET /v1/risks/{id}", h.handleGet)
	mux.HandleFunc("PUT /v1/risks/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /v1/risks/{id}", h.handleDelete)
}

func (h *RiskHandler) handleList(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	risks, err := h.store.List(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("risk list failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to list risks")
		return
	}
	// Highest inherent level first, then by code.
	sort.Slice(risks, func(i, j int) bool {
		if risks[i].Level != risks[j].Level {
			return risks[i].Level > risks[j].Level
		}
		return risks[i].Code < risks[j].Code
	})

	writeJSON(w, http.StatusOK, map[string]any{"data": risks, "count": len(risks)})
}

func (h *RiskHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	risk, err := h.store.Get(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		if writeStoreError(w, err, "risk") {
			h.logger.Error("risk get failed", "error", err, "tenant", tenantID)
		}
		return
	}
	writeJSON(w, http.StatusOK, risk)
}

func (h *RiskHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	var input Risk
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := normalizeRisk(&input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	input.ID = ""
	input.TenantID = tenantID
	now := time.Now().UTC()
	input.CreatedAt = now
	input.UpdatedAt = now

	if err := h.store.Create(r.Context(), &input); err != nil {
		if writeStoreError(w, err, "risk with this code") {
			h.logger.Error("risk create failed", "error", err, "tenant", tenantID)
		}
		return
	}
	writeJSON(w, http.StatusCreated, input)
}

func (h *RiskHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	existing, err := h.store.Get(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		if writeStoreError(w, err, "risk") {
			h.logger.Error("risk get failed", "error", err, "tenant", tenantID)
		}
		return
	}

	var input Risk
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if input.Code == "" {
		input.Code = existing.Code
	}
	if err := normalizeRisk(&input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	input.ID = existing.ID
	input.TenantID = existing.TenantID
	input.CreatedAt = existing.CreatedAt
	input.UpdatedAt = time.Now().UTC()

	if err := h.store.Update(r.Context(), &input); err != nil {
		if writeStoreError(w, err, "risk with this code") {
			h.logger.Error("risk update failed", "error", err, "tenant", tenantID)
		}
		return
	}
	writeJSON(w, http.StatusOK, input)
}

func (h *RiskHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	if err := h.store.Delete(r.Context(), tenantID, r.PathValue("id")); err != nil {
		if writeStoreError(w, err, "risk") {
			h.logger.Error("risk delete failed", "error", err, "tenant", tenantID)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// normalizeRisk applies defaults, validates the input and computes the
// derived levels and ratings.
func normalizeRisk(r *Risk) error {
	r.Code = strings.TrimSpace(r.Code)
	r.Title = strings.TrimSpace(r.Title)
	if r.Title == "" {
		return invalid("title is required")
	}
	if len(r.Title) > 300 {
		return invalid("title must be at most 300 characters")
	}
	if len(r.Code) > 30 {
		return invalid("code must be at most 30 characters")
	}
	if r.Likelihood < 1 || r.Likelihood > 5 {
		return invalid("likelihood must be 1-5")
	}
	if r.Impact < 1 || r.Impact > 5 {
		return invalid("impact must be 1-5")
	}
	if r.Treatment == "" {
		r.Treatment = "mitigate"
	}
	if !oneOf(r.Treatment, riskTreatments) {
		return invalid("treatment must be one of: %s", strings.Join(riskTreatments, ", "))
	}
	if r.Status == "" {
		r.Status = "open"
	}
	if !oneOf(r.Status, riskStatuses) {
		return invalid("status must be one of: %s", strings.Join(riskStatuses, ", "))
	}
	if (r.ResidualLikelihood == nil) != (r.ResidualImpact == nil) {
		return invalid("residual_likelihood and residual_impact must be provided together")
	}
	if r.ResidualLikelihood != nil {
		if *r.ResidualLikelihood < 1 || *r.ResidualLikelihood > 5 {
			return invalid("residual_likelihood must be 1-5")
		}
		if *r.ResidualImpact < 1 || *r.ResidualImpact > 5 {
			return invalid("residual_impact must be 1-5")
		}
	}
	if r.ReviewDate != "" {
		if _, err := time.Parse("2006-01-02", r.ReviewDate); err != nil {
			return invalid("review_date must be YYYY-MM-DD")
		}
	}
	r.derive()
	return nil
}

// derive fills the computed fields from the stored ones.
func (r *Risk) derive() {
	r.Level = r.Likelihood * r.Impact
	r.Rating = RiskRating(r.Level)
	r.ResidualLevel = nil
	r.ResidualRating = ""
	if r.ResidualLikelihood != nil && r.ResidualImpact != nil {
		level := *r.ResidualLikelihood * *r.ResidualImpact
		r.ResidualLevel = &level
		r.ResidualRating = RiskRating(level)
	}
}

// nextRiskCode returns the next sequential "R-NNN" after the given codes.
func nextRiskCode(codes []string) string {
	highest := 0
	for _, c := range codes {
		if m := riskCodeRe.FindStringSubmatch(c); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > highest {
				highest = n
			}
		}
	}
	return fmt.Sprintf("R-%03d", highest+1)
}

// ── In-Memory Store (development) ──

// MemoryRiskStore is a thread-safe in-memory risk store for development.
type MemoryRiskStore struct {
	t *memTable[Risk]
}

// NewMemoryRiskStore creates an empty in-memory store.
func NewMemoryRiskStore() *MemoryRiskStore {
	return &MemoryRiskStore{t: newMemTable[Risk]()}
}

func (m *MemoryRiskStore) List(_ context.Context, tenantID string) ([]Risk, error) {
	return m.t.list(tenantID), nil
}

func (m *MemoryRiskStore) Get(_ context.Context, tenantID, id string) (*Risk, error) {
	r, ok := m.t.get(tenantID, id)
	if !ok {
		return nil, ErrNotFound
	}
	return &r, nil
}

func (m *MemoryRiskStore) Create(_ context.Context, r *Risk) error {
	r.ID = newUUID()
	// Assign the code and insert under one lock so concurrent creates
	// never receive the same code.
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	rows := m.t.rows[r.TenantID]
	codes := make([]string, 0, len(rows))
	for _, existing := range rows {
		if r.Code != "" && existing.Code == r.Code {
			return ErrConflict
		}
		codes = append(codes, existing.Code)
	}
	if r.Code == "" {
		r.Code = nextRiskCode(codes)
	}
	if rows == nil {
		rows = make(map[string]Risk)
		m.t.rows[r.TenantID] = rows
	}
	rows[r.ID] = *r
	return nil
}

func (m *MemoryRiskStore) Update(_ context.Context, r *Risk) error {
	return m.t.replace(r.TenantID, r.ID, *r, func(_ string, other Risk) bool {
		return other.Code == r.Code
	})
}

func (m *MemoryRiskStore) Delete(_ context.Context, tenantID, id string) error {
	return m.t.remove(tenantID, id)
}
