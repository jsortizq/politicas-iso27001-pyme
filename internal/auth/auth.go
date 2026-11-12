// Package auth provides authentication, sessions and organization selection
// for the SGSI API.
//
// People sign in in one of two ways:
//
//  1. OIDC: Authorization Code flow with PKCE against an external identity
//     provider (Keycloak, Google, Microsoft Entra ID, ...). See oidc.go.
//  2. Development login: accepts any credentials. It exists only for local
//     work and must be enabled explicitly with AUTH_DEV_MODE=true.
//
// A session identifies a person and, once chosen, the organization they are
// working in. Membership and role are read from the Directory on every
// request, so removing a member or changing a role takes effect immediately.
package auth

import (
	"context"
	"errors"
)

// Roles, from most to least privileged. Owner, admin and editor may modify
// SGSI records; auditor and viewer have read-only access. Owner and admin
// manage members.
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

// User is the authenticated member of the organization a request acts on.
type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"` // see the Role* constants
}

// CanWrite reports whether the user may create, change or delete SGSI records.
func (u *User) CanWrite() bool {
	return u != nil && (u.Role == RoleOwner || u.Role == RoleAdmin || u.Role == RoleEditor)
}

// CanManageMembers reports whether the user may invite, change or remove members.
func (u *User) CanManageMembers() bool {
	return u != nil && (u.Role == RoleOwner || u.Role == RoleAdmin)
}

// CanViewAudit reports whether the user may read the audit trail.
func (u *User) CanViewAudit() bool {
	return u != nil && (u.Role == RoleOwner || u.Role == RoleAdmin || u.Role == RoleAuditor)
}

type contextKey string

const userKey contextKey = "user"

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

// Identity is a person as asserted by the identity provider.
type Identity struct {
	Subject       string `json:"-"` // OIDC "sub"; empty for development sessions
	Email         string `json:"email"`
	EmailVerified bool   `json:"-"`
	Name          string `json:"name"`
}

// Membership links an identity to one organization.
type Membership struct {
	UserID     string `json:"user_id"`
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	Role       string `json:"role"`
}

func (m *Membership) user() *User {
	return &User{ID: m.UserID, Email: m.Email, Name: m.Name, TenantID: m.TenantID, Role: m.Role}
}

// ErrNoMembership means the user does not (or no longer does) belong to the organization.
var ErrNoMembership = errors.New("auth: no membership")

// Directory is the membership store the auth layer depends on.
// Implementations live in internal/directory.
type Directory interface {
	// Resolve returns the organizations an identity belongs to, most
	// recently used first. Pending invitations addressed to the identity's
	// e-mail are accepted here, but only if the provider verified that e-mail.
	Resolve(ctx context.Context, id Identity) ([]Membership, error)

	// Membership returns the current membership of a user in an
	// organization, or ErrNoMembership.
	Membership(ctx context.Context, tenantID, userID string) (*Membership, error)

	// CreateOrganization creates an organization owned by the identity.
	CreateOrganization(ctx context.Context, name string, owner Identity) (*Membership, error)

	// EnsureDevUser creates or updates a member for the development login.
	// An empty role keeps the member's current role; new members default to
	// owner in an empty organization and admin otherwise.
	EnsureDevUser(ctx context.Context, tenantID, email, name, role string) (*Membership, error)
}
