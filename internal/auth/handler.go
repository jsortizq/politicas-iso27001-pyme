package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// sessionCookie is the name of the HttpOnly cookie carrying the session token.
const sessionCookie = "sgsi_session"

// codeNoOrganization tells clients to send the person to organization
// selection or onboarding.
const codeNoOrganization = "no_organization"

// Options configures the auth Handler.
type Options struct {
	// DevMode enables POST /auth/login, which accepts any credentials.
	DevMode bool
	// OIDC enables single sign-on when its Issuer is set.
	OIDC OIDCConfig
	// AppURL is the public URL of the web application; people are sent
	// back to it after single sign-on.
	AppURL string
	// SecureCookies marks cookies Secure (HTTPS only).
	SecureCookies bool
	// AllowSignup lets any signed-in person create a new organization.
	AllowSignup bool
	Logger      *slog.Logger
}

// Handler serves the /auth endpoints and the API authentication middleware.
type Handler struct {
	sessions *SessionStore
	dir      Directory
	opts     Options
	logger   *slog.Logger
	oidc     *oidcProvider // nil when single sign-on is not configured
}

// NewHandler creates an auth handler.
func NewHandler(sessions *SessionStore, dir Directory, opts Options) *Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	opts.AppURL = strings.TrimRight(opts.AppURL, "/")
	h := &Handler{sessions: sessions, dir: dir, opts: opts, logger: logger}
	if opts.OIDC.Issuer != "" {
		h.oidc = newOIDCProvider(opts.OIDC)
	}
	return h
}

// Register wires auth routes into the mux.
// These routes do NOT go through tenant middleware.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/config", h.handleConfig)
	mux.HandleFunc("POST /auth/login", h.handleDevLogin)
	mux.HandleFunc("POST /auth/logout", h.handleLogout)
	mux.HandleFunc("GET /auth/session", h.handleSession)
	mux.HandleFunc("POST /auth/organizations", h.handleCreateOrganization)
	mux.HandleFunc("POST /auth/switch", h.handleSwitch)
	mux.HandleFunc("GET /auth/oidc/login", h.handleOIDCLogin)
	mux.HandleFunc("GET /auth/callback", h.handleOIDCCallback)
}

// Middleware authenticates API requests. It requires a session with a
// current membership, injects the user into the context, and overwrites the
// X-Tenant-ID header with the session's organization, so a client can never
// choose which organization's data it reaches.
func (h *Handler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := h.sessions.Get(extractToken(r))
		if session == nil {
			writeAuthError(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		if session.TenantID == "" {
			writeAuthError(w, http.StatusForbidden, "select or create an organization first", codeNoOrganization)
			return
		}

		// Read membership on every request: a removed member loses access
		// and a role change applies without waiting for a new login.
		membership, err := h.dir.Membership(r.Context(), session.TenantID, session.UserID)
		if errors.Is(err, ErrNoMembership) {
			h.sessions.SetMembership(session.Token, nil)
			writeAuthError(w, http.StatusForbidden, "you no longer belong to this organization", codeNoOrganization)
			return
		}
		if err != nil {
			h.logger.Error("membership check failed", "error", err)
			writeAuthError(w, http.StatusInternalServerError, "internal error", "")
			return
		}

		r.Header.Set("X-Tenant-ID", membership.TenantID)
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), membership.user())))
	})
}

// ── Endpoints ──

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"dev_login": h.opts.DevMode,
		"sso":       h.oidc != nil,
		"signup":    h.opts.AllowSignup,
	})
}

type devLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"` // optional
}

type sessionResponse struct {
	// Token is returned on login for non-browser clients (Authorization:
	// Bearer). Browsers use the HttpOnly cookie and should ignore it.
	Token             string       `json:"token,omitempty"`
	Identity          Identity     `json:"identity"`
	User              *User        `json:"user"`
	Organizations     []Membership `json:"organizations"`
	NeedsOrganization bool         `json:"needs_organization"`
	CanCreateOrg      bool         `json:"can_create_organization"`
	ExpiresAt         time.Time    `json:"expires_at"`
}

