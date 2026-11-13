package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/config"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/storage"
)

const (
	tenantA = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	tenantB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("AUTH_DEV_MODE", "true")
	t.Setenv("DATABASE_URL", "")
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(cfg, logger, nil, testStorage(t))
}

func testStorage(t *testing.T) storage.Store {
	t.Helper()
	store, err := storage.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// login opens a dev session and returns its bearer token.
func login(t *testing.T, s *Server, tenantID, role string) string {
	t.Helper()
	body := fmt.Sprintf(`{"email":"%s@empresa.test","password":"x","tenant_id":%q,"role":%q}`, role, tenantID, role)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil || resp.Token == "" {
		t.Fatalf("login: no token (%v)", err)
	}
	return resp.Token
}

func call(s *Server, token, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rr := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(rr, req)
	return rr
}

func TestHealthz(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /healthz: expected 200, got %d", rr.Code)
	}
}

func TestReadyz(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /readyz: expected 200, got %d", rr.Code)
	}
}

func TestRoot(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /: expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "sgsi-api") {
		t.Errorf("expected sgsi-api in body, got %s", body)
	}
}

func TestAPI_RequiresAuthentication(t *testing.T) {
	s := testServer(t)

	for _, path := range []string{"/api/v1/info", "/api/v1/soa", "/api/v1/assets", "/api/v1/risks"} {
		// A client-supplied tenant header must not grant access.
		rr := call(s, "", http.MethodGet, path, "", "X-Tenant-ID", tenantA)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without session: expected 401, got %d", path, rr.Code)
		}
	}
	if rr := call(s, "not-a-real-token", http.MethodGet, "/api/v1/info", ""); rr.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: expected 401, got %d", rr.Code)
	}
}

func TestAPI_TenantComesFromSessionNotHeader(t *testing.T) {
	s := testServer(t)
	token := login(t, s, tenantA, "admin")

	// The client tries to reach another organization by forging the header.
	rr := call(s, token, http.MethodGet, "/api/v1/info", "", "X-Tenant-ID", tenantB)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/info: expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, tenantA) || strings.Contains(body, tenantB) {
		t.Errorf("expected session tenant in body, got %s", body)
	}
}

func TestAPI_EndToEndIsolationBetweenSessions(t *testing.T) {
	s := testServer(t)
	a := login(t, s, tenantA, "admin")
	b := login(t, s, tenantB, "admin")

	rr := call(s, a, http.MethodPost, "/api/v1/risks", `{"title":"Ransomware","likelihood":4,"impact":5}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create risk: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var risk struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&risk)

	if rr := call(s, b, http.MethodGet, "/api/v1/risks/"+risk.ID, "", "X-Tenant-ID", tenantA); rr.Code != http.StatusNotFound {
		t.Errorf("tenant B reading tenant A's risk: expected 404, got %d", rr.Code)
	}
	if rr := call(s, b, http.MethodGet, "/api/v1/risks", ""); !strings.Contains(rr.Body.String(), `"count":0`) {
		t.Errorf("tenant B should list no risks, got %s", rr.Body.String())
	}
	if rr := call(s, a, http.MethodGet, "/api/v1/risks/"+risk.ID, ""); rr.Code != http.StatusOK {
		t.Errorf("tenant A reading its risk: expected 200, got %d", rr.Code)
	}
}

func TestAPI_ReadOnlyRolesCannotWrite(t *testing.T) {
	s := testServer(t)
	admin := login(t, s, tenantA, "admin")
	rr := call(s, admin, http.MethodPost, "/api/v1/assets", `{"name":"ERP","asset_type":"software"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("admin create: expected 201, got %d", rr.Code)
	}
	var asset struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&asset)

	for _, role := range []string{"viewer", "auditor"} {
		token := login(t, s, tenantA, role)
		if rr := call(s, token, http.MethodGet, "/api/v1/assets", ""); rr.Code != http.StatusOK {
			t.Errorf("%s read: expected 200, got %d", role, rr.Code)
		}
		writes := map[string][2]string{
			"POST":   {"/api/v1/assets", `{"name":"x","asset_type":"software"}`},
			"PUT":    {"/api/v1/assets/" + asset.ID, `{"name":"x","asset_type":"software"}`},
			"DELETE": {"/api/v1/assets/" + asset.ID, ""},
		}
		for method, req := range writes {
			if rr := call(s, token, method, req[0], req[1]); rr.Code != http.StatusForbidden {
				t.Errorf("%s %s %s: expected 403, got %d", role, method, req[0], rr.Code)
			}
		}
		if rr := call(s, token, http.MethodPost, "/api/v1/soa/initialize", ""); rr.Code != http.StatusForbidden {
			t.Errorf("%s initialize SoA: expected 403, got %d", role, rr.Code)
		}
	}

	editor := login(t, s, tenantA, "editor")
	if rr := call(s, editor, http.MethodDelete, "/api/v1/assets/"+asset.ID, ""); rr.Code != http.StatusNoContent {
		t.Errorf("editor delete: expected 204, got %d", rr.Code)
	}
}

func TestLogin_DisabledUnlessDevModeIsExplicit(t *testing.T) {
	t.Setenv("AUTH_DEV_MODE", "")
	t.Setenv("OIDC_ISSUER", "")
	s := New(config.Load(), slog.New(slog.NewTextHandler(io.Discard, nil)), nil, testStorage(t))

	rr := call(s, "", http.MethodPost, "/auth/login", fmt.Sprintf(`{"email":"a@b.com","tenant_id":%q}`, tenantA))
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("login without AUTH_DEV_MODE: expected 501, got %d", rr.Code)
	}
}

