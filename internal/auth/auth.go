// Package auth provides authentication and session management for the SGSI API.
//
// It supports two modes:
//  1. OIDC: Full OpenID Connect flow with an external identity provider
//  2. Dev mode: Simple session-based auth for local development
//
// Dev mode accepts any credentials, so it is only enabled when
// AUTH_DEV_MODE=true is set explicitly. Without it (and until the OIDC flow
// is implemented) the login endpoint answers 501.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// Roles, from most to least privileged. Owner, admin and editor may modify
// SGSI records; auditor and viewer have read-only access.
const (
	RoleOwner   = "owner"
	RoleAdmin   = "admin"
	RoleEditor  = "editor"
	RoleAuditor = "auditor"
	RoleViewer  = "viewer"
)

// ValidRole reports whether role is a known role.
func ValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleEditor, RoleAuditor, RoleViewer:
		return true
	}
	return false
}

// CanWrite reports whether the user may create, change or delete records.
func (u *User) CanWrite() bool {
	return u != nil && (u.Role == RoleOwner || u.Role == RoleAdmin || u.Role == RoleEditor)
}

// sessionCookie is the name of the HttpOnly cookie carrying the session token.
const sessionCookie = "sgsi_session"

// contextKey is an unexported type for context keys.
type contextKey string

const userKey contextKey = "user"

// User represents an authenticated user.
type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"` // see the Role* constants
}

// FromContext extracts the authenticated user from the context.
func FromContext(ctx context.Context) *User {
	if u, ok := ctx.Value(userKey).(*User); ok {
		return u
	}
	return nil
}

// WithUser returns a context with the given user.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// ── Session Store ──

// Session represents a user session.
type Session struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	TenantID  string    `json:"tenant_id"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SessionStore manages user sessions.
// In production, this would use Redis; for now, in-memory.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewSessionStore creates a store with the given session TTL.
func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

// Create generates a new session for the user and returns the token.
func (s *SessionStore) Create(user *User) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("auth: failed to generate token: %w", err)
	}

	session := &Session{
		Token:     token,
		UserID:    user.ID,
		Email:     user.Email,
		Name:      user.Name,
		TenantID:  user.TenantID,
		Role:      user.Role,
		ExpiresAt: time.Now().Add(s.ttl),
	}

	s.mu.Lock()
	// Drop expired sessions so abandoned logins do not accumulate.
	now := time.Now()
	for t, old := range s.sessions {
		if now.After(old.ExpiresAt) {
			delete(s.sessions, t)
		}
	}
	s.sessions[token] = session
	s.mu.Unlock()

	return token, nil
}

// Get retrieves a session by token. Returns nil if not found or expired.
func (s *SessionStore) Get(token string) *Session {
	s.mu.RLock()
	session, ok := s.sessions[token]
	s.mu.RUnlock()

	if !ok {
		return nil
	}

	if time.Now().After(session.ExpiresAt) {
		s.Delete(token)
		return nil
	}

	return session
}

// Delete removes a session.
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// ── Auth Handler ──

// Handler provides login/logout/session endpoints.
type Handler struct {
	sessions *SessionStore
	devMode  bool

	// secureCookies marks the session cookie Secure (HTTPS only).
	secureCookies bool
	// provisionTenant, if set, is called on dev login so the tenant exists
	// in the database before the session is used.
	provisionTenant func(ctx context.Context, tenantID string) error
}

// SetSecureCookies controls the Secure attribute of the session cookie.
// Enable it whenever the platform is served over HTTPS.
func (h *Handler) SetSecureCookies(secure bool) { h.secureCookies = secure }

// SetTenantProvisioner registers a hook that makes sure the tenant exists
// when a development session is created.
func (h *Handler) SetTenantProvisioner(fn func(ctx context.Context, tenantID string) error) {
	h.provisionTenant = fn
}

// NewHandler creates an auth handler.
// If devMode is true, a simple login endpoint is available
// that doesn't require an OIDC provider.
func NewHandler(sessions *SessionStore, devMode bool) *Handler {
	return &Handler{
		sessions: sessions,
		devMode:  devMode,
	}
}

// Register wires auth routes into the mux.
// These routes do NOT go through tenant middleware.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", h.handleLogin)
	mux.HandleFunc("POST /auth/logout", h.handleLogout)
	mux.HandleFunc("GET /auth/session", h.handleSession)
}

// Middleware validates the session token and injects the user into context.
// It also overwrites the X-Tenant-ID header with the session's tenant, so a
// client can never choose which organization's data it reaches.
func (h *Handler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		session := h.sessions.Get(token)
		if session == nil {
			writeAuthError(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}

		user := &User{
			ID:       session.UserID,
			Email:    session.Email,
			Name:     session.Name,
			TenantID: session.TenantID,
			Role:     session.Role,
		}

		ctx := WithUser(r.Context(), user)

		// Set tenant header so tenant middleware can pick it up
		r.Header.Set("X-Tenant-ID", session.TenantID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ── Handler implementations ──

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"` // dev mode only; defaults to admin
}

type loginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !h.devMode {
		writeAuthError(w, http.StatusNotImplemented, "use OIDC login flow at /auth/oidc/login")
		return
	}

	var req loginRequest
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" {
		writeAuthError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.TenantID == "" {
		writeAuthError(w, http.StatusBadRequest, "tenant_id is required")
		return
	}

	if !tenant.IsValidUUID(req.TenantID) {
		writeAuthError(w, http.StatusBadRequest, "tenant_id must be a UUID")
		return
	}
	if req.Role == "" {
		req.Role = RoleAdmin
	}
	if !ValidRole(req.Role) {
		writeAuthError(w, http.StatusBadRequest, "unknown role")
		return
	}
	tenantID := strings.ToLower(req.TenantID)

	if h.provisionTenant != nil {
		if err := h.provisionTenant(r.Context(), tenantID); err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to prepare organization")
			return
		}
	}

	// In dev mode, accept any email/password combination
	user := &User{
		ID:       "dev-" + req.Email,
		Email:    req.Email,
		Name:     nameFromEmail(req.Email),
		TenantID: tenantID,
		Role:     req.Role,
	}

	token, err := h.sessions.Create(user)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	// Browsers authenticate with this cookie: HttpOnly keeps the token away
	// from scripts and SameSite=Lax keeps it off cross-site POST/PUT/DELETE.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(h.sessions.ttl),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(loginResponse{
		Token: token,
		User:  *user,
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token != "" {
		h.sessions.Delete(token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSession(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token == "" {
		writeAuthError(w, http.StatusUnauthorized, "no session")
		return
	}

	session := h.sessions.Get(token)
	if session == nil {
		writeAuthError(w, http.StatusUnauthorized, "invalid or expired session")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"user": User{
			ID:       session.UserID,
			Email:    session.Email,
			Name:     session.Name,
			TenantID: session.TenantID,
			Role:     session.Role,
		},
		"expires_at": session.ExpiresAt,
	})
}

// ── Helpers ──

func extractToken(r *http.Request) string {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Check cookie
	cookie, err := r.Cookie(sessionCookie)
	if err == nil {
		return cookie.Value
	}

	return ""
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func nameFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 0 {
		return email
	}
	name := strings.ReplaceAll(parts[0], ".", " ")
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
