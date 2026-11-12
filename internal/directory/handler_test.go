package directory_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/directory"
)

// fixture is an organization with one member per role.
type fixture struct {
	t       *testing.T
	mux     *http.ServeMux
	store   *directory.MemoryStore
	tenant  string
	members map[string]*directory.Member // by role
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := directory.NewMemoryStore()
	org := mustOrg(t, store, "Empresa A", ana)
	f := &fixture{t: t, mux: http.NewServeMux(), store: store, tenant: org.TenantID,
		members: map[string]*directory.Member{auth.RoleOwner: {ID: org.UserID, Email: org.Email, Role: auth.RoleOwner}}}
	for _, role := range []string{auth.RoleAdmin, auth.RoleEditor, auth.RoleAuditor, auth.RoleViewer} {
		m, err := store.Invite(ctx, org.TenantID, role+"@empresa.co", "", role)
		if err != nil {
			t.Fatal(err)
		}
		f.members[role] = m
	}
	directory.NewHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(f.mux)
	return f
}

// as performs a request as the member registered under key (a role name).
func (f *fixture) as(key, method, path, body string) (int, string) {
	f.t.Helper()
	m := f.members[key]
	role := m.Role
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(auth.WithUser(req.Context(),
		&auth.User{ID: m.ID, Email: m.Email, TenantID: f.tenant, Role: role}))
	rr := httptest.NewRecorder()
	f.mux.ServeHTTP(rr, req)
	return rr.Code, rr.Body.String()
}

func TestHandler_WhoMayListMembers(t *testing.T) {
	f := newFixture(t)
	for role, want := range map[string]int{
		auth.RoleOwner: 200, auth.RoleAdmin: 200, auth.RoleAuditor: 200,
		auth.RoleEditor: 403, auth.RoleViewer: 403,
	} {
		if status, _ := f.as(role, http.MethodGet, "/v1/users", ""); status != want {
			t.Errorf("%s listing members: got %d, want %d", role, status, want)
		}
	}
	_, body := f.as(auth.RoleOwner, http.MethodGet, "/v1/users", "")
	var list struct {
		Count int
		Data  []directory.Member
	}
	json.Unmarshal([]byte(body), &list)
	if list.Count != 5 || list.Data[0].Role != auth.RoleOwner || list.Data[0].Status != "active" || list.Data[1].Status != "invited" {
		t.Errorf("unexpected list: %+v", list)
	}
}

func TestHandler_OnlyOwnersAndAdminsManageMembers(t *testing.T) {
	f := newFixture(t)
	target := f.members[auth.RoleViewer].ID
	for _, role := range []string{auth.RoleEditor, auth.RoleAuditor, auth.RoleViewer} {
		for _, req := range [][3]string{
			{http.MethodPost, "/v1/users", `{"email":"x@y.co","role":"viewer"}`},
			{http.MethodPut, "/v1/users/" + f.members[auth.RoleAdmin].ID, `{"role":"viewer"}`},
			{http.MethodDelete, "/v1/users/" + f.members[auth.RoleAdmin].ID, ""},
		} {
			if status, _ := f.as(role, req[0], req[1], req[2]); status != http.StatusForbidden {
				t.Errorf("%s %s %s: expected 403, got %d", role, req[0], req[1], status)
			}
		}
	}

	if status, body := f.as(auth.RoleAdmin, http.MethodPost, "/v1/users", `{"email":" Nueva@Empresa.co ","name":"Nueva","role":"editor"}`); status != http.StatusCreated || !strings.Contains(body, `"email":"nueva@empresa.co"`) || !strings.Contains(body, `"status":"invited"`) {
		t.Errorf("admin invite: %d %s", status, body)
	}
	if status, body := f.as(auth.RoleAdmin, http.MethodPut, "/v1/users/"+target, `{"role":"auditor"}`); status != http.StatusOK || !strings.Contains(body, `"role":"auditor"`) {
		t.Errorf("admin role change: %d %s", status, body)
	}
	if status, _ := f.as(auth.RoleAdmin, http.MethodDelete, "/v1/users/"+target, ""); status != http.StatusNoContent {
		t.Errorf("admin removal: expected 204, got %d", status)
	}
	if status, _ := f.as(auth.RoleAdmin, http.MethodDelete, "/v1/users/"+target, ""); status != http.StatusNotFound {
		t.Errorf("removing twice: expected 404, got %d", status)
	}
}

