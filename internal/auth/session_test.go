package auth

import (
	"context"
	"testing"
	"time"
)

func TestFromContext_Nil(t *testing.T) {
	if u := FromContext(context.Background()); u != nil {
		t.Error("expected nil user from empty context")
	}
}

func TestWithUser_Roundtrip(t *testing.T) {
	ctx := WithUser(context.Background(), &User{ID: "1", Role: RoleAdmin})
	if got := FromContext(ctx); got == nil || got.ID != "1" {
		t.Errorf("expected user ID '1', got %v", got)
	}
}

func TestUser_Permissions(t *testing.T) {
	cases := map[string][3]bool{ // write, manage members, view audit
		RoleOwner:   {true, true, true},
		RoleAdmin:   {true, true, true},
		RoleEditor:  {true, false, false},
		RoleAuditor: {false, false, true},
		RoleViewer:  {false, false, false},
		"":          {false, false, false},
	}
	for role, want := range cases {
		u := &User{Role: role}
		if got := [3]bool{u.CanWrite(), u.CanManageMembers(), u.CanViewAudit()}; got != want {
			t.Errorf("role %q: got %v, want %v", role, got, want)
		}
	}
	var nobody *User
	if nobody.CanWrite() || nobody.CanManageMembers() || nobody.CanViewAudit() {
		t.Error("a nil user must have no permissions")
	}
}

func TestSessionStore_Lifecycle(t *testing.T) {
	store := NewSessionStore(time.Hour)
	id := Identity{Subject: "sub-1", Email: "a@b.com"}

	token, err := store.Create(id, nil)
	if err != nil || len(token) != 64 {
		t.Fatalf("Create: token=%q err=%v", token, err)
	}
	session := store.Get(token)
	if session == nil || session.Identity.Email != "a@b.com" || session.TenantID != "" {
		t.Fatalf("unexpected session: %+v", session)
	}

	store.SetMembership(token, &Membership{UserID: "u1", TenantID: "t1"})
	if s := store.Get(token); s.UserID != "u1" || s.TenantID != "t1" {
		t.Errorf("membership not stored: %+v", s)
	}
	// Get returns a copy: callers cannot alter the stored session.
	store.Get(token).TenantID = "tampered"
	if s := store.Get(token); s.TenantID != "t1" {
		t.Errorf("stored session was modified through a returned copy: %+v", s)
	}
	store.SetMembership(token, nil)
	if s := store.Get(token); s.TenantID != "" {
		t.Errorf("membership not cleared: %+v", s)
	}

	store.Delete(token)
	if store.Get(token) != nil {
		t.Error("expected deleted session to be nil")
	}
	if store.Get("") != nil || store.Get("unknown") != nil {
		t.Error("unknown tokens must not resolve to a session")
	}
}

func TestSessionStore_ExpiryAndPurge(t *testing.T) {
	store := NewSessionStore(time.Millisecond)
	first, _ := store.Create(Identity{Email: "1@x.co"}, nil)
	store.Create(Identity{Email: "2@x.co"}, nil)
	time.Sleep(5 * time.Millisecond)

	if store.Get(first) != nil {
		t.Error("expected expired session to be nil")
	}
	store.Create(Identity{Email: "3@x.co"}, nil)

	store.mu.RLock()
	n := len(store.sessions)
	store.mu.RUnlock()
	if n != 1 {
		t.Errorf("expected only the fresh session to remain, got %d", n)
	}
}

func TestNameFromEmail(t *testing.T) {
	for email, want := range map[string]string{
		"john.doe@example.com": "John doe",
		"admin@test.com":       "Admin",
		"":                     "",
	} {
		if got := nameFromEmail(email); got != want {
			t.Errorf("nameFromEmail(%q) = %q, want %q", email, got, want)
		}
	}
}

func TestAudience_AcceptsStringOrArray(t *testing.T) {
	var one, many audience
	if err := one.UnmarshalJSON([]byte(`"client"`)); err != nil || !one.contains("client") {
		t.Errorf("string audience: %v %v", one, err)
	}
	if err := many.UnmarshalJSON([]byte(`["a","client"]`)); err != nil || !many.contains("client") || len(many) != 2 {
		t.Errorf("array audience: %v %v", many, err)
	}
}
