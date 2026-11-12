// Package database provides the PostgreSQL connection, tenant-scoped
// transactions and schema migrations for the SGSI platform.
//
// Multi-tenant isolation (ADR-002) relies on Row-Level Security. Every
// tenant-scoped query must run inside TenantTx, which sets `app.tenant_id`
// for the duration of the transaction. The application must connect with a
// role that is neither superuser nor table owner, otherwise PostgreSQL
// bypasses the policies; Open refuses nothing, but CheckRLSEnforced reports it.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/jsortizq/politicas-iso27001-pyme/catalog"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// Open connects to PostgreSQL and verifies the connection.
func Open(ctx context.Context, dsn string, maxConns int) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}
	if maxConns > 0 {
		db.SetMaxOpenConns(maxConns)
		db.SetMaxIdleConns(maxConns / 2)
	}
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return db, nil
}

// TenantTx runs fn inside a transaction scoped to tenantID.
//
// The tenant is set with set_config(..., is_local => true), the
// parameterised equivalent of SET LOCAL: it is cleared automatically on
// commit or rollback, so pooled connections never leak tenant context.
func TenantTx(ctx context.Context, db *sql.DB, tenantID string, fn func(tx *sql.Tx) error) error {
	if !tenant.IsValidUUID(tenantID) {
		return fmt.Errorf("database: invalid tenant id")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.tenant_id', $1, true)`, strings.ToLower(tenantID)); err != nil {
		return fmt.Errorf("database: set tenant: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	return nil
}

// CheckRLSEnforced returns an error when the current role would bypass
// Row-Level Security (superuser or BYPASSRLS). The API calls this at startup
// so that a misconfigured DATABASE_URL fails loudly instead of silently
// disabling tenant isolation.
func CheckRLSEnforced(ctx context.Context, db *sql.DB) error {
	var bypass bool
	var role string
	err := db.QueryRowContext(ctx,
		`SELECT rolname, rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`,
	).Scan(&role, &bypass)
	if err != nil {
		return fmt.Errorf("database: check role: %w", err)
	}
	if bypass {
		return fmt.Errorf("database: role %q bypasses row-level security; connect the API with a non-superuser role (see deploy/docker/initdb)", role)
	}
	return nil
}

// EnsureTenant creates the tenant row if it does not exist.
// It is used by development login only; production tenants are provisioned
// through the organization onboarding flow.
func EnsureTenant(ctx context.Context, db *sql.DB, tenantID, name string) error {
	if !tenant.IsValidUUID(tenantID) {
		return fmt.Errorf("database: invalid tenant id")
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO tenants (id, name, slug) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		tenantID, name, "org-"+strings.ToLower(tenantID))
	if err != nil {
		return fmt.Errorf("database: ensure tenant: %w", err)
	}
	return nil
}

// ── Migrations ──

// Migrate applies every pending *.up.sql file found in fsys, in filename
// order, each inside its own transaction. Applied versions are recorded in
// schema_migrations. It returns the versions applied by this call.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) ([]string, error) {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return nil, fmt.Errorf("database: create schema_migrations: %w", err)
	}

	names, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return nil, fmt.Errorf("database: list migrations: %w", err)
	}
	sort.Strings(names)

	var applied []string
	for _, name := range names {
		version := strings.TrimSuffix(name, ".up.sql")

		var exists bool
		if err := db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); err != nil {
			return applied, fmt.Errorf("database: check migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return applied, fmt.Errorf("database: read %s: %w", name, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return applied, fmt.Errorf("database: begin %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback() //nolint:errcheck
			return applied, fmt.Errorf("database: apply %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			tx.Rollback() //nolint:errcheck
			return applied, fmt.Errorf("database: record %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return applied, fmt.Errorf("database: commit %s: %w", version, err)
		}
		applied = append(applied, version)
	}
	return applied, nil
}

// SeedCatalog upserts the Annex A reference controls into control_catalog.
// It is idempotent and must run with the migration (administrator) role.
func SeedCatalog(ctx context.Context, db *sql.DB) (int, error) {
	controls, err := catalog.Controls()
	if err != nil {
		return 0, err
	}
	if len(controls) == 0 {
		return 0, errors.New("database: empty control catalog")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("database: begin seed: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO control_catalog (id, standard, category, clause_en, clause_es)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE
		SET standard = EXCLUDED.standard, category = EXCLUDED.category,
		    clause_en = EXCLUDED.clause_en, clause_es = EXCLUDED.clause_es`)
	if err != nil {
		return 0, fmt.Errorf("database: prepare seed: %w", err)
	}
	defer stmt.Close()

	for _, c := range controls {
		if _, err := stmt.ExecContext(ctx, c.ID, catalog.Standard, c.Category, c.Clause, c.ClauseES); err != nil {
			return 0, fmt.Errorf("database: seed %s: %w", c.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("database: commit seed: %w", err)
	}
	return len(controls), nil
}
