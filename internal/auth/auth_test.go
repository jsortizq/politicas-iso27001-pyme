package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFromContext_Nil(t *testing.T) {
	u := FromContext(context.Background())
	if u != nil {
		t.Error("expected nil user from empty context")
	}
}

func TestWithUser_Roundtrip(t *testing.T) {
	user := &User{ID: "1", Email: "test@example.com", TenantID: "t1", Role: "admin"}
	ctx := WithUser(context.Background(), user)
	got := FromContext(ctx)
	if got == nil || got.ID != "1" {
		t.Errorf("expected user ID '1', got %v", got)
	}
}

func TestSessionStore_CreateAndGet(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	user := &User{ID: "u1", Email: "a@b.com", TenantID: "t1", Role: "admin"}

	token, err := store.Create(user)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	session := store.Get(token)
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.Email != "a@b.com" {
		t.Errorf("expected a@b.com, got %s", session.Email)
	}
}

func TestSessionStore_Expiry(t *testing.T) {
	store := NewSessionStore(1 * time.Millisecond)
	user := &User{ID: "u1", Email: "a@b.com", TenantID: "t1"}

	token, _ := store.Create(user)
	time.Sleep(5 * time.Millisecond)

	session := store.Get(token)
	if session != nil {
		t.Error("expected expired session to be nil")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	user := &User{ID: "u1", Email: "a@b.com", TenantID: "t1"}

	token, _ := store.Create(user)
	store.Delete(token)

	session := store.Get(token)
	if session != nil {
		t.Error("expected deleted session to be nil")
	}
}

func TestHandler_DevLogin(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	h := NewHandler(store, true)

	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"email":"admin@test.com","password":"pass","tenant_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp loginResponse
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Token == "" {
		t.Error("expected token in response")
	}
	if resp.User.Email != "admin@test.com" {
		t.Errorf("expected admin@test.com, got %s", resp.User.Email)
	}
}

func TestHandler_DevLogin_MissingFields(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	h := NewHandler(store, true)

	mux := http.NewServeMux()
	h.Register(mux)

	// Missing tenant_id
	body := `{"email":"admin@test.com"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandler_NonDevLogin(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	h := NewHandler(store, false) // not dev mode

	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"email":"admin@test.com","tenant_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rr.Code)
	}
}

func TestHandler_SessionEndpoint(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	h := NewHandler(store, true)

	mux := http.NewServeMux()
	h.Register(mux)

	// Login first
	body := `{"email":"admin@test.com","password":"x","tenant_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var loginResp loginResponse
	json.NewDecoder(rr.Body).Decode(&loginResp)

	// Check session
	req = httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("session: expected 200, got %d", rr.Code)
	}
}

func TestHandler_Logout(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	h := NewHandler(store, true)

	mux := http.NewServeMux()
	h.Register(mux)

	// Login
	body := `{"email":"admin@test.com","password":"x","tenant_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var loginResp loginResponse
	json.NewDecoder(rr.Body).Decode(&loginResp)

	// Logout
	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("logout: expected 204, got %d", rr.Code)
	}

	// Session should be gone
	req = httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("session after logout: expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleware_Valid(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	user := &User{ID: "u1", Email: "a@b.com", TenantID: "t1", Role: "admin"}
	token, _ := store.Create(user)

	h := NewHandler(store, true)

	var capturedUser *User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := h.Middleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedUser == nil || capturedUser.ID != "u1" {
		t.Error("expected user in context")
	}
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)
	h := NewHandler(store, true)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	handler := h.Middleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestNameFromEmail(t *testing.T) {
	tests := []struct {
		email    string
		expected string
	}{
		{"john.doe@example.com", "John doe"},
		{"admin@test.com", "Admin"},
		{"", ""},
	}

	for _, tt := range tests {
		got := nameFromEmail(tt.email)
		if got != tt.expected {
			t.Errorf("nameFromEmail(%q) = %q, want %q", tt.email, got, tt.expected)
		}
	}
}

func devLogin(t *testing.T, mux http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestHandler_DevLogin_RejectsBadTenantAndRole(t *testing.T) {
	h := NewHandler(NewSessionStore(time.Hour), true)
	mux := http.NewServeMux()
	h.Register(mux)

	if rr := devLogin(t, mux, `{"email":"a@b.com","tenant_id":"t1"}`); rr.Code != http.StatusBadRequest {
		t.Errorf("non-UUID tenant: expected 400, got %d", rr.Code)
	}
	if rr := devLogin(t, mux, `{"email":"a@b.com","tenant_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","role":"root"}`); rr.Code != http.StatusBadRequest {
		t.Errorf("unknown role: expected 400, got %d", rr.Code)
	}
}

func TestHandler_DevLogin_SetsHttpOnlyCookieAndCookieAuthWorks(t *testing.T) {
	h := NewHandler(NewSessionStore(time.Hour), true)
	h.SetSecureCookies(true)
	mux := http.NewServeMux()
	h.Register(mux)

	rr := devLogin(t, mux, `{"email":"a@b.com","tenant_id":"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE","role":"viewer"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rr.Code, rr.Body.String())
	}
	var resp loginResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.User.TenantID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" || resp.User.Role != "viewer" {
		t.Errorf("unexpected user: %+v", resp.User)
	}

	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "sgsi_session" || !cookies[0].HttpOnly || !cookies[0].Secure ||
		cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected cookies: %+v", cookies)
	}

	// The cookie alone authenticates and pins the tenant, whatever the client sends.
	var gotTenant string
	protected := h.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = r.Header.Get("X-Tenant-ID")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/soa", nil)
	req.AddCookie(cookies[0])
	req.Header.Set("X-Tenant-ID", "ffffffff-ffff-ffff-ffff-ffffffffffff")
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || gotTenant != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("cookie auth: code=%d tenant=%q", rec.Code, gotTenant)
	}
}

func TestHandler_DevLogin_ProvisionsTenant(t *testing.T) {
	h := NewHandler(NewSessionStore(time.Hour), true)
	var provisioned string
	h.SetTenantProvisioner(func(_ context.Context, id string) error {
		provisioned = id
		return nil
	})
	mux := http.NewServeMux()
	h.Register(mux)

	devLogin(t, mux, `{"email":"a@b.com","tenant_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`)
	if provisioned != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("provisioner not called, got %q", provisioned)
	}
}

func TestUser_CanWrite(t *testing.T) {
	for role, want := range map[string]bool{"owner": true, "admin": true, "editor": true, "auditor": false, "viewer": false, "": false} {
		if got := (&User{Role: role}).CanWrite(); got != want {
			t.Errorf("CanWrite(%q) = %v, want %v", role, got, want)
		}
	}
	var nobody *User
	if nobody.CanWrite() {
		t.Error("nil user must not write")
	}
}

func TestSessionStore_PurgesExpiredOnCreate(t *testing.T) {
	store := NewSessionStore(time.Millisecond)
	store.Create(&User{ID: "1"})
	store.Create(&User{ID: "2"})
	time.Sleep(5 * time.Millisecond)
	store.Create(&User{ID: "3"})

	store.mu.RLock()
	n := len(store.sessions)
	store.mu.RUnlock()
	if n != 1 {
		t.Errorf("expected only the fresh session to remain, got %d", n)
	}
}
