package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/catalog"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
)

// ── Controlled documents (ISO/IEC 27001 clause 7.5) ──
//
// A document (policy, procedure, record…) has numbered versions. Each
// version is one uploaded file and is never overwritten. A version moves
//
//	draft ──submit──▶ in_review ──approve──▶ approved
//	                      └──────reject───▶ rejected
//
// and only the newest version can move. Approving a version supersedes the
// one previously in force; uploading a new version supersedes older ones
// that were still waiting. A document can be declared obsolete, after
// which it no longer changes.

// Version statuses.
const (
	VersionDraft      = "draft"
	VersionInReview   = "in_review"
	VersionApproved   = "approved"
	VersionRejected   = "rejected"
	VersionSuperseded = "superseded"
)

// Document statuses.
const (
	DocumentDraft    = "draft"    // no version has been approved or is under review
	DocumentReview   = "review"   // the newest version awaits approval, none is in force
	DocumentApproved = "approved" // a version is in force
	DocumentObsolete = "obsolete" // withdrawn
)

var documentTypes = []string{"policy", "procedure", "record", "template", "report", "other"}

// DocumentVersion is one immutable file of a document.
type DocumentVersion struct {
	Number        int    `json:"number"`
	Status        string `json:"status"`
	ChangeSummary string `json:"change_summary"`
	File

	UploadedBy      string     `json:"-"` // member id
	UploadedByEmail string     `json:"uploaded_by"`
	UploadedAt      time.Time  `json:"uploaded_at"`
	ReviewedBy      string     `json:"-"`
	ReviewedByEmail string     `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	ReviewNote      string     `json:"review_note,omitempty"`
}

// Document is a controlled document with its versions, newest first.
type Document struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id,omitempty"`
	Title          string            `json:"title"`
	Type           string            `json:"doc_type"`
	Status         string            `json:"status"`
	Owner          string            `json:"owner"`
	NextReview     string            `json:"next_review,omitempty"` // YYYY-MM-DD
	ReviewOverdue  bool              `json:"review_overdue"`
	Controls       []string          `json:"controls"`
	Versions       []DocumentVersion `json:"versions"`
	ApprovedNumber int               `json:"approved_version,omitempty"` // version in force
	CreatedBy      string            `json:"-"`
	CreatedByEmail string            `json:"created_by"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// errTransition is returned when an action does not apply to the current state.
type errTransition struct{ msg string }

func (e *errTransition) Error() string { return e.msg }

func noTransition(format string, args ...any) error {
	return &errTransition{fmt.Sprintf(format, args...)}
}

func (d *Document) latest() *DocumentVersion {
	if len(d.Versions) == 0 {
		return nil
	}
	return &d.Versions[0]
}

func (d *Document) version(number int) *DocumentVersion {
	for i := range d.Versions {
		if d.Versions[i].Number == number {
			return &d.Versions[i]
		}
	}
	return nil
}

// refresh recomputes the fields derived from the versions and dates.
func (d *Document) refresh() {
	sort.Slice(d.Versions, func(i, j int) bool { return d.Versions[i].Number > d.Versions[j].Number })
	d.ApprovedNumber = 0
	for _, v := range d.Versions {
		if v.Status == VersionApproved {
			d.ApprovedNumber = v.Number
		}
	}
	if d.Status != DocumentObsolete {
		switch {
		case d.ApprovedNumber != 0:
			d.Status = DocumentApproved
		case d.latest() != nil && d.latest().Status == VersionInReview:
			d.Status = DocumentReview
		default:
			d.Status = DocumentDraft
		}
	}
	d.ReviewOverdue = d.Status == DocumentApproved && d.NextReview != "" && d.NextReview < today()
	if d.Controls == nil {
		d.Controls = []string{}
	}
}

// addVersion appends a new draft version. Older versions that were still
// waiting (draft or in review) are superseded: only the newest can advance.
func (d *Document) addVersion(v DocumentVersion) error {
	if d.Status == DocumentObsolete {
		return noTransition("the document is obsolete")
	}
	next := 1
	for i := range d.Versions {
		if d.Versions[i].Number >= next {
			next = d.Versions[i].Number + 1
		}
		if s := d.Versions[i].Status; s == VersionDraft || s == VersionInReview {
			d.Versions[i].Status = VersionSuperseded
		}
	}
	v.Number = next
	v.Status = VersionDraft
	d.Versions = append(d.Versions, v)
	d.refresh()
	return nil
}

// submit sends the newest version for approval.
func (d *Document) submit(number int) error {
	v, err := d.movable(number)
	if err != nil {
		return err
	}
	if v.Status != VersionDraft {
		return noTransition("only a draft can be sent for review (version %d is %s)", number, v.Status)
	}
	v.Status = VersionInReview
	d.refresh()
	return nil
}

// review approves or rejects the version under review.
func (d *Document) review(number int, approve bool, reviewer *auth.User, note string, now time.Time) error {
	v, err := d.movable(number)
	if err != nil {
		return err
	}
	if v.Status != VersionInReview {
		return noTransition("version %d is not under review (it is %s)", number, v.Status)
	}
	if approve {
		for i := range d.Versions {
			if d.Versions[i].Status == VersionApproved {
				d.Versions[i].Status = VersionSuperseded
			}
		}
		v.Status = VersionApproved
	} else {
		v.Status = VersionRejected
	}
	v.ReviewedBy, v.ReviewedByEmail = reviewer.ID, reviewer.Email
	v.ReviewedAt = &now
	v.ReviewNote = note
	d.refresh()
	return nil
}

// markObsolete withdraws the document.
func (d *Document) markObsolete() error {
	if d.Status == DocumentObsolete {
		return noTransition("the document is already obsolete")
	}
	d.Status = DocumentObsolete
	d.refresh()
	return nil
}

func (d *Document) movable(number int) (*DocumentVersion, error) {
	if d.Status == DocumentObsolete {
		return nil, noTransition("the document is obsolete")
	}
	v := d.version(number)
	if v == nil {
		return nil, ErrNotFound
	}
	if v.Number != d.latest().Number {
		return nil, noTransition("only the newest version (%d) can change state", d.latest().Number)
	}
	return v, nil
}

// everApproved reports whether any version was ever in force. Such
// documents are part of the organization's history and cannot be deleted.
func (d *Document) everApproved() bool {
	for _, v := range d.Versions {
		if v.ReviewedAt != nil && (v.Status == VersionApproved || v.Status == VersionSuperseded) {
			return true
		}
	}
	return false
}

// AuditNote describes a change for the audit trail.
type AuditNote struct {
	Action string
	Detail map[string]any
}

// DocumentStore persists controlled documents. Every method is scoped to
// one tenant. Documents are returned with all their versions, refreshed.
type DocumentStore interface {
	List(ctx context.Context, tenantID string) ([]Document, error)
	Get(ctx context.Context, tenantID, id string) (*Document, error)
	// Create stores a new document with its first version; assigns doc.ID.
	Create(ctx context.Context, doc *Document) error
	// UpdateMeta changes title, type, owner, review date and linked controls.
	UpdateMeta(ctx context.Context, doc *Document) error
	// Mutate loads the document, applies change and saves the resulting
	// status and versions atomically. change returns the audit note for
	// what it did; if it returns an error nothing is saved.
	Mutate(ctx context.Context, tenantID, id string, change func(d *Document) (AuditNote, error)) (*Document, error)
	// Delete removes the document and returns the storage keys of its files.
	Delete(ctx context.Context, tenantID, id string) ([]string, error)
}

// ── Handler ──

// DocumentHandler serves controlled documents.
//
// Everyone in the organization can read and download. Owners, admins and
// editors create documents, upload versions and send them for review.
// Only owners and admins approve, reject, withdraw and delete.
type DocumentHandler struct {
	store  DocumentStore
	files  *Files
	logger *slog.Logger
}

// NewDocumentHandler creates the handler.
func NewDocumentHandler(store DocumentStore, files *Files, logger *slog.Logger) *DocumentHandler {
	return &DocumentHandler{store: store, files: files, logger: logger}
}

// Register wires the document routes into the given mux.
func (h *DocumentHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/documents", h.handleList)
	mux.HandleFunc("POST /v1/documents", h.handleCreate)
	mux.HandleFunc("GET /v1/documents/{id}", h.handleGet)
	mux.HandleFunc("PUT /v1/documents/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /v1/documents/{id}", h.handleDelete)
	mux.HandleFunc("POST /v1/documents/{id}/obsolete", h.handleObsolete)
	mux.HandleFunc("POST /v1/documents/{id}/versions", h.handleAddVersion)
	mux.HandleFunc("POST /v1/documents/{id}/versions/{number}/submit", h.handleSubmit)
	mux.HandleFunc("POST /v1/documents/{id}/versions/{number}/approve", h.handleApprove)
	mux.HandleFunc("POST /v1/documents/{id}/versions/{number}/reject", h.handleReject)
	mux.HandleFunc("GET /v1/documents/{id}/versions/{number}/download", h.handleDownload)
	mux.HandleFunc("GET /v1/documents/{id}/versions/{number}/integrity", h.handleIntegrity)
}

func (h *DocumentHandler) fail(w http.ResponseWriter, err error, what string) {
	var te *errTransition
	var ve *validationError
	switch {
	case errors.As(err, &te):
		writeError(w, http.StatusConflict, te.msg)
	case errors.As(err, &ve):
		writeError(w, http.StatusUnprocessableEntity, ve.msg)
	default:
		if writeStoreError(w, err, "document") {
			h.logger.Error("document: "+what+" failed", "error", err)
		}
	}
}

func (h *DocumentHandler) handleList(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	docs, err := h.store.List(r.Context(), user.TenantID)
	if err != nil {
		h.fail(w, err, "list")
		return
	}
	if control := r.URL.Query().Get("control"); control != "" {
		docs = filterByControl(docs, control, func(d Document) []string { return d.Controls })
	}
	sort.Slice(docs, func(i, j int) bool { return strings.ToLower(docs[i].Title) < strings.ToLower(docs[j].Title) })
	writeJSON(w, http.StatusOK, map[string]any{"data": docs, "count": len(docs)})
}

func (h *DocumentHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	doc, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *DocumentHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
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
	doc := Document{
		TenantID:       user.TenantID,
		Title:          fields.Get("title"),
		Type:           fields.Get("doc_type"),
		Owner:          fields.Get("owner"),
		NextReview:     fields.Get("next_review"),
		Controls:       splitList(fields["controls"]),
		CreatedBy:      user.ID,
		CreatedByEmail: user.Email,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := normalizeDocument(&doc); err != nil {
		h.fail(w, err, "create")
		return
	}

	if err := h.files.save(r.Context(), user.TenantID+"/documents/"+newUUID(), up); err != nil {
		h.files.writeUploadError(w, err)
		return
	}
	doc.addVersion(newVersion(up.File, fields.Get("change_summary"), user, now)) //nolint:errcheck // a new document is never obsolete

	if err := h.store.Create(r.Context(), &doc); err != nil {
		h.files.remove(r.Context(), up.StorageKey)
		h.fail(w, err, "create")
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *DocumentHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
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
	if existing.Status == DocumentObsolete {
		writeError(w, http.StatusConflict, "the document is obsolete")
		return
	}

	var input struct {
		Title      string   `json:"title"`
		Type       string   `json:"doc_type"`
		Owner      string   `json:"owner"`
		NextReview string   `json:"next_review"`
		Controls   []string `json:"controls"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	existing.Title, existing.Type, existing.Owner = input.Title, input.Type, input.Owner
	existing.NextReview, existing.Controls = input.NextReview, input.Controls
	if err := normalizeDocument(existing); err != nil {
		h.fail(w, err, "update")
		return
	}
	existing.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateMeta(r.Context(), existing); err != nil {
		h.fail(w, err, "update")
		return
	}
	existing.refresh()
	writeJSON(w, http.StatusOK, existing)
}

