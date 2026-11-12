package directory_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database/dbtest"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/directory"
)

var ctx = context.Background()

var (
	ana  = auth.Identity{Subject: "sub-ana", Email: "ana@empresa.co", EmailVerified: true, Name: "Ana Pérez"}
	luis = auth.Identity{Subject: "sub-luis", Email: "luis@empresa.co", EmailVerified: true, Name: "Luis Gómez"}
)

// eachStore runs the same contract against the in-memory store and, when
// TEST_DATABASE_URL is set, against PostgreSQL through the RLS-restricted
// application role.
func eachStore(t *testing.T, test func(t *testing.T, store directory.Store)) {
	t.Run("memory", func(t *testing.T) { test(t, directory.NewMemoryStore()) })
	t.Run("postgres", func(t *testing.T) {
		dbs := dbtest.New(t) // skips without TEST_DATABASE_URL
		test(t, directory.NewPgStore(dbs.App))
	})
}

func mustOrg(t *testing.T, store directory.Store, name string, owner auth.Identity) *auth.Membership {
	t.Helper()
	m, err := store.CreateOrganization(ctx, name, owner)
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	return m
}

func TestStore_CreateOrganization(t *testing.T) {
	eachStore(t, func(t *testing.T, store directory.Store) {
		m := mustOrg(t, store, "Ferretería El Tornillo", ana)
		if m.Role != auth.RoleOwner || m.Email != "ana@empresa.co" || m.TenantName != "Ferretería El Tornillo" || m.UserID == "" {
			t.Fatalf("unexpected membership: %+v", m)
		}

		got, err := store.Membership(ctx, m.TenantID, m.UserID)
		if err != nil || *got != *m {
			t.Errorf("Membership = %+v, %v; want %+v", got, err, m)
		}
		members, err := store.ListMembers(ctx, m.TenantID)
		if err != nil || len(members) != 1 || members[0].Status != "active" || members[0].Role != auth.RoleOwner {
			t.Errorf("ListMembers = %+v, %v", members, err)
		}

		if _, err := store.CreateOrganization(ctx, "Sin correo", auth.Identity{Subject: "s"}); err == nil {
			t.Error("an identity without e-mail cannot own an organization")
		}
	})
}

func TestStore_InvitationFlow(t *testing.T) {
	eachStore(t, func(t *testing.T, store directory.Store) {
		org := mustOrg(t, store, "Empresa A", ana)

		invited, err := store.Invite(ctx, org.TenantID, "luis@empresa.co", "", auth.RoleAuditor)
		if err != nil || invited.Status != "invited" || invited.Role != auth.RoleAuditor {
			t.Fatalf("Invite = %+v, %v", invited, err)
		}
		if _, err := store.Invite(ctx, org.TenantID, "luis@empresa.co", "", auth.RoleViewer); !errors.Is(err, directory.ErrConflict) {
			t.Errorf("duplicate invitation: expected ErrConflict, got %v", err)
		}

		// Not accepted without a verified e-mail…
		unverified := luis
		unverified.Subject, unverified.EmailVerified = "sub-impostor", false
		if ms, err := store.Resolve(ctx, unverified); err != nil || len(ms) != 0 {
			t.Fatalf("unverified e-mail resolved memberships: %+v, %v", ms, err)
		}
		// …accepted with one, and the provider's name fills the blank.
		ms, err := store.Resolve(ctx, luis)
		if err != nil || len(ms) != 1 || ms[0].UserID != invited.ID || ms[0].Role != auth.RoleAuditor || ms[0].Name != "Luis Gómez" {
			t.Fatalf("Resolve = %+v, %v", ms, err)
		}
		members, _ := store.ListMembers(ctx, org.TenantID)
		if len(members) != 2 || members[1].Status != "active" || members[1].LastLogin == nil {
			t.Errorf("invitation should now be active: %+v", members)
		}

		// Bound to the subject from now on.
		renamed := luis
		renamed.Email = "nuevo@otro.co"
		if ms, _ := store.Resolve(ctx, renamed); len(ms) != 1 {
			t.Errorf("bound member not found after e-mail change: %+v", ms)
		}
		squatter := luis
		squatter.Subject = "sub-squatter"
		if ms, _ := store.Resolve(ctx, squatter); len(ms) != 0 {
			t.Errorf("accepted invitation reused by another identity: %+v", ms)
		}
	})
}

