// Package directory manages organizations and their members.
//
// A person (an identity from the identity provider) has one member record
// per organization they belong to. Members are invited by e-mail; the
// invitation is accepted the first time that person signs in with an
// identity provider that confirms the e-mail address.
package directory

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
)

// Errors returned by Store implementations.
var (
	ErrNotFound  = errors.New("directory: member not found")
	ErrConflict  = errors.New("directory: member already exists")
	ErrLastOwner = errors.New("directory: an organization must keep at least one owner")
)

// Member is a person's record within one organization.
type Member struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	Status    string     `json:"status"` // "invited" until the first sign-in, then "active"
	LastLogin *time.Time `json:"last_login,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Store persists organizations and members. It also implements the
// auth.Directory operations used at sign-in.
type Store interface {
	auth.Directory

	ListMembers(ctx context.Context, tenantID string) ([]Member, error)
	// Invite adds a member who has not signed in yet.
	Invite(ctx context.Context, tenantID, email, name, role string) (*Member, error)
	// SetRole changes a member's role. It refuses to demote the last owner.
	SetRole(ctx context.Context, tenantID, memberID, role string) (*Member, error)
	// Remove deletes a member. It refuses to remove the last owner.
	Remove(ctx context.Context, tenantID, memberID string) error
}

// devOrganizationName is given to organizations created by the development login.
const devOrganizationName = "Organización de desarrollo"

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// NormalizeEmail lower-cases and trims an address and reports whether it
// looks like an e-mail address.
func NormalizeEmail(email string) (string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	return email, len(email) <= 254 && emailPattern.MatchString(email)
}

func statusOf(bound bool, lastLogin *time.Time) string {
	if bound || lastLogin != nil {
		return "active"
	}
	return "invited"
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify builds a URL-safe organization slug; suffix keeps it unique.
func slugify(name, suffix string) string {
	slug := strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	if slug == "" {
		slug = "org"
	}
	return slug + "-" + suffix
}