func (h *DocumentHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanManageMembers() {
		writeError(w, http.StatusForbidden, "only owners and administrators can delete documents")
		return
	}
	existing, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	if existing.everApproved() {
		writeError(w, http.StatusConflict,
			"a document that was approved cannot be deleted; mark it obsolete to keep its history")
		return
	}
	keys, err := h.store.Delete(r.Context(), user.TenantID, existing.ID)
	if err != nil {
		h.fail(w, err, "delete")
		return
	}
	h.files.remove(r.Context(), keys...)
	w.WriteHeader(http.StatusNoContent)
}

func (h *DocumentHandler) handleAddVersion(w http.ResponseWriter, r *http.Request) {
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

	// Check before storing the file, so refused uploads leave nothing behind.
	existing, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return
	}
	if existing.Status == DocumentObsolete {
		writeError(w, http.StatusConflict, "the document is obsolete")
		return
	}
	summary := strings.TrimSpace(fields.Get("change_summary"))
	if len(summary) > 2000 {
		writeError(w, http.StatusUnprocessableEntity, "change_summary must be at most 2000 characters")
		return
	}

	if err := h.files.save(r.Context(), user.TenantID+"/documents/"+newUUID(), up); err != nil {
		h.files.writeUploadError(w, err)
		return
	}
	doc, err := h.store.Mutate(r.Context(), user.TenantID, existing.ID, func(d *Document) (AuditNote, error) {
		if err := d.addVersion(newVersion(up.File, summary, user, time.Now().UTC())); err != nil {
			return AuditNote{}, err
		}
		v := d.latest()
		return AuditNote{"document.version_add", map[string]any{
			"title": d.Title, "version": v.Number, "file_name": v.Name, "sha256": v.SHA256}}, nil
	})
	if err != nil {
		h.files.remove(r.Context(), up.StorageKey)
		h.fail(w, err, "add version")
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *DocumentHandler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanWrite() {
		writeError(w, http.StatusForbidden, "your role does not allow changes")
		return
	}
	h.transition(w, r, user, func(d *Document, number int) (AuditNote, error) {
		return AuditNote{"document.submit", map[string]any{"title": d.Title, "version": number}}, d.submit(number)
	})
}