func TestStore_ResolveAcrossOrganizations(t *testing.T) {
	eachStore(t, func(t *testing.T, store directory.Store) {
		zeta := mustOrg(t, store, "Zeta Ltda", ana)
		alfa := mustOrg(t, store, "Alfa SAS", ana)
		mustOrg(t, store, "Ajena", luis)

		ms, err := store.Resolve(ctx, ana)
		if err != nil || len(ms) != 2 || ms[0].TenantID != alfa.TenantID || ms[1].TenantID != zeta.TenantID {
			t.Errorf("expected Alfa then Zeta, got %+v, %v", ms, err)
		}
		if ms, _ := store.Resolve(ctx, auth.Identity{}); len(ms) != 0 {
			t.Errorf("an empty identity must resolve nothing: %+v", ms)
		}
	})
}

func TestStore_LastOwnerIsProtected(t *testing.T) {
	eachStore(t, func(t *testing.T, store directory.Store) {
		org := mustOrg(t, store, "Empresa A", ana)

		if _, err := store.SetRole(ctx, org.TenantID, org.UserID, auth.RoleAdmin); !errors.Is(err, directory.ErrLastOwner) {
			t.Errorf("demoting the only owner: expected ErrLastOwner, got %v", err)
		}
		if err := store.Remove(ctx, org.TenantID, org.UserID); !errors.Is(err, directory.ErrLastOwner) {
			t.Errorf("removing the only owner: expected ErrLastOwner, got %v", err)
		}

		second, err := store.Invite(ctx, org.TenantID, "luis@empresa.co", "Luis", auth.RoleOwner)
		if err != nil {
			t.Fatal(err)
		}
		if m, err := store.SetRole(ctx, org.TenantID, org.UserID, auth.RoleAdmin); err != nil || m.Role != auth.RoleAdmin {
			t.Errorf("demotion with a second owner: %+v, %v", m, err)
		}
		if err := store.Remove(ctx, org.TenantID, second.ID); !errors.Is(err, directory.ErrLastOwner) {
			t.Errorf("removing the remaining owner: expected ErrLastOwner, got %v", err)
		}
	})
}