func TestHandler_OwnerRoleIsReservedToOwners(t *testing.T) {
	f := newFixture(t)
	owner := f.members[auth.RoleOwner].ID
	editor := f.members[auth.RoleEditor].ID

	// An administrator cannot create owners, promote to owner, or touch an owner.
	for _, req := range [][3]string{
		{http.MethodPost, "/v1/users", `{"email":"x@y.co","role":"owner"}`},
		{http.MethodPut, "/v1/users/" + editor, `{"role":"owner"}`},
		{http.MethodPut, "/v1/users/" + owner, `{"role":"viewer"}`},
		{http.MethodDelete, "/v1/users/" + owner, ""},
	} {
		if status, _ := f.as(auth.RoleAdmin, req[0], req[1], req[2]); status != http.StatusForbidden {
			t.Errorf("admin %s %s %s: expected 403, got %d", req[0], req[1], req[2], status)
		}
	}
	if m, _ := f.store.Membership(ctx, f.tenant, owner); m == nil || m.Role != auth.RoleOwner {
		t.Fatalf("owner was altered: %+v", m)
	}

	// An owner can.
	if status, _ := f.as(auth.RoleOwner, http.MethodPut, "/v1/users/"+editor, `{"role":"owner"}`); status != http.StatusOK {
		t.Errorf("owner promoting to owner: expected 200, got %d", status)
	}
}

func TestHandler_NobodyChangesTheirOwnMembership(t *testing.T) {
	f := newFixture(t)
	for _, role := range []string{auth.RoleOwner, auth.RoleAdmin} {
		self := f.members[role].ID
		if status, _ := f.as(role, http.MethodPut, "/v1/users/"+self, `{"role":"viewer"}`); status != http.StatusForbidden {
			t.Errorf("%s changing own role: expected 403, got %d", role, status)
		}
		if status, _ := f.as(role, http.MethodDelete, "/v1/users/"+self, ""); status != http.StatusForbidden {
			t.Errorf("%s removing self: expected 403, got %d", role, status)
		}
	}
}

func TestHandler_LastOwnerAndValidation(t *testing.T) {
	f := newFixture(t)
	// Promote the admin to owner, then let that second owner demote the
	// first: allowed. Demoting the remaining one is then impossible for
	// anyone (the only owner cannot act on themselves either).
	admin := f.members[auth.RoleAdmin]
	f.as(auth.RoleOwner, http.MethodPut, "/v1/users/"+admin.ID, `{"role":"owner"}`)
	promoted := *admin
	promoted.Role = auth.RoleOwner
	f.members["owner2"] = &promoted
	if status, _ := f.as("owner2", http.MethodDelete, "/v1/users/"+f.members[auth.RoleOwner].ID, ""); status != http.StatusNoContent {
		t.Fatalf("second owner removing the first: expected 204, got %d", status)
	}
	// Store-level guard, reached through an admin-less path: a direct call.
	if err := f.store.Remove(ctx, f.tenant, admin.ID); err == nil {
		t.Error("the last owner was removed")
	}

	for name, body := range map[string]string{
		"bad email":    `{"email":"no-arroba","role":"viewer"}`,
		"unknown role": `{"email":"x@y.co","role":"root"}`,
		"long name":    `{"email":"x@y.co","name":"` + strings.Repeat("n", 201) + `"}`,
	} {
		if status, _ := f.as("owner2", http.MethodPost, "/v1/users", body); status != http.StatusUnprocessableEntity {
			t.Errorf("%s: expected 422, got %d", name, status)
		}
	}
	if status, _ := f.as("owner2", http.MethodPost, "/v1/users", `{`); status != http.StatusBadRequest {
		t.Errorf("invalid JSON: expected 400, got %d", status)
	}
	if status, _ := f.as("owner2", http.MethodPost, "/v1/users", `{"email":"editor@empresa.co"}`); status != http.StatusConflict {
		t.Errorf("duplicate e-mail: expected 409, got %d", status)
	}
	// Role defaults to the least privileged one.
	if status, body := f.as("owner2", http.MethodPost, "/v1/users", `{"email":"nuevo@y.co"}`); status != http.StatusCreated || !strings.Contains(body, `"role":"viewer"`) {
		t.Errorf("default role: %d %s", status, body)
	}
	if status, _ := f.as("owner2", http.MethodPut, "/v1/users/"+f.members[auth.RoleViewer].ID, `{"role":"root"}`); status != http.StatusUnprocessableEntity {
		t.Errorf("unknown role on change: expected 422, got %d", status)
	}
}
