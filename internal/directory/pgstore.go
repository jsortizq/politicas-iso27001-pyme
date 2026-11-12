package directory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// PgStore persists organizations and members in PostgreSQL.
//
// Member management runs in tenant transactions (Row-Level Security). The
// sign-in lookup is the one query that crosses organizations; it is allowed
// by the auth_lookup_users policy, which only exposes the rows of the
// identity whose ID token was just verified (see migration 005).
type PgStore struct {
	db *sql.DB
}

// NewPgStore creates a store backed by db.
func NewPgStore(db *sql.DB) *PgStore { return &PgStore{db: db} }

func isUniqueViolation(err error) bool {
	var pqe *pq.Error
	return errors.As(err, &pqe) && pqe.Code == "23505"
}

// actor identifies who performs a member-management operation.
func actor(ctx context.Context) database.Actor {
	if u := auth.FromContext(ctx); u != nil {
		return database.Actor{Email: u.Email, Role: u.Role}
	}
	return database.Actor{}
}

// ── auth.Directory ──

func (s *PgStore) Resolve(ctx context.Context, id auth.Identity) ([]auth.Membership, error) {
	verifiedEmail := ""
	if id.EmailVerified {
		verifiedEmail, _ = NormalizeEmail(id.Email)
	}
	if id.Subject == "" && verifiedEmail == "" {
		return nil, nil
	}

	type row struct {
		auth.Membership
		pending bool
	}
	var rows []row

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("directory: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	if _, err := tx.ExecContext(ctx,
		`SELECT set_config('app.auth_sub', $1, true), set_config('app.auth_email', $2, true)`,
		id.Subject, verifiedEmail); err != nil {
		return nil, fmt.Errorf("directory: set identity: %w", err)
	}
	// The WHERE clause repeats the policy on purpose: two independent checks.
	result, err := tx.QueryContext(ctx, `
		SELECT u.id, u.tenant_id, t.name, u.email, u.name, u.role, u.oidc_sub IS NULL
		FROM users u JOIN tenants t ON t.id = u.tenant_id
		WHERE t.status = 'active'
		  AND (($1 <> '' AND u.oidc_sub = $1) OR ($2 <> '' AND u.oidc_sub IS NULL AND u.email = $2))
		ORDER BY t.name, u.tenant_id, u.oidc_sub IS NULL`,
		id.Subject, verifiedEmail)
	if err != nil {
		return nil, fmt.Errorf("directory: lookup: %w", err)
	}
	for result.Next() {
		var r row
		if err := result.Scan(&r.UserID, &r.TenantID, &r.TenantName, &r.Email, &r.Name, &r.Role, &r.pending); err != nil {
			result.Close()
			return nil, err
		}
		rows = append(rows, r)
	}
	result.Close()
	if err := result.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	var memberships []auth.Membership
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.TenantID] {
			continue // already a member there; the bound row sorts first
		}
		if r.pending && id.Subject != "" {
			accepted, err := s.acceptInvitation(ctx, r.Membership, id)
			if err != nil {
				return nil, err
			}
			if !accepted {
				continue
			}
			if r.Name == "" {
				r.Name = id.Name
			}
		}
		seen[r.TenantID] = true
		memberships = append(memberships, r.Membership)
	}
	return memberships, nil
}

// acceptInvitation binds a pending member record to the identity.
func (s *PgStore) acceptInvitation(ctx context.Context, m auth.Membership, id auth.Identity) (bool, error) {
	accepted := false
	err := database.TenantTx(ctx, s.db, m.TenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE users SET oidc_sub = $3, last_login = NOW(),
				name = CASE WHEN name = '' THEN $4 ELSE name END
			WHERE tenant_id = $1 AND id = $2 AND oidc_sub IS NULL`,
			m.TenantID, m.UserID, id.Subject, id.Name)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // accepted concurrently by another request
		}
		accepted = true
		return database.Audit(ctx, tx, m.TenantID, database.Actor{Email: m.Email, Role: m.Role},
			"member.join", "user", m.UserID, nil)
	})
	if isUniqueViolation(err) {
		return false, nil // this identity already has another record in the organization
	}
	return accepted, err
}

func (s *PgStore) Membership(ctx context.Context, tenantID, userID string) (*auth.Membership, error) {
	if !tenant.IsValidUUID(tenantID) || !tenant.IsValidUUID(userID) {
		return nil, auth.ErrNoMembership
	}
	var m auth.Membership
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT u.id, u.tenant_id, t.name, u.email, u.name, u.role
			FROM users u JOIN tenants t ON t.id = u.tenant_id
			WHERE u.tenant_id = $1 AND u.id = $2 AND t.status = 'active'`,
			tenantID, userID).Scan(&m.UserID, &m.TenantID, &m.TenantName, &m.Email, &m.Name, &m.Role)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, auth.ErrNoMembership
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *PgStore) CreateOrganization(ctx context.Context, name string, owner auth.Identity) (*auth.Membership, error) {
	email, ok := NormalizeEmail(owner.Email)
	if !ok {
		return nil, errors.New("directory: the identity has no usable e-mail address")
	}
	tenantID := tenant.NewUUID()
	m := auth.Membership{TenantID: tenantID, TenantName: name, Email: email, Name: owner.Name, Role: auth.RoleOwner}

	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tenants (id, name, slug) VALUES ($1, $2, $3)`,
			tenantID, name, slugify(name, tenantID[:8])); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO users (tenant_id, email, name, role, oidc_sub, last_login)
			VALUES ($1, $2, $3, 'owner', NULLIF($4, ''), NOW())
			RETURNING id`,
			tenantID, email, owner.Name, owner.Subject).Scan(&m.UserID); err != nil {
			return err
		}
		return database.Audit(ctx, tx, tenantID, database.Actor{Email: email, Role: auth.RoleOwner},
			"organization.create", "tenant", tenantID, map[string]any{"name": name})
	})
	if err != nil {
		return nil, fmt.Errorf("directory: create organization: %w", err)
	}
	return &m, nil
}

