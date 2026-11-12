package directory

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
)

// Handler serves member management for the current organization.
//
// Who may do what:
//   - owner, admin and auditor can list members;
//   - owner and admin can invite, change roles and remove members;
//   - only an owner can grant the owner role or change another owner;
//   - nobody can change or remove themselves (ask another owner/admin),
//     which also prevents locking oneself out by accident.
type Handler struct {
	store  Store
	logger *slog.Logger
}

// NewHandler creates a member-management handler.
func NewHandler(store Store, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

// Register wires the routes into the tenant-scoped API mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/users", h.handleList)
	mux.HandleFunc("POST /v1/users", h.handleInvite)
	mux.HandleFunc("PUT /v1/users/{id}", h.handleSetRole)
	mux.HandleFunc("DELETE /v1/users/{id}", h.handleRemove)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanManageMembers() && !user.CanViewAudit() {
		writeError(w, http.StatusForbidden, "your role cannot view members")
		return
	}
	members, err := h.store.ListMembers(r.Context(), user.TenantID)
	if err != nil {
		h.fail(w, err, "list members")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": members, "count": len(members)})
}

func (h *Handler) handleInvite(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanManageMembers() {
		writeError(w, http.StatusForbidden, "your role cannot manage members")
		return
	}
	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, ok := NormalizeEmail(req.Email)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "a valid email is required")
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) > 200 {
		writeError(w, http.StatusUnprocessableEntity, "name must be at most 200 characters")
		return
	}
	if req.Role == "" {
		req.Role = auth.RoleViewer
	}
	if !auth.ValidRole(req.Role) {
		writeError(w, http.StatusUnprocessableEntity, "unknown role")
		return
	}
	if req.Role == auth.RoleOwner && user.Role != auth.RoleOwner {
		writeError(w, http.StatusForbidden, "only an owner can grant the owner role")
		return
	}

	member, err := h.store.Invite(r.Context(), user.TenantID, email, name, req.Role)
	if err != nil {
		h.fail(w, err, "invite member")
		return
	}
	writeJSON(w, http.StatusCreated, member)
}

func (h *Handler) handleSetRole(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	target, ok := h.authorizeChange(w, r, user)
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !auth.ValidRole(req.Role) {
		writeError(w, http.StatusUnprocessableEntity, "unknown role")
		return
	}
	if req.Role == auth.RoleOwner && user.Role != auth.RoleOwner {
		writeError(w, http.StatusForbidden, "only an owner can grant the owner role")
		return
	}

	member, err := h.store.SetRole(r.Context(), user.TenantID, target.ID, req.Role)
	if err != nil {
		h.fail(w, err, "change role")
		return
	}
	writeJSON(w, http.StatusOK, member)
}

func (h *Handler) handleRemove(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	target, ok := h.authorizeChange(w, r, user)
	if !ok {
		return
	}
	if err := h.store.Remove(r.Context(), user.TenantID, target.ID); err != nil {
		h.fail(w, err, "remove member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// authorizeChange loads the member addressed by the request and checks
// that the acting user may change it. It writes the response on refusal.
func (h *Handler) authorizeChange(w http.ResponseWriter, r *http.Request, user *auth.User) (*Member, bool) {
	if !user.CanManageMembers() {
		writeError(w, http.StatusForbidden, "your role cannot manage members")
		return nil, false
	}
	id := r.PathValue("id")
	members, err := h.store.ListMembers(r.Context(), user.TenantID)
	if err != nil {
		h.fail(w, err, "load member")
		return nil, false
	}
	var target *Member
	for i := range members {
		if members[i].ID == id {
			target = &members[i]
		}
	}
	switch {
	case target == nil:
		writeError(w, http.StatusNotFound, "member not found")
	case target.ID == user.ID:
		writeError(w, http.StatusForbidden, "you cannot change your own membership; ask another owner or administrator")
	case target.Role == auth.RoleOwner && user.Role != auth.RoleOwner:
		writeError(w, http.StatusForbidden, "only an owner can change another owner")
	default:
		return target, true
	}
	return nil, false
}

func (h *Handler) fail(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "member not found")
	case errors.Is(err, ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "a member with this email already exists", "code": "member_exists"})
	case errors.Is(err, ErrLastOwner):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "the organization must keep at least one owner", "code": "last_owner"})
	default:
		h.logger.Error("directory: "+what+" failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // client went away
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
