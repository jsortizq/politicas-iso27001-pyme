// Package dbtest provisions a throwaway PostgreSQL database for integration
// tests. Tests are skipped unless TEST_DATABASE_URL points at a server where
// the given role may create databases and roles (e.g. the docker-compose
// administrator: postgres://sgsi:sgsi_dev@localhost:5432/sgsi?sslmode=disable).
//
// The database named in the URL is never modified: each call creates its own
// database, migrates it, and drops it when the test finishes.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
	"github.com/jsortizq/politicas-iso27001-pyme/migrations"
)

const (
	appRole     = "sgsi_app_test"
	appPassword = "sgsi_app_test"
)

// DBs holds the two connections a test needs.
type DBs struct {
	Admin *sql.DB // owner/superuser: bypasses RLS, used for setup and assertions
	App   *sql.DB // non-superuser member of app_user: subject to RLS, like the API
}

// New creates a migrated, catalog-seeded database and returns connections to it.
func New(t *testing.T) DBs {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()

	root, err := database.Open(ctx, base, 2)
	if err != nil {
		t.Fatalf("dbtest: connect: %v", err)
	}

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "sgsi_test_" + hex.EncodeToString(suffix)
	if _, err := root.ExecContext(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatalf("dbtest: create database: %v", err)
	}

	adminURL := withDatabase(t, base, name, nil)
	admin, err := database.Open(ctx, adminURL, 5)
	if err != nil {
		t.Fatalf("dbtest: connect admin: %v", err)
	}

	if _, err := database.Migrate(ctx, admin, migrations.FS); err != nil {
		t.Fatalf("dbtest: migrate: %v", err)
	}
	if _, err := database.SeedCatalog(ctx, admin); err != nil {
		t.Fatalf("dbtest: seed: %v", err)
	}

	// Login role for the application side. Roles are cluster-wide, so it is
	// created once and reused by later runs.
	if _, err := admin.ExecContext(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '`+appRole+`') THEN
				CREATE ROLE `+appRole+` LOGIN PASSWORD '`+appPassword+`' IN ROLE app_user;
			END IF;
		END
		$$`); err != nil {
		t.Fatalf("dbtest: create app role: %v", err)
	}

	app, err := database.Open(ctx, withDatabase(t, base, name, url.UserPassword(appRole, appPassword)), 10)
	if err != nil {
		t.Fatalf("dbtest: connect app: %v", err)
	}

	t.Cleanup(func() {
		app.Close()
		admin.Close()
		root.ExecContext(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`) //nolint:errcheck
		root.Close()
	})
	return DBs{Admin: admin, App: app}
}

// Tenant inserts a tenant row and returns its id.
func Tenant(t *testing.T, admin *sql.DB, id, name string) string {
	t.Helper()
	if err := database.EnsureTenant(context.Background(), admin, id, name); err != nil {
		t.Fatalf("dbtest: tenant: %v", err)
	}
	return id
}

func withDatabase(t *testing.T, base, name string, user *url.Userinfo) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" {
		t.Fatalf("dbtest: TEST_DATABASE_URL must be a postgres:// URL")
	}
	u.Path = "/" + name
	if user != nil {
		u.User = user
	}
	return u.String()
}