func (s *PgStore) EnsureDevUser(ctx context.Context, tenantID, email, name, role string) (*auth.Membership, error) {
	email, ok := NormalizeEmail(email)
	if !ok {
		return nil, errors.New("directory: invalid e-mail address")
	}
	if err := database.EnsureTenant(ctx, s.db, tenantID, devOrganizationName); err != nil {
		return nil, err
	}

	m := auth.Membership{TenantID: strings.ToLower(tenantID), Email: email}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT name FROM tenants WHERE id = $1`, tenantID).Scan(&m.TenantName); err != nil {
			return err
		}
		defaultRole := auth.RoleAdmin
		var members int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&members); err != nil {
			return err
		}
		if members == 0 {
			defaultRole = auth.RoleOwner
		}
		// $4 is the requested role ('' keeps the current one).
		return tx.QueryRowContext(ctx, `
			INSERT INTO users (tenant_id, email, name, role, last_login)
			VALUES ($1, $2, $3, COALESCE(NULLIF($4, ''), $5), NOW())
			ON CONFLICT (tenant_id, email) DO UPDATE
			SET role = COALESCE(NULLIF($4, ''), users.role), last_login = NOW()
			RETURNING id, name, role`,
			tenantID, email, name, role, defaultRole).Scan(&m.UserID, &m.Name, &m.Role)
	})
	if err != nil {
		return nil, fmt.Errorf("directory: ensure dev user: %w", err)
	}
	return &m, nil
}

// ── Member management ──

const memberColumns = `id, email, name, role, oidc_sub IS NOT NULL, last_login, created_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanMember(row scanner) (Member, error) {
	var m Member
	var bound bool
	var lastLogin sql.NullTime
	err := row.Scan(&m.ID, &m.Email, &m.Name, &m.Role, &bound, &lastLogin, &m.CreatedAt)
	if lastLogin.Valid {
		m.LastLogin = &lastLogin.Time
	}
	m.Status = statusOf(bound, m.LastLogin)
	return m, err
}

func (s *PgStore) ListMembers(ctx context.Context, tenantID string) ([]Member, error) {
	members := []Member{}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT `+memberColumns+` FROM users WHERE tenant_id = $1 ORDER BY created_at, email`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanMember(rows)
			if err != nil {
				return err
			}
			members = append(members, m)
		}
		return rows.Err()
	})
	return members, err
}

func (s *PgStore) Invite(ctx context.Context, tenantID, email, name, role string) (*Member, error) {
	var m Member
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var err error
		m, err = scanMember(tx.QueryRowContext(ctx, `
			INSERT INTO users (tenant_id, email, name, role) VALUES ($1, $2, $3, $4)
			RETURNING `+memberColumns, tenantID, email, name, role))
		if err != nil {
			return err
		}
		return database.Audit(ctx, tx, tenantID, actor(ctx), "member.invite", "user", m.ID,
			map[string]any{"email": email, "member_role": role})
	})
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// lockMember locks the organization's owners and the target member, and
// refuses changes that would leave the organization without an owner.
// newRole is "" for a removal. It returns the member's current role.
func lockMember(ctx context.Context, tx *sql.Tx, tenantID, memberID, newRole string) (string, error) {
	// Locking every owner row serialises concurrent demotions/removals, so
	// two requests cannot each see "another owner remains".
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM users WHERE tenant_id = $1 AND role = 'owner' ORDER BY id FOR UPDATE`, tenantID)
	if err != nil {
		return "", err
	}
	owners := 0
	for rows.Next() {
		owners++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}

	var current string
	err = tx.QueryRowContext(ctx,
		`SELECT role FROM users WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenantID, memberID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if current == auth.RoleOwner && newRole != auth.RoleOwner && owners <= 1 {
		return "", ErrLastOwner
	}
	return current, nil
}

func (s *PgStore) SetRole(ctx context.Context, tenantID, memberID, role string) (*Member, error) {
	if !tenant.IsValidUUID(memberID) {
		return nil, ErrNotFound
	}
	var m Member
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		before, err := lockMember(ctx, tx, tenantID, memberID, role)
		if err != nil {
			return err
		}
		m, err = scanMember(tx.QueryRowContext(ctx, `
			UPDATE users SET role = $3 WHERE tenant_id = $1 AND id = $2
			RETURNING `+memberColumns, tenantID, memberID, role))
		if err != nil {
			return err
		}
		return database.Audit(ctx, tx, tenantID, actor(ctx), "member.role_change", "user", memberID,
			map[string]any{"email": m.Email, "before": before, "after": role})
	})
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *PgStore) Remove(ctx context.Context, tenantID, memberID string) error {
	if !tenant.IsValidUUID(memberID) {
		return ErrNotFound
	}
	return database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		role, err := lockMember(ctx, tx, tenantID, memberID, "")
		if err != nil {
			return err
		}
		var email string
		if err := tx.QueryRowContext(ctx,
			`DELETE FROM users WHERE tenant_id = $1 AND id = $2 RETURNING email`,
			tenantID, memberID).Scan(&email); err != nil {
			return err
		}
		return database.Audit(ctx, tx, tenantID, actor(ctx), "member.remove", "user", memberID,
			map[string]any{"email": email, "member_role": role})
	})
}
