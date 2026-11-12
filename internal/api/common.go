// Package api provides HTTP handlers for the SGSI REST API.
//
// Each resource (SoA, assets, risks) has a handler, a store interface and
// two store implementations: in-memory (development and unit tests) and
// PostgreSQL (tenant-scoped transactions with Row-Level Security).
package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
)

// Store errors shared by every implementation.
var (
	// ErrNotFound means the record does not exist for the current tenant.
	ErrNotFound = errors.New("not found")
	// ErrConflict means the record violates a uniqueness rule.
	ErrConflict = errors.New("conflict")
)

// validationError is returned for input that fails business validation.
type validationError struct {
	msg string
}

func (e *validationError) Error() string { return e.msg }

func invalid(format string, args ...any) error {
	return &validationError{fmt.Sprintf(format, args...)}
}

// maxBodyBytes caps request bodies; SGSI records are small JSON documents.
const maxBodyBytes = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // client went away
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeStoreError maps store errors to HTTP statuses. It reports whether the
// error was an unexpected one that the caller should log.
func writeStoreError(w http.ResponseWriter, err error, what string) (unexpected bool) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, what+" not found")
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, what+" already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
		return true
	}
	return false
}

// newUUID returns a random (version 4) UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("api: crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// memTable is a thread-safe in-memory table partitioned by tenant.
type memTable[T any] struct {
	mu   sync.RWMutex
	rows map[string]map[string]T // tenant_id -> id -> row
}

func newMemTable[T any]() *memTable[T] {
	return &memTable[T]{rows: make(map[string]map[string]T)}
}

func (m *memTable[T]) list(tenantID string) []T {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]T, 0, len(m.rows[tenantID]))
	for _, row := range m.rows[tenantID] {
		out = append(out, row)
	}
	return out
}

func (m *memTable[T]) get(tenantID, id string) (T, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row, ok := m.rows[tenantID][id]
	return row, ok
}

// insert stores row unless clash reports that an existing row conflicts.
func (m *memTable[T]) insert(tenantID, id string, row T, clash func(existing T) bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.rows[tenantID] {
		if clash != nil && clash(existing) {
			return ErrConflict
		}
	}
	if m.rows[tenantID] == nil {
		m.rows[tenantID] = make(map[string]T)
	}
	m.rows[tenantID][id] = row
	return nil
}

// replace overwrites an existing row; clash is evaluated against the others.
func (m *memTable[T]) replace(tenantID, id string, row T, clash func(otherID string, other T) bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[tenantID][id]; !ok {
		return ErrNotFound
	}
	for otherID, other := range m.rows[tenantID] {
		if otherID != id && clash != nil && clash(otherID, other) {
			return ErrConflict
		}
	}
	m.rows[tenantID][id] = row
	return nil
}

func (m *memTable[T]) remove(tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[tenantID][id]; !ok {
		return ErrNotFound
	}
	delete(m.rows[tenantID], id)
	return nil
}
