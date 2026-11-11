// Package auth provides authentication and session management for the SGSI API.
//
// It supports two modes:
//  1. OIDC: Full OpenID Connect flow with an external identity provider
//  2. Dev mode: Simple session-based auth for local development
//
// When OIDC_ISSUER is not configured, the server runs in dev mode.
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
)

// contextKey is an unexported type for context keys.
type contextKey string

const userKey contextKey = "user"

// User represents an authenticated user.
type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"` // "admin", "auditor", "viewer"
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
// It also sets the X-Tenant-ID header for downstream tenant middleware.
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

	// In dev mode, accept any email/password combination
	user := &User{
		ID:       "dev-" + req.Email,
		Email:    req.Email,
		Name:     nameFromEmail(req.Email),
		TenantID: req.TenantID,
		Role:     "admin",
	}

	token, err := h.sessions.Create(user)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

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
	cookie, err := r.Cookie("sgsi_session")
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