func TestStore_RemoveEndsMembership(t *testing.T) {
	eachStore(t, func(t *testing.T, store directory.Store) {
		org := mustOrg(t, store, "Empresa A", ana)
		invited, _ := store.Invite(ctx, org.TenantID, "luis@empresa.co", "", auth.RoleEditor)
		store.Resolve(ctx, luis)

		if err := store.Remove(ctx, org.TenantID, invited.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Membership(ctx, org.TenantID, invited.ID); !errors.Is(err, auth.ErrNoMembership) {
			t.Errorf("expected ErrNoMembership, got %v", err)
		}
		if ms, _ := store.Resolve(ctx, luis); len(ms) != 0 {
			t.Errorf("removed member still resolves: %+v", ms)
		}
		if err := store.Remove(ctx, org.TenantID, invited.ID); !errors.Is(err, directory.ErrNotFound) {
			t.Errorf("second removal: expected ErrNotFound, got %v", err)
		}
		for _, id := range []string{"not-a-uuid", "99999999-9999-4999-8999-999999999999"} {
			if _, err := store.SetRole(ctx, org.TenantID, id, auth.RoleViewer); !errors.Is(err, directory.ErrNotFound) {
				t.Errorf("SetRole(%s): expected ErrNotFound, got %v", id, err)
			}
			if _, err := store.Membership(ctx, org.TenantID, id); !errors.Is(err, auth.ErrNoMembership) {
				t.Errorf("Membership(%s): expected ErrNoMembership, got %v", id, err)
			}
		}
	})
}

func TestStore_OrganizationsAreIsolated(t *testing.T) {
	eachStore(t, func(t *testing.T, store directory.Store) {
		a := mustOrg(t, store, "Empresa A", ana)
		b := mustOrg(t, store, "Empresa B", luis)

		// Acting inside B, A's owner cannot be read, changed or removed.
		if _, err := store.Membership(ctx, b.TenantID, a.UserID); !errors.Is(err, auth.ErrNoMembership) {
			t.Errorf("cross-organization Membership: got %v", err)
		}
		if _, err := store.SetRole(ctx, b.TenantID, a.UserID, auth.RoleViewer); !errors.Is(err, directory.ErrNotFound) {
			t.Errorf("cross-organization SetRole: got %v", err)
		}
		if err := store.Remove(ctx, b.TenantID, a.UserID); !errors.Is(err, directory.ErrNotFound) {
			t.Errorf("cross-organization Remove: got %v", err)
		}
		members, _ := store.ListMembers(ctx, b.TenantID)
		if len(members) != 1 || members[0].Email != "luis@empresa.co" {
			t.Errorf("B lists members of another organization: %+v", members)
		}
		// The same address can be invited to both, independently.
		if _, err := store.Invite(ctx, a.TenantID, "comun@x.co", "", auth.RoleViewer); err != nil {
			t.Error(err)
		}
		if _, err := store.Invite(ctx, b.TenantID, "comun@x.co", "", auth.RoleAdmin); err != nil {
			t.Error(err)
		}
		if got, _ := store.Membership(ctx, a.TenantID, a.UserID); got == nil || got.Role != auth.RoleOwner {
			t.Errorf("A's owner was affected: %+v", got)
		}
	})
}

func TestStore_EnsureDevUser(t *testing.T) {
	eachStore(t, func(t *testing.T, store directory.Store) {
		const tenantID = "11111111-1111-4111-8111-111111111111"

		first, err := store.EnsureDevUser(ctx, tenantID, "Dev@Local.test", "Dev", "")
		if err != nil || first.Role != auth.RoleOwner || first.Email != "dev@local.test" || first.TenantName == "" {
			t.Fatalf("first dev user: %+v, %v", first, err)
		}
		second, _ := store.EnsureDevUser(ctx, tenantID, "otra@local.test", "Otra", "")
		if second.Role != auth.RoleAdmin {
			t.Errorf("second dev user should default to admin: %+v", second)
		}
		// Same person again: same record; role kept unless one is given.
		again, _ := store.EnsureDevUser(ctx, tenantID, "dev@local.test", "Dev", "")
		if again.UserID != first.UserID || again.Role != auth.RoleOwner {
			t.Errorf("repeat login changed the member: %+v", again)
		}
		changed, _ := store.EnsureDevUser(ctx, tenantID, "otra@local.test", "Otra", auth.RoleViewer)
		if changed.UserID != second.UserID || changed.Role != auth.RoleViewer {
			t.Errorf("explicit role not applied: %+v", changed)
		}
		if _, err := store.EnsureDevUser(ctx, tenantID, "not-an-email", "", ""); err == nil {
			t.Error("invalid e-mail accepted")
		}
	})
}

// ── PostgreSQL only ──

// The sign-in lookup is the only query that crosses organizations. These
// checks hit the policy directly, without the store's own WHERE clause.
func TestPg_SignInLookupPolicy(t *testing.T) {
	dbs := dbtest.New(t)
	store := directory.NewPgStore(dbs.App)
	a := mustOrg(t, store, "Empresa A", ana)
	mustOrg(t, store, "Empresa B", luis)
	store.Invite(ctx, a.TenantID, "invitada@x.co", "", auth.RoleViewer)

	visible := func(sub, email string) (emails []string) {
		t.Helper()
		tx, err := dbs.App.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx,
			`SELECT set_config('app.auth_sub', $1, true), set_config('app.auth_email', $2, true)`, sub, email); err != nil {
			t.Fatal(err)
		}
		rows, err := tx.QueryContext(ctx, `SELECT email FROM users ORDER BY email`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var e string
			rows.Scan(&e)
			emails = append(emails, e)
		}
		// The lookup is read-only: it must not allow changing what it sees.
		for _, stmt := range []string{`UPDATE users SET role = 'owner'`, `DELETE FROM users`} {
			res, err := tx.ExecContext(ctx, stmt)
			if err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
			if n, _ := res.RowsAffected(); n != 0 {
				t.Errorf("%q through the sign-in lookup changed %d rows", stmt, n)
			}
		}
		return emails
	}

	for name, tc := range map[string]struct {
		sub, email string
		want       []string
	}{
		"no identity":                    {"", "", nil},
		"subject sees own rows only":     {"sub-ana", "", []string{"ana@empresa.co"}},
		"unknown subject":                {"sub-nadie", "", nil},
		"e-mail sees pending invitation": {"", "invitada@x.co", []string{"invitada@x.co"}},
		// A bound member is never exposed by e-mail alone.
		"e-mail of a bound member": {"", "ana@empresa.co", nil},
	} {
		got := visible(tc.sub, tc.email)
		if len(got) != len(tc.want) || (len(got) > 0 && got[0] != tc.want[0]) {
			t.Errorf("%s: visible = %v, want %v", name, got, tc.want)
		}
	}
}

