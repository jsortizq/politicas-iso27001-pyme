package database_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database/dbtest"
	"github.com/jsortizq/politicas-iso27001-pyme/migrations"
)

const (
	tenantA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	tenantB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func TestTenantTx_RejectsInvalidTenant(t *testing.T) {
	// No database needed: validation happens before any query.
	err := database.TenantTx(context.Background(), nil, "not-a-uuid", func(*sql.Tx) error { return nil })
	if err == nil {
		t.Fatal("expected error for invalid tenant id")
	}
}

func TestMigrate_IdempotentAndSeedsCatalog(t *testing.T) {
	dbs := dbtest.New(t)
	ctx := context.Background()

	applied, err := database.Migrate(ctx, dbs.Admin, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Errorf("second migrate should apply nothing, applied %v", applied)
	}

	if n, err := database.SeedCatalog(ctx, dbs.Admin); err != nil || n != 93 {
		t.Fatalf("reseed: n=%d err=%v", n, err)
	}
	var count int
	if err := dbs.Admin.QueryRowContext(ctx, `SELECT count(*) FROM control_catalog`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 93 {
		t.Errorf("expected 93 catalog controls, got %d", count)
	}
}

func TestCheckRLSEnforced(t *testing.T) {
	dbs := dbtest.New(t)
	ctx := context.Background()

	if err := database.CheckRLSEnforced(ctx, dbs.App); err != nil {
		t.Errorf("application role should be subject to RLS: %v", err)
	}

	var super bool
	if err := dbs.Admin.QueryRowContext(ctx,
		`SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&super); err != nil {
		t.Fatal(err)
	}
	if super {
		if err := database.CheckRLSEnforced(ctx, dbs.Admin); err == nil {
			t.Error("superuser connection should be reported as bypassing RLS")
		}
	}
}

// TestRLS_IsolatesTenants exercises the policies directly, without the
// WHERE tenant_id clauses the stores add: the database alone must refuse
// cross-tenant reads and writes.
func TestRLS_IsolatesTenants(t *testing.T) {
	dbs := dbtest.New(t)
	ctx := context.Background()
	dbtest.Tenant(t, dbs.Admin, tenantA, "Empresa A")
	dbtest.Tenant(t, dbs.Admin, tenantB, "Empresa B")

	insertRisk := func(tx *sql.Tx, tenantID, code string) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO risks (tenant_id, code, title) VALUES ($1, $2, 'riesgo')`, tenantID, code)
		return err
	}

	if err := database.TenantTx(ctx, dbs.App, tenantA, func(tx *sql.Tx) error {
		return insertRisk(tx, tenantA, "R-001")
	}); err != nil {
		t.Fatalf("tenant A insert: %v", err)
	}

	t.Run("other tenant sees and changes nothing", func(t *testing.T) {
		err := database.TenantTx(ctx, dbs.App, tenantB, func(tx *sql.Tx) error {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM risks`).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("tenant B sees %d risks of tenant A", n)
			}
			for _, stmt := range []string{`UPDATE risks SET title = 'hijacked'`, `DELETE FROM risks`} {
				res, err := tx.ExecContext(ctx, stmt)
				if err != nil {
					return err
				}
				if rows, _ := res.RowsAffected(); rows != 0 {
					t.Errorf("%q affected %d rows of another tenant", stmt, rows)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cannot write rows for another tenant", func(t *testing.T) {
		err := database.TenantTx(ctx, dbs.App, tenantB, func(tx *sql.Tx) error {
			return insertRisk(tx, tenantA, "R-666")
		})
		if err == nil || !strings.Contains(err.Error(), "row-level security") {
			t.Errorf("expected row-level security violation, got %v", err)
		}
	})

	t.Run("no tenant context means no access", func(t *testing.T) {
		var n int
		err := dbs.App.QueryRowContext(ctx, `SELECT count(*) FROM risks`).Scan(&n)
		if err == nil && n != 0 {
			t.Errorf("query without tenant context returned %d rows", n)
		}
	})

	t.Run("tenant context does not leak across pooled connections", func(t *testing.T) {
		conn, err := dbs.App.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantA); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		var n int
		err = conn.QueryRowContext(ctx, `SELECT count(*) FROM risks`).Scan(&n)
		if err == nil && n != 0 {
			t.Errorf("same connection still sees %d rows after the tenant transaction ended", n)
		}
	})

	t.Run("owner still sees its row", func(t *testing.T) {
		err := database.TenantTx(ctx, dbs.App, tenantA, func(tx *sql.Tx) error {
			var title string
			if err := tx.QueryRowContext(ctx, `SELECT title FROM risks`).Scan(&title); err != nil {
				return err
			}
			if title != "riesgo" {
				t.Errorf("row was modified: %q", title)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestTenantTx_RollsBackOnError(t *testing.T) {
	dbs := dbtest.New(t)
	ctx := context.Background()
	dbtest.Tenant(t, dbs.Admin, tenantA, "Empresa A")

	boom := errors.New("boom")
	err := database.TenantTx(ctx, dbs.App, tenantA, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO risks (tenant_id, code, title) VALUES ($1, 'R-001', 'x')`, tenantA); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	var n int
	if err := dbs.Admin.QueryRowContext(ctx, `SELECT count(*) FROM risks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("insert should have been rolled back, found %d rows", n)
	}
}

func TestAppRole_LeastPrivilege(t *testing.T) {
	dbs := dbtest.New(t)
	ctx := context.Background()
	dbtest.Tenant(t, dbs.Admin, tenantA, "Empresa A")

	denied := []string{
		`UPDATE audit_log SET action = 'x'`,
		`DELETE FROM audit_log`,
		`DELETE FROM tenants`,
		`UPDATE tenants SET plan = 'enterprise'`,
		`DELETE FROM control_catalog`,
		`SELECT * FROM schema_migrations`,
	}
	for _, stmt := range denied {
		err := database.TenantTx(ctx, dbs.App, tenantA, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%q: expected permission denied, got %v", stmt, err)
		}
	}
}
