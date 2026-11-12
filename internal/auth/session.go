package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Session is a signed-in person and the organization they are working in.
type Session struct {
	Token     string
	Identity  Identity
	UserID    string // empty until an organization is selected
	TenantID  string // empty until an organization is selected
	ExpiresAt time.Time
}

// SessionStore keeps sessions in memory.
//
// Sessions are lost when the process restarts and are not shared between
// instances; the Redis-backed store planned in the architecture replaces
// this before running more than one API instance.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewSessionStore creates a store with the given session TTL.
func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session), ttl: ttl}
}

// Create opens a session for the identity and returns its token.
// membership may be nil when the person has not chosen an organization yet.
func (s *SessionStore) Create(id Identity, membership *Membership) (string, error) {
	token, err := randomHex(32)
	if err != nil {
		return "", fmt.Errorf("auth: failed to generate token: %w", err)
	}
	session := &Session{Token: token, Identity: id, ExpiresAt: time.Now().Add(s.ttl)}
	if membership != nil {
		session.UserID, session.TenantID = membership.UserID, membership.TenantID
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

// Get returns a copy of the session, or nil if it is unknown or expired.
func (s *SessionStore) Get(token string) *Session {
	s.mu.RLock()
	session, ok := s.sessions[token]
	var snapshot Session
	if ok {
		snapshot = *session
	}
	s.mu.RUnlock()

	if !ok {
		return nil
	}
	if time.Now().After(snapshot.ExpiresAt) {
		s.Delete(token)
		return nil
	}
	return &snapshot
}

// SetMembership changes the organization a session works in.
// A nil membership leaves the session without an organization.
func (s *SessionStore) SetMembership(token string, membership *Membership) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[token]
	if !ok {
		return
	}
	if membership == nil {
		session.UserID, session.TenantID = "", ""
		return
	}
	session.UserID, session.TenantID = membership.UserID, membership.TenantID
}

// Delete removes a session.
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
