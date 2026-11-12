// Package tenant provides multi-tenant context management for the SGSI platform.
//
// It implements ADR-002 (PostgreSQL RLS for multi-tenant isolation) by:
//  1. Extracting tenant_id from the request (JWT claim or header)
//  2. Setting the PostgreSQL session variable `app.tenant_id`
//  3. Ensuring all queries through that connection are RLS-filtered
package tenant

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const tenantIDKey contextKey = "tenant_id"

// uuidPattern validates UUID v4 format.
var uuidPattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

// IsValidUUID checks whether s is a valid UUID string.
func IsValidUUID(s string) bool {
	return uuidPattern.MatchString(strings.ToLower(s))
}

// NewUUID returns a random (version 4) UUID.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("tenant: crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// FromContext extracts the tenant ID from the request context.
// Returns empty string if no tenant ID is set.
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(tenantIDKey).(string); ok {
		return id
	}
	return ""
}

// WithTenantID returns a new context with the given tenant ID.
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, tenantIDKey, id)
}

// Middleware extracts the tenant ID from the request and adds it to the context.
// It looks for the tenant ID in:
//  1. The "X-Tenant-ID" header (for API clients)
//  2. The JWT claims (when OIDC is configured)
//
// If no valid tenant ID is found, it returns 403 Forbidden.
//
// SECURITY: this middleware trusts the X-Tenant-ID header. It must only run
// behind auth.Middleware, which overwrites that header with the tenant of
// the authenticated session. Never expose it directly to clients.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantStr := r.Header.Get("X-Tenant-ID")

		// Fallback: extract from Authorization bearer token claims
		if tenantStr == "" {
			tenantStr = extractFromBearer(r)
		}

		if tenantStr == "" {
			http.Error(w, `{"error":"missing tenant context"}`, http.StatusForbidden)
			return
		}

		if !IsValidUUID(tenantStr) {
			http.Error(w, `{"error":"invalid tenant ID"}`, http.StatusBadRequest)
			return
		}

		ctx := WithTenantID(r.Context(), strings.ToLower(tenantStr))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SetOnConnection sets the PostgreSQL session variable `app.tenant_id`
// for RLS policy enforcement. Call this after acquiring a connection
// from the pool, before executing any tenant-scoped query.
func SetOnConnection(ctx context.Context, conn *sql.Conn) error {
	tenantID := FromContext(ctx)
	if tenantID == "" {
		return fmt.Errorf("tenant: no tenant_id in context")
	}

	_, err := conn.ExecContext(ctx,
		`SELECT set_config('app.tenant_id', $1, true)`, tenantID)
	return err
}

// SetOnTx sets the PostgreSQL session variable within a transaction.
// This is the preferred method: SET LOCAL scopes the setting to the
// current transaction, ensuring it's automatically cleared on commit/rollback.
func SetOnTx(ctx context.Context, tx *sql.Tx) error {
	tenantID := FromContext(ctx)
	if tenantID == "" {
		return fmt.Errorf("tenant: no tenant_id in context")
	}

	_, err := tx.ExecContext(ctx,
		`SELECT set_config('app.tenant_id', $1, true)`, tenantID)
	return err
}

// extractFromBearer attempts to extract tenant_id from a JWT bearer token.
// This is a placeholder for OIDC integration — in production, this would
// validate the JWT and extract the tenant_id claim.
func extractFromBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	// TODO: Implement JWT validation and claim extraction
	return ""
}