func TestCORS_Preflight(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/info", nil)
	rr := httptest.NewRecorder()

	s.http.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("OPTIONS: expected 204, got %d", rr.Code)
	}
	if h := rr.Header().Get("Access-Control-Allow-Methods"); h == "" {
		t.Error("expected CORS headers in preflight response")
	}
}

func TestRecoverMiddleware(t *testing.T) {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{cfg: cfg, logger: logger}

	// Handler that panics
	panicker := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := s.recoverMiddleware(panicker)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", rr.Code)
	}
}

func TestAPI_MembersAndAuditEndpointsAreWired(t *testing.T) {
	s := testServer(t)
	owner := login(t, s, tenantA, "owner")

	rr := call(s, owner, http.MethodPost, "/api/v1/users", `{"email":"nuevo@empresa.test","role":"editor"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("invite: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := call(s, owner, http.MethodGet, "/api/v1/users", ""); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"count":2`) {
		t.Errorf("list members: %d %s", rr.Code, rr.Body.String())
	}
	// Members of another organization are not visible.
	other := login(t, s, tenantB, "owner")
	if rr := call(s, other, http.MethodGet, "/api/v1/users", ""); !strings.Contains(rr.Body.String(), `"count":1`) {
		t.Errorf("tenant B sees foreign members: %s", rr.Body.String())
	}

	// Without a database the audit trail is not recorded, and says so.
	rr = call(s, owner, http.MethodGet, "/api/v1/audit-log", "")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"recorded":false`) {
		t.Errorf("audit log: %d %s", rr.Code, rr.Body.String())
	}
	editor := login(t, s, tenantA, "editor")
	if rr := call(s, editor, http.MethodGet, "/api/v1/audit-log", ""); rr.Code != http.StatusForbidden {
		t.Errorf("editor reading the audit trail: expected 403, got %d", rr.Code)
	}
}

func TestOrigin_CrossSiteWritesAreRejected(t *testing.T) {
	t.Setenv("APP_URL", "https://sgsi.example.com")
	t.Setenv("CORS_ORIGINS", "https://sgsi.example.com, http://localhost:3000")
	s := testServer(t)
	token := login(t, s, tenantA, "admin")
	body := `{"name":"ERP","asset_type":"software"}`

	if rr := call(s, token, http.MethodPost, "/api/v1/assets", body, "Origin", "https://evil.example"); rr.Code != http.StatusForbidden {
		t.Errorf("cross-site POST: expected 403, got %d", rr.Code)
	}
	if rr := call(s, "", http.MethodPost, "/auth/logout", "", "Origin", "https://evil.example"); rr.Code != http.StatusForbidden {
		t.Errorf("cross-site logout: expected 403, got %d", rr.Code)
	}
	for _, origin := range []string{"https://sgsi.example.com", "http://localhost:3000"} {
		if rr := call(s, token, http.MethodPost, "/api/v1/assets", strings.Replace(body, "ERP", origin, 1), "Origin", origin); rr.Code != http.StatusCreated {
			t.Errorf("POST from %s: expected 201, got %d", origin, rr.Code)
		}
	}
	// Reads and clients without an Origin header are unaffected.
	if rr := call(s, token, http.MethodGet, "/api/v1/assets", "", "Origin", "https://evil.example"); rr.Code != http.StatusOK {
		t.Errorf("cross-site GET: expected 200, got %d", rr.Code)
	}
	if rr := call(s, token, http.MethodPost, "/api/v1/assets", `{"name":"CLI","asset_type":"software"}`); rr.Code != http.StatusCreated {
		t.Errorf("POST without Origin: expected 201, got %d", rr.Code)
	}
}

func TestAPI_DocumentsAndEvidenceAreWired(t *testing.T) {
	s := testServer(t)
	editor := login(t, s, tenantA, "editor")

	multipartRequest := func(token, path string, fields map[string]string, content string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		for k, v := range fields {
			w.WriteField(k, v)
		}
		part, _ := w.CreateFormFile("file", "archivo.txt")
		part.Write([]byte(content))
		w.Close()
		req := httptest.NewRequest(http.MethodPost, path, &body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(rr, req)
		return rr
	}

	if rr := multipartRequest("", "/api/v1/evidence", nil, "x"); rr.Code != http.StatusUnauthorized {
		t.Errorf("upload without session: expected 401, got %d", rr.Code)
	}

	rr := multipartRequest(editor, "/api/v1/evidence", map[string]string{"title": "Captura", "controls": "A.8.13"}, "contenido")
	if rr.Code != http.StatusCreated {
		t.Fatalf("evidence upload: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var ev struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&ev)

	rr = multipartRequest(editor, "/api/v1/documents", map[string]string{"title": "Política", "doc_type": "policy"}, "v1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("document upload: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// A read-only member downloads but cannot upload; another organization sees nothing.
	viewer := login(t, s, tenantA, "viewer")
	if rr := call(s, viewer, http.MethodGet, "/api/v1/evidence/"+ev.ID+"/download", ""); rr.Code != http.StatusOK || rr.Body.String() != "contenido" {
		t.Errorf("viewer download: %d %q", rr.Code, rr.Body.String())
	}
	if rr := multipartRequest(viewer, "/api/v1/evidence", nil, "x"); rr.Code != http.StatusForbidden {
		t.Errorf("viewer upload: expected 403, got %d", rr.Code)
	}
	other := login(t, s, tenantB, "owner")
	if rr := call(s, other, http.MethodGet, "/api/v1/evidence/"+ev.ID+"/download", ""); rr.Code != http.StatusNotFound {
		t.Errorf("download from another organization: expected 404, got %d", rr.Code)
	}
	if rr := call(s, other, http.MethodGet, "/api/v1/documents", ""); !strings.Contains(rr.Body.String(), `"count":0`) {
		t.Errorf("another organization lists documents: %s", rr.Body.String())
	}
}