func (h *Handler) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !h.opts.DevMode {
		writeAuthError(w, http.StatusNotImplemented, "password login is disabled; use single sign-on at /auth/oidc/login", "")
		return
	}

	var req devLoginRequest
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body", "")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		writeAuthError(w, http.StatusBadRequest, "a valid email is required", "")
		return
	}
	if !tenant.IsValidUUID(req.TenantID) {
		writeAuthError(w, http.StatusBadRequest, "tenant_id must be a UUID", "")
		return
	}
	if req.Role != "" && !ValidRole(req.Role) {
		writeAuthError(w, http.StatusBadRequest, "unknown role", "")
		return
	}

	// In dev mode, accept any email/password combination.
	name := nameFromEmail(email)
	membership, err := h.dir.EnsureDevUser(r.Context(), strings.ToLower(req.TenantID), email, name, req.Role)
	if err != nil {
		h.logger.Error("dev login failed", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to prepare the development user", "")
		return
	}

	id := Identity{Email: email, EmailVerified: true, Name: name}
	token, err := h.sessions.Create(id, membership)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to create session", "")
		return
	}
	h.setSessionCookie(w, token)
	h.writeSession(w, r, h.sessions.Get(token), true)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := extractToken(r); token != "" {
		h.sessions.Delete(token)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.opts.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSession(w http.ResponseWriter, r *http.Request) {
	session := h.sessions.Get(extractToken(r))
	if session == nil {
		writeAuthError(w, http.StatusUnauthorized, "no session", "")
		return
	}
	h.writeSession(w, r, session, false)
}

func (h *Handler) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	session := h.sessions.Get(extractToken(r))
	if session == nil {
		writeAuthError(w, http.StatusUnauthorized, "authentication required", "")
		return
	}
	if !h.opts.AllowSignup {
		writeAuthError(w, http.StatusForbidden, "creating organizations is disabled; ask an administrator for an invitation", "")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body", "")
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) < 2 || len(name) > 120 {
		writeAuthError(w, http.StatusUnprocessableEntity, "name must be between 2 and 120 characters", "")
		return
	}

	membership, err := h.dir.CreateOrganization(r.Context(), name, session.Identity)
	if err != nil {
		h.logger.Error("create organization failed", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to create the organization", "")
		return
	}
	h.sessions.SetMembership(session.Token, membership)
	h.logger.Info("organization created", "tenant", membership.TenantID, "owner", membership.Email)

	w.Header().Set("Content-Type", "application/json")
	h.writeSessionStatus(w, r, h.sessions.Get(session.Token), false, http.StatusCreated)
}

func (h *Handler) handleSwitch(w http.ResponseWriter, r *http.Request) {
	session := h.sessions.Get(extractToken(r))
	if session == nil {
		writeAuthError(w, http.StatusUnauthorized, "authentication required", "")
		return
	}
	var req struct {
		TenantID string `json:"tenant_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !tenant.IsValidUUID(req.TenantID) {
		writeAuthError(w, http.StatusBadRequest, "tenant_id must be a UUID", "")
		return
	}

	memberships, err := h.dir.Resolve(r.Context(), session.Identity)
	if err != nil {
		h.logger.Error("resolve memberships failed", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "internal error", "")
		return
	}
	for i := range memberships {
		if memberships[i].TenantID == strings.ToLower(req.TenantID) {
			h.sessions.SetMembership(session.Token, &memberships[i])
			h.writeSession(w, r, h.sessions.Get(session.Token), false)
			return
		}
	}
	// Same answer whether the organization exists or not.
	writeAuthError(w, http.StatusForbidden, "you do not belong to that organization", "")
}

// writeSession reports who is signed in, the active organization and the
// organizations the person can switch to.
func (h *Handler) writeSession(w http.ResponseWriter, r *http.Request, session *Session, withToken bool) {
	h.writeSessionStatus(w, r, session, withToken, http.StatusOK)
}

func (h *Handler) writeSessionStatus(w http.ResponseWriter, r *http.Request, session *Session, withToken bool, status int) {
	if session == nil {
		writeAuthError(w, http.StatusUnauthorized, "no session", "")
		return
	}
	memberships, err := h.dir.Resolve(r.Context(), session.Identity)
	if err != nil {
		h.logger.Error("resolve memberships failed", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "internal error", "")
		return
	}

	resp := sessionResponse{
		Identity:      session.Identity,
		Organizations: memberships,
		CanCreateOrg:  h.opts.AllowSignup,
		ExpiresAt:     session.ExpiresAt,
	}
	if resp.Organizations == nil {
		resp.Organizations = []Membership{}
	}
	if withToken {
		resp.Token = session.Token
	}
	if session.TenantID != "" {
		membership, err := h.dir.Membership(r.Context(), session.TenantID, session.UserID)
		switch {
		case err == nil:
			resp.User = membership.user()
		case errors.Is(err, ErrNoMembership):
			h.sessions.SetMembership(session.Token, nil)
		default:
			h.logger.Error("membership check failed", "error", err)
			writeAuthError(w, http.StatusInternalServerError, "internal error", "")
			return
		}
	}
	resp.NeedsOrganization = resp.User == nil
	writeJSON(w, status, resp)
}

// ── Helpers ──

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string) {
	// Browsers authenticate with this cookie: HttpOnly keeps the token away
	// from scripts and SameSite=Lax keeps it off cross-site POST/PUT/DELETE.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(h.sessions.ttl),
		HttpOnly: true,
		Secure:   h.opts.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		return cookie.Value
	}
	return ""
}

func nameFromEmail(email string) string {
	local, _, _ := strings.Cut(email, "@")
	name := strings.ReplaceAll(local, ".", " ")
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // client went away
}

func writeAuthError(w http.ResponseWriter, status int, msg, code string) {
	body := map[string]string{"error": msg}
	if code != "" {
		body["code"] = code
	}
	writeJSON(w, status, body)
}
