package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// ── Asset domain model ──

// Asset is an entry of the information asset inventory (Annex A 5.9).
// Each asset is valued 1–5 for confidentiality, integrity and availability;
// its criticality is the highest of the three.
type Asset struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id,omitempty"`
	Name            string    `json:"name"`
	Type            string    `json:"asset_type"`
	Description     string    `json:"description"`
	Owner           string    `json:"owner"`
	Location        string    `json:"location"`
	Classification  string    `json:"classification"`
	Confidentiality int       `json:"confidentiality"`
	Integrity       int       `json:"integrity"`
	Availability    int       `json:"availability"`
	Criticality     int       `json:"criticality"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

var (
	assetTypes           = []string{"information", "software", "hardware", "service", "people", "facility"}
	assetClassifications = []string{"public", "internal", "confidential", "restricted"}
	assetStatuses        = []string{"active", "retired"}
)

// AssetStore is the persistence interface for the asset inventory.
// Asset names are unique per tenant (ErrConflict otherwise).
type AssetStore interface {
	List(ctx context.Context, tenantID string) ([]Asset, error)
	Get(ctx context.Context, tenantID, id string) (*Asset, error)
	Create(ctx context.Context, asset *Asset) error // assigns asset.ID
	Update(ctx context.Context, asset *Asset) error
	Delete(ctx context.Context, tenantID, id string) error
}

// AssetHandler handles asset inventory requests.
type AssetHandler struct {
	store  AssetStore
	logger *slog.Logger
}

// NewAssetHandler creates a handler with the given store and logger.
func NewAssetHandler(store AssetStore, logger *slog.Logger) *AssetHandler {
	return &AssetHandler{store: store, logger: logger}
}

// Register wires the asset routes into the given mux.
func (h *AssetHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/assets", h.handleList)
	mux.HandleFunc("POST /v1/assets", h.handleCreate)
	mux.HandleFunc("GET /v1/assets/{id}", h.handleGet)
	mux.HandleFunc("PUT /v1/assets/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /v1/assets/{id}", h.handleDelete)
}

func (h *AssetHandler) handleList(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	assets, err := h.store.List(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("asset list failed", "error", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}
	// Most critical first, then by name.
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].Criticality != assets[j].Criticality {
			return assets[i].Criticality > assets[j].Criticality
		}
		return strings.ToLower(assets[i].Name) < strings.ToLower(assets[j].Name)
	})

	writeJSON(w, http.StatusOK, map[string]any{"data": assets, "count": len(assets)})
}

func (h *AssetHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	asset, err := h.store.Get(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		if writeStoreError(w, err, "asset") {
			h.logger.Error("asset get failed", "error", err, "tenant", tenantID)
		}
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

func (h *AssetHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	var input Asset
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := normalizeAsset(&input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	input.ID = ""
	input.TenantID = tenantID
	now := time.Now().UTC()
	input.CreatedAt = now
	input.UpdatedAt = now

	if err := h.store.Create(r.Context(), &input); err != nil {
		if writeStoreError(w, err, "asset with this name") {
			h.logger.Error("asset create failed", "error", err, "tenant", tenantID)
		}
		return
	}
	writeJSON(w, http.StatusCreated, input)
}

func (h *AssetHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	existing, err := h.store.Get(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		if writeStoreError(w, err, "asset") {
			h.logger.Error("asset get failed", "error", err, "tenant", tenantID)
		}
		return
	}

	var input Asset
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := normalizeAsset(&input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	input.ID = existing.ID
	input.TenantID = existing.TenantID
	input.CreatedAt = existing.CreatedAt
	input.UpdatedAt = time.Now().UTC()

	if err := h.store.Update(r.Context(), &input); err != nil {
		if writeStoreError(w, err, "asset with this name") {
			h.logger.Error("asset update failed", "error", err, "tenant", tenantID)
		}
		return
	}
	writeJSON(w, http.StatusOK, input)
}

func (h *AssetHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.FromContext(r.Context())

	if err := h.store.Delete(r.Context(), tenantID, r.PathValue("id")); err != nil {
		if writeStoreError(w, err, "asset") {
			h.logger.Error("asset delete failed", "error", err, "tenant", tenantID)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// normalizeAsset applies defaults, validates the input and computes the
// derived criticality.
func normalizeAsset(a *Asset) error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return invalid("name is required")
	}
	if len(a.Name) > 200 {
		return invalid("name must be at most 200 characters")
	}
	if !oneOf(a.Type, assetTypes) {
		return invalid("asset_type must be one of: %s", strings.Join(assetTypes, ", "))
	}
	if a.Classification == "" {
		a.Classification = "internal"
	}
	if !oneOf(a.Classification, assetClassifications) {
		return invalid("classification must be one of: %s", strings.Join(assetClassifications, ", "))
	}
	if a.Status == "" {
		a.Status = "active"
	}
	if !oneOf(a.Status, assetStatuses) {
		return invalid("status must be one of: %s", strings.Join(assetStatuses, ", "))
	}
	for _, v := range []struct {
		name  string
		value *int
	}{
		{"confidentiality", &a.Confidentiality},
		{"integrity", &a.Integrity},
		{"availability", &a.Availability},
	} {
		if *v.value == 0 {
			*v.value = 1
		}
		if *v.value < 1 || *v.value > 5 {
			return invalid("%s must be 1-5", v.name)
		}
	}
	a.Criticality = max(a.Confidentiality, a.Integrity, a.Availability)
	return nil
}

func oneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// ── In-Memory Store (development) ──

// MemoryAssetStore is a thread-safe in-memory asset store for development.
type MemoryAssetStore struct {
	t *memTable[Asset]
}

// NewMemoryAssetStore creates an empty in-memory store.
func NewMemoryAssetStore() *MemoryAssetStore {
	return &MemoryAssetStore{t: newMemTable[Asset]()}
}

func (m *MemoryAssetStore) List(_ context.Context, tenantID string) ([]Asset, error) {
	return m.t.list(tenantID), nil
}

func (m *MemoryAssetStore) Get(_ context.Context, tenantID, id string) (*Asset, error) {
	a, ok := m.t.get(tenantID, id)
	if !ok {
		return nil, ErrNotFound
	}
	return &a, nil
}

func (m *MemoryAssetStore) Create(_ context.Context, a *Asset) error {
	a.ID = newUUID()
	return m.t.insert(a.TenantID, a.ID, *a, func(existing Asset) bool {
		return existing.Name == a.Name
	})
}

func (m *MemoryAssetStore) Update(_ context.Context, a *Asset) error {
	return m.t.replace(a.TenantID, a.ID, *a, func(_ string, other Asset) bool {
		return other.Name == a.Name
	})
}

func (m *MemoryAssetStore) Delete(_ context.Context, tenantID, id string) error {
	return m.t.remove(tenantID, id)
}
