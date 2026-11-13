package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
)

// ── Evidence ──
//
// Evidence is a file that shows a control operating: a screenshot, a
// report, an export, signed minutes. The file is immutable and identified
// by its SHA-256; what can change is its description, its dates and the
// controls it supports.

// Evidence is one evidence record.
type Evidence struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenant_id,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	CollectedOn string   `json:"collected_on"`          // YYYY-MM-DD: when it was obtained
	ValidUntil  string   `json:"valid_until,omitempty"` // YYYY-MM-DD: renew after this date
	Expired     bool     `json:"expired"`
	Controls    []string `json:"controls"`
	File

	UploadedBy      string    `json:"-"`
	UploadedByEmail string    `json:"uploaded_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (e *Evidence) refresh() {
	e.Expired = e.ValidUntil != "" && e.ValidUntil < today()
	if e.Controls == nil {
		e.Controls = []string{}
	}
}

// EvidenceStore persists evidence records, scoped to one tenant.
type EvidenceStore interface {
	List(ctx context.Context, tenantID string) ([]Evidence, error)
	Get(ctx context.Context, tenantID, id string) (*Evidence, error)
	Create(ctx context.Context, e *Evidence) error // assigns e.ID
	// Update changes the description, dates and linked controls. The file
	// cannot be changed.
	Update(ctx context.Context, e *Evidence) error
	// Delete removes the record and returns the storage key of its file.
	Delete(ctx context.Context, tenantID, id string) (string, error)
}

// EvidenceHandler serves evidence. Everyone in the organization can read
// and download; owners, admins and editors upload and edit; only owners
// and admins delete.
type EvidenceHandler struct {
	store  EvidenceStore
	files  *Files
	logger *slog.Logger
}

// NewEvidenceHandler creates the handler.
func NewEvidenceHandler(store EvidenceStore, files *Files, logger *slog.Logger) *EvidenceHandler {
	return &EvidenceHandler{store: store, files: files, logger: logger}
}

// Register wires the evidence routes into the given mux.
func (h *EvidenceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/evidence", h.handleList)
	mux.HandleFunc("POST /v1/evidence", h.handleCreate)
	mux.HandleFunc("GET /v1/evidence/{id}", h.handleGet)
	mux.HandleFunc("PUT /v1/evidence/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /v1/evidence/{id}", h.handleDelete)
	mux.HandleFunc("GET /v1/evidence/{id}/download", h.handleDownload)
	mux.HandleFunc("GET /v1/evidence/{id}/integrity", h.handleIntegrity)
}

func (h *EvidenceHandler) fail(w http.ResponseWriter, err error, what string) {
	if ve, ok := err.(*validationError); ok {
		writeError(w, http.StatusUnprocessableEntity, ve.msg)
		return
	}
	if writeStoreError(w, err, "evidence") {
		h.logger.Error("evidence: "+what+" failed", "error", err)
	}
}

func (h *EvidenceHandler) handleList(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	items, err := h.store.List(r.Context(), user.TenantID)
	if err != nil {
		h.fail(w, err, "list")
		return
	}
	if control := r.URL.Query().Get("control"); control != "" {
		items = filterByControl(items, control, func(e Evidence) []string { return e.Controls })
	}
	// Newest evidence first.
	sort.Slice(items, func(i, j int) bool {
		if items[i].CollectedOn != items[j].CollectedOn {
			return items[i].CollectedOn > items[j].CollectedOn
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
}

func (h *EvidenceHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	e, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *EvidenceHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanWrite() {
		writeError(w, http.StatusForbidden, "your role does not allow changes")
		return
	}
	fields, up, err := h.files.receive(w, r, true)
	if err != nil {
		h.files.writeUploadError(w, err)
		return
	}
	defer up.discard()

	now := time.Now().UTC()
	e := Evidence{
		TenantID:        user.TenantID,
		Title:           fields.Get("title"),
		Description:     fields.Get("description"),
		CollectedOn:     fields.Get("collected_on"),
		ValidUntil:      fields.Get("valid_until"),
		Controls:        splitList(fields["controls"]),
		UploadedBy:      user.ID,
		UploadedByEmail: user.Email,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if strings.TrimSpace(e.Title) == "" {
		e.Title = up.Name // a sensible default: the file's own name
	}
	if err := normalizeEvidence(&e); err != nil {
		h.fail(w, err, "create")
		return
	}

	if err := h.files.save(r.Context(), user.TenantID+"/evidence/"+newUUID(), up); err != nil {
		h.files.writeUploadError(w, err)
		return
	}
	e.File = up.File
	if err := h.store.Create(r.Context(), &e); err != nil {
		h.files.remove(r.Context(), up.StorageKey)
		h.fail(w, err, "create")
		return
	}
	e.refresh()
	writeJSON(w, http.StatusCreated, e)
}

func (h *EvidenceHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanWrite() {
		writeError(w, http.StatusForbidden, "your role does not allow changes")
		return
	}
	existing, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	var input struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		CollectedOn string   `json:"collected_on"`
		ValidUntil  string   `json:"valid_until"`
		Controls    []string `json:"controls"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	existing.Title, existing.Description = input.Title, input.Description
	existing.CollectedOn, existing.ValidUntil, existing.Controls = input.CollectedOn, input.ValidUntil, input.Controls
	if err := normalizeEvidence(existing); err != nil {
		h.fail(w, err, "update")
		return
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := h.store.Update(r.Context(), existing); err != nil {
		h.fail(w, err, "update")
		return
	}
	existing.refresh()
	writeJSON(w, http.StatusOK, existing)
}