func TestPg_MemberChangesAreAudited(t *testing.T) {
	dbs := dbtest.New(t)
	store := directory.NewPgStore(dbs.App)
	org := mustOrg(t, store, "Empresa A", ana)
	asOwner := auth.WithUser(ctx, &auth.User{Email: "ana@empresa.co", Role: auth.RoleOwner})

	invited, _ := store.Invite(asOwner, org.TenantID, "luis@empresa.co", "", auth.RoleViewer)
	store.Resolve(ctx, luis)
	store.SetRole(asOwner, org.TenantID, invited.ID, auth.RoleEditor)
	store.Remove(asOwner, org.TenantID, invited.ID)
	// Refused operations leave no trace.
	store.Remove(asOwner, org.TenantID, org.UserID)

	rows, err := dbs.Admin.Query(`SELECT action, detail->>'actor' FROM audit_log WHERE tenant_id = $1 ORDER BY id`, org.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var action, who string
		rows.Scan(&action, &who)
		got = append(got, action+" by "+who)
	}
	want := []string{
		"organization.create by ana@empresa.co",
		"member.invite by ana@empresa.co",
		"member.join by luis@empresa.co",
		"member.role_change by ana@empresa.co",
		"member.remove by ana@empresa.co",
	}
	if len(got) != len(want) {
		t.Fatalf("audit trail = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("audit[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Two owners demoting each other at the same time must not leave the
// organization without an owner.
func TestPg_ConcurrentDemotionsKeepOneOwner(t *testing.T) {
	dbs := dbtest.New(t)
	store := directory.NewPgStore(dbs.App)

	for round := 0; round < 5; round++ {
		org := mustOrg(t, store, "Empresa", ana)
		second, err := store.Invite(ctx, org.TenantID, "luis@empresa.co", "", auth.RoleOwner)
		if err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, id := range []string{org.UserID, second.ID} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = store.SetRole(ctx, org.TenantID, id, auth.RoleAdmin)
			}()
		}
		wg.Wait()

		refused := 0
		for _, err := range errs {
			if errors.Is(err, directory.ErrLastOwner) {
				refused++
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		var owners int
		err = database.TenantTx(ctx, dbs.App, org.TenantID, func(tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role = 'owner'`).Scan(&owners)
		})
		if err != nil {
			t.Fatal(err)
		}
		if owners != 1 || refused != 1 {
			t.Fatalf("round %d: %d owners left, %d demotions refused (want 1 and 1)", round, owners, refused)
		}
	}
}