func (h *DocumentHandler) handleApprove(w http.ResponseWriter, r *http.Request) {
	h.handleReview(w, r, true)
}

func (h *DocumentHandler) handleReject(w http.ResponseWriter, r *http.Request) {
	h.handleReview(w, r, false)
}

func (h *DocumentHandler) handleReview(w http.ResponseWriter, r *http.Request, approve bool) {
	user := auth.FromContext(r.Context())
	if !user.CanManageMembers() {
		writeError(w, http.StatusForbidden, "only owners and administrators can approve or reject documents")
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	note := strings.TrimSpace(input.Note)
	if len(note) > 2000 {
		writeError(w, http.StatusUnprocessableEntity, "note must be at most 2000 characters")
		return
	}
	if !approve && note == "" {
		writeError(w, http.StatusUnprocessableEntity, "explain why the version is rejected")
		return
	}
	action := "document.reject"
	if approve {
		action = "document.approve"
	}
	h.transition(w, r, user, func(d *Document, number int) (AuditNote, error) {
		err := d.review(number, approve, user, note, time.Now().UTC())
		return AuditNote{action, map[string]any{"title": d.Title, "version": number, "note": note}}, err
	})
}

func (h *DocumentHandler) handleObsolete(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanManageMembers() {
		writeError(w, http.StatusForbidden, "only owners and administrators can withdraw documents")
		return
	}
	doc, err := h.store.Mutate(r.Context(), user.TenantID, r.PathValue("id"), func(d *Document) (AuditNote, error) {
		return AuditNote{"document.obsolete", map[string]any{"title": d.Title}}, d.markObsolete()
	})
	if err != nil {
		h.fail(w, err, "obsolete")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// transition applies a state change to the version named in the path.
func (h *DocumentHandler) transition(w http.ResponseWriter, r *http.Request, user *auth.User,
	change func(d *Document, number int) (AuditNote, error)) {
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || number < 1 {
		writeError(w, http.StatusNotFound, "version not found")
		return
	}
	doc, err := h.store.Mutate(r.Context(), user.TenantID, r.PathValue("id"), func(d *Document) (AuditNote, error) {
		return change(d, number)
	})
	if err != nil {
		h.fail(w, err, "transition")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *DocumentHandler) versionFile(w http.ResponseWriter, r *http.Request) (File, bool) {
	user := auth.FromContext(r.Context())
	doc, err := h.store.Get(r.Context(), user.TenantID, r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "get")
		return File{}, false
	}
	number, _ := strconv.Atoi(r.PathValue("number"))
	v := doc.version(number)
	if v == nil {
		writeError(w, http.StatusNotFound, "version not found")
		return File{}, false
	}
	return v.File, true
}

func (h *DocumentHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	if file, ok := h.versionFile(w, r); ok {
		h.files.serve(w, r, file)
	}
}

func (h *DocumentHandler) handleIntegrity(w http.ResponseWriter, r *http.Request) {
	if file, ok := h.versionFile(w, r); ok {
		h.files.writeIntegrity(w, r, file)
	}
}

// ── Helpers ──

func newVersion(file File, summary string, user *auth.User, now time.Time) DocumentVersion {
	return DocumentVersion{
		ChangeSummary:   strings.TrimSpace(summary),
		File:            file,
		UploadedBy:      user.ID,
		UploadedByEmail: user.Email,
		UploadedAt:      now,
	}
}

func normalizeDocument(d *Document) error {
	d.Title = strings.TrimSpace(d.Title)
	d.Owner = strings.TrimSpace(d.Owner)
	if d.Title == "" {
		return invalid("title is required")
	}
	if len(d.Title) > 300 {
		return invalid("title must be at most 300 characters")
	}
	if len(d.Owner) > 200 {
		return invalid("owner must be at most 200 characters")
	}
	if !oneOf(d.Type, documentTypes) {
		return invalid("doc_type must be one of: %s", strings.Join(documentTypes, ", "))
	}
	var err error
	if d.NextReview, err = parseDate("next_review", d.NextReview); err != nil {
		return err
	}
	d.Controls, err = normalizeControls(d.Controls)
	return err
}

// normalizeControls validates control ids against the catalog, removes
// duplicates and sorts them in catalog order.
func normalizeControls(ids []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		if _, ok := catalog.Lookup(id); !ok {
			return nil, invalid("control %s is not in the ISO/IEC 27001:2022 Annex A catalog", id)
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return controlLess(out[i], out[j]) })
	return out, nil
}

// splitList accepts repeated form fields and comma-separated values.
func splitList(values []string) []string {
	var out []string
	for _, v := range values {
		out = append(out, strings.Split(v, ",")...)
	}
	return out
}

func filterByControl[T any](items []T, control string, controls func(T) []string) []T {
	out := items[:0:0]
	for _, item := range items {
		for _, c := range controls(item) {
			if c == control {
				out = append(out, item)
				break
			}
		}
	}
	return out
}

// ── In-Memory Store (development) ──

// MemoryDocumentStore is a thread-safe in-memory document store.
type MemoryDocumentStore struct {
	t *memTable[Document]
}

// NewMemoryDocumentStore creates an empty in-memory store.
func NewMemoryDocumentStore() *MemoryDocumentStore {
	return &MemoryDocumentStore{t: newMemTable[Document]()}
}

// cloneDocument copies the slices so callers cannot alter stored data.
func cloneDocument(d Document) Document {
	d.Versions = append([]DocumentVersion(nil), d.Versions...)
	d.Controls = append([]string(nil), d.Controls...)
	d.refresh()
	return d
}

func (m *MemoryDocumentStore) List(_ context.Context, tenantID string) ([]Document, error) {
	docs := m.t.list(tenantID)
	for i := range docs {
		docs[i] = cloneDocument(docs[i])
	}
	return docs, nil
}

func (m *MemoryDocumentStore) Get(_ context.Context, tenantID, id string) (*Document, error) {
	d, ok := m.t.get(tenantID, id)
	if !ok {
		return nil, ErrNotFound
	}
	d = cloneDocument(d)
	return &d, nil
}

func (m *MemoryDocumentStore) Create(_ context.Context, doc *Document) error {
	doc.ID = newUUID()
	doc.refresh()
	return m.t.insert(doc.TenantID, doc.ID, cloneDocument(*doc), nil)
}

func (m *MemoryDocumentStore) UpdateMeta(_ context.Context, doc *Document) error {
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	stored, ok := m.t.rows[doc.TenantID][doc.ID]
	if !ok {
		return ErrNotFound
	}
	stored.Title, stored.Type, stored.Owner = doc.Title, doc.Type, doc.Owner
	stored.NextReview, stored.UpdatedAt = doc.NextReview, doc.UpdatedAt
	stored.Controls = append([]string(nil), doc.Controls...)
	m.t.rows[doc.TenantID][doc.ID] = stored
	return nil
}

func (m *MemoryDocumentStore) Mutate(_ context.Context, tenantID, id string, change func(d *Document) (AuditNote, error)) (*Document, error) {
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	stored, ok := m.t.rows[tenantID][id]
	if !ok {
		return nil, ErrNotFound
	}
	working := cloneDocument(stored)
	if _, err := change(&working); err != nil {
		return nil, err
	}
	working.UpdatedAt = time.Now().UTC()
	working.refresh()
	m.t.rows[tenantID][id] = cloneDocument(working)
	return &working, nil
}

func (m *MemoryDocumentStore) Delete(_ context.Context, tenantID, id string) ([]string, error) {
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	stored, ok := m.t.rows[tenantID][id]
	if !ok {
		return nil, ErrNotFound
	}
	keys := make([]string, 0, len(stored.Versions))
	for _, v := range stored.Versions {
		keys = append(keys, v.StorageKey)
	}
	delete(m.t.rows[tenantID], id)
	return keys, nil
}