func (h *EvidenceHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanManageMembers() {
		writeError(w, http.StatusForbidden, "only owners and administrators can delete evidence")
		return
	}
	key, err := h.store.Delete(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "delete")
		return
	}
	h.files.remove(r.Context(), key)
	w.WriteHeader(http.StatusNoContent)
}

func (h *EvidenceHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	e, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	h.files.serve(w, r, e.File)
}

func (h *EvidenceHandler) handleIntegrity(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	e, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	h.files.writeIntegrity(w, r, e.File)
}

func normalizeEvidence(e *Evidence) error {
	e.Title = strings.TrimSpace(e.Title)
	e.Description = strings.TrimSpace(e.Description)
	if e.Title == "" {
		return invalid("title is required")
	}
	if len(e.Title) > 300 {
		return invalid("title must be at most 300 characters")
	}
	if len(e.Description) > 4000 {
		return invalid("description must be at most 4000 characters")
	}
	var err error
	if e.CollectedOn, err = parseDate("collected_on", e.CollectedOn); err != nil {
		return err
	}
	if e.CollectedOn == "" {
		e.CollectedOn = today()
	}
	if e.CollectedOn > today() {
		return invalid("collected_on cannot be in the future")
	}
	if e.ValidUntil, err = parseDate("valid_until", e.ValidUntil); err != nil {
		return err
	}
	if e.ValidUntil != "" && e.ValidUntil < e.CollectedOn {
		return invalid("valid_until cannot be before collected_on")
	}
	e.Controls, err = normalizeControls(e.Controls)
	return err
}

// ── In-Memory Store (development) ──

// MemoryEvidenceStore is a thread-safe in-memory evidence store.
type MemoryEvidenceStore struct {
	t *memTable[Evidence]
}

// NewMemoryEvidenceStore creates an empty in-memory store.
func NewMemoryEvidenceStore() *MemoryEvidenceStore {
	return &MemoryEvidenceStore{t: newMemTable[Evidence]()}
}

func cloneEvidence(e Evidence) Evidence {
	e.Controls = append([]string(nil), e.Controls...)
	e.refresh()
	return e
}

func (m *MemoryEvidenceStore) List(_ context.Context, tenantID string) ([]Evidence, error) {
	items := m.t.list(tenantID)
	for i := range items {
		items[i] = cloneEvidence(items[i])
	}
	return items, nil
}

func (m *MemoryEvidenceStore) Get(_ context.Context, tenantID, id string) (*Evidence, error) {
	e, ok := m.t.get(tenantID, id)
	if !ok {
		return nil, ErrNotFound
	}
	e = cloneEvidence(e)
	return &e, nil
}

func (m *MemoryEvidenceStore) Create(_ context.Context, e *Evidence) error {
	e.ID = newUUID()
	return m.t.insert(e.TenantID, e.ID, cloneEvidence(*e), nil)
}

func (m *MemoryEvidenceStore) Update(_ context.Context, e *Evidence) error {
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	stored, ok := m.t.rows[e.TenantID][e.ID]
	if !ok {
		return ErrNotFound
	}
	// Only the descriptive fields change; the file stays as uploaded.
	stored.Title, stored.Description = e.Title, e.Description
	stored.CollectedOn, stored.ValidUntil, stored.UpdatedAt = e.CollectedOn, e.ValidUntil, e.UpdatedAt
	stored.Controls = append([]string(nil), e.Controls...)
	m.t.rows[e.TenantID][e.ID] = stored
	return nil
}

func (m *MemoryEvidenceStore) Delete(_ context.Context, tenantID, id string) (string, error) {
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	stored, ok := m.t.rows[tenantID][id]
	if !ok {
		return "", ErrNotFound
	}
	delete(m.t.rows[tenantID], id)
	return stored.StorageKey, nil
}
