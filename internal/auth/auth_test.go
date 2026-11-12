package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth/oidctest"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/directory"
)

const (
	appURL    = "http://app.test"
	devTenant = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

// env is a running relying party (our API's /auth endpoints plus one
// protected route), a fake identity provider, and the member directory.
type env struct {
	t      *testing.T
	rp     *httptest.Server
	idp    *oidctest.Provider
	dir    *directory.MemoryStore
	signup bool
}

type envOption func(*auth.Options)

func newEnv(t *testing.T, opts ...envOption) *env {
	t.Helper()
	e := &env{t: t, dir: directory.NewMemoryStore()}
	e.idp = oidctest.New("sgsi-web", "s3cret/with+odd chars")
	t.Cleanup(e.idp.Server.Close)

	var handler http.Handler
	e.rp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(e.rp.Close)

	options := auth.Options{
		OIDC: auth.OIDCConfig{
			Issuer:       e.idp.Issuer(),
			ClientID:     e.idp.ClientID,
			ClientSecret: e.idp.ClientSecret,
			RedirectURL:  e.rp.URL + "/auth/callback",
		},
		AppURL:      appURL,
		AllowSignup: true,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, opt := range opts {
		opt(&options)
	}
	h := auth.NewHandler(auth.NewSessionStore(time.Hour), e.dir, options)

	mux := http.NewServeMux()
	h.Register(mux)
	// A protected route that reports who the middleware authenticated.
	mux.Handle("GET /api/whoami", h.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := auth.FromContext(r.Context())
		json.NewEncoder(w).Encode(map[string]string{
			"email": u.Email, "role": u.Role, "tenant": u.TenantID,
			"tenant_header": r.Header.Get("X-Tenant-ID"),
		})
	})))
	handler = mux
	return e
}

// browser is an HTTP client with its own cookie jar. It follows redirects
// between the relying party and the provider and stops when it is sent to
// the web application.
type browser struct {
	t      *testing.T
	e      *env
	client *http.Client
}

func (e *env) browser() *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{t: e.t, e: e, client: &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Host == "app.test" {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}}
}

// signIn runs the whole single sign-on flow and returns where the browser
// was finally sent (a path of the web application).
func (b *browser) signIn(person oidctest.Person) string {
	b.t.Helper()
	b.e.idp.SignInAs(person)
	resp, err := b.client.Get(b.e.rp.URL + "/auth/oidc/login")
	if err != nil {
		b.t.Fatalf("sign-in: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		b.t.Fatalf("sign-in: expected a redirect to the app, got %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, appURL) {
		b.t.Fatalf("sign-in: redirected outside the app: %s", location)
	}
	return strings.TrimPrefix(location, appURL)
}

func (b *browser) do(method, path, body string) (int, map[string]any) {
	b.t.Helper()
	req, _ := http.NewRequest(method, b.e.rp.URL+path, strings.NewReader(body))
	resp, err := b.client.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (b *browser) hasSession() bool {
	status, _ := b.do(http.MethodGet, "/auth/session", "")
	return status == http.StatusOK
}

var ana = oidctest.Person{Subject: "sub-ana", Email: "Ana@Empresa.co", EmailVerified: true, Name: "Ana Pérez"}

// ── Single sign-on ──

func TestSSO_NewPersonCreatesOrganization(t *testing.T) {
	e := newEnv(t)
	b := e.browser()

	if dest := b.signIn(ana); dest != "/onboarding" {
		t.Fatalf("a person without organizations should go to onboarding, went to %s", dest)
	}

	status, session := b.do(http.MethodGet, "/auth/session", "")
	if status != http.StatusOK || session["needs_organization"] != true || session["user"] != nil {
		t.Fatalf("unexpected session: %d %v", status, session)
	}
	if email := session["identity"].(map[string]any)["email"]; email != "ana@empresa.co" {
		t.Errorf("identity e-mail should be normalised, got %v", email)
	}

	// Without an organization the API is closed, with a code the UI can act on.
	status, body := b.do(http.MethodGet, "/api/whoami", "")
	if status != http.StatusForbidden || body["code"] != "no_organization" {
		t.Errorf("API without organization: %d %v", status, body)
	}

	status, session = b.do(http.MethodPost, "/auth/organizations", `{"name":"Ferretería El Tornillo"}`)
	if status != http.StatusCreated || session["needs_organization"] != false {
		t.Fatalf("create organization: %d %v", status, session)
	}
	user := session["user"].(map[string]any)
	if user["role"] != "owner" || user["email"] != "ana@empresa.co" {
		t.Errorf("creator should be the owner: %v", user)
	}

	status, who := b.do(http.MethodGet, "/api/whoami", "")
	if status != http.StatusOK || who["role"] != "owner" || who["tenant"] != user["tenant_id"] || who["tenant_header"] != user["tenant_id"] {
		t.Errorf("whoami: %d %v", status, who)
	}

	// Signing in again (another browser) lands directly in the organization.
	if dest := e.browser().signIn(ana); dest != "/dashboard" {
		t.Errorf("returning member should go to the dashboard, went to %s", dest)
	}
}

func TestSSO_InvitationIsAcceptedOnlyWithVerifiedEmail(t *testing.T) {
	e := newEnv(t)
	owner := e.browser()
	owner.signIn(ana)
	_, session := owner.do(http.MethodPost, "/auth/organizations", `{"name":"Empresa A"}`)
	tenantID := session["user"].(map[string]any)["tenant_id"].(string)
	if _, err := e.dir.Invite(context.Background(), tenantID, "luis@empresa.co", "", auth.RoleAuditor); err != nil {
		t.Fatal(err)
	}

	// Someone whose provider does not vouch for the address must not get in.
	impostor := e.browser()
	if dest := impostor.signIn(oidctest.Person{Subject: "sub-impostor", Email: "luis@empresa.co", EmailVerified: false, Name: "X"}); dest != "/onboarding" {
		t.Fatalf("unverified e-mail must not accept an invitation, went to %s", dest)
	}
	if status, _ := impostor.do(http.MethodGet, "/api/whoami", ""); status != http.StatusForbidden {
		t.Errorf("impostor reached the API: %d", status)
	}

	luis := e.browser()
	if dest := luis.signIn(oidctest.Person{Subject: "sub-luis", Email: "luis@empresa.co", EmailVerified: true, Name: "Luis Gómez"}); dest != "/dashboard" {
		t.Fatalf("invited member should go to the dashboard, went to %s", dest)
	}
	status, who := luis.do(http.MethodGet, "/api/whoami", "")
	if status != http.StatusOK || who["role"] != "auditor" || who["tenant"] != tenantID {
		t.Errorf("invited member: %d %v", status, who)
	}

	// The membership is now bound to the subject: it survives an e-mail
	// change at the provider, and the old address no longer opens it.
	moved := e.browser()
	if dest := moved.signIn(oidctest.Person{Subject: "sub-luis", Email: "luis.gomez@otro.co", EmailVerified: true}); dest != "/dashboard" {
		t.Errorf("bound member with a new e-mail should still get in, went to %s", dest)
	}
	squatter := e.browser()
	if dest := squatter.signIn(oidctest.Person{Subject: "sub-squatter", Email: "luis@empresa.co", EmailVerified: true}); dest != "/onboarding" {
		t.Errorf("an accepted invitation must not be reusable by another identity, went to %s", dest)
	}
}

func TestSSO_RejectsInvalidIDTokens(t *testing.T) {
	cases := map[string]func(p *oidctest.Provider){
		"wrong issuer": func(p *oidctest.Provider) {
			p.MutateClaims(func(c map[string]any) { c["iss"] = "https://evil.example" })
		},
		"other audience": func(p *oidctest.Provider) { p.MutateClaims(func(c map[string]any) { c["aud"] = "another-client" }) },
		"extra audience": func(p *oidctest.Provider) {
			p.MutateClaims(func(c map[string]any) { c["aud"] = []string{"sgsi-web", "other"} })
		},
		"expired": func(p *oidctest.Provider) {
			p.MutateClaims(func(c map[string]any) { c["exp"] = time.Now().Add(-10 * time.Minute).Unix() })
		},
		"issued in future": func(p *oidctest.Provider) {
			p.MutateClaims(func(c map[string]any) { c["iat"] = time.Now().Add(10 * time.Minute).Unix() })
		},
		"wrong nonce":         func(p *oidctest.Provider) { p.MutateClaims(func(c map[string]any) { c["nonce"] = "replayed" }) },
		"no subject":          func(p *oidctest.Provider) { p.MutateClaims(func(c map[string]any) { c["sub"] = "" }) },
		"unsigned (alg none)": func(p *oidctest.Provider) { p.UseAlgorithm("none") },
		"HS256 header":        func(p *oidctest.Provider) { p.UseAlgorithm("HS256") },
		"forged signature":    func(p *oidctest.Provider) { p.SignWithUnknownKey() },
	}
	for name, sabotage := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			sabotage(e.idp)
			b := e.browser()
			if dest := b.signIn(ana); dest != "/login?error=sso_failed" {
				t.Errorf("expected the sign-in to be rejected, went to %s", dest)
			}
			if b.hasSession() {
				t.Error("a rejected sign-in must not open a session")
			}
		})
	}

	// Control: the same flow succeeds when nothing is tampered with, with
	// an audience array that names this client as the authorized party.
	e := newEnv(t)
	e.idp.MutateClaims(func(c map[string]any) {
		c["aud"] = []string{"sgsi-web", "other"}
		c["azp"] = "sgsi-web"
	})
	if dest := e.browser().signIn(ana); dest != "/onboarding" {
		t.Errorf("valid multi-audience token was rejected: %s", dest)
	}
}

func TestSSO_CallbackIsBoundToTheBrowserAndSingleUse(t *testing.T) {
	e := newEnv(t)
	e.idp.SignInAs(ana)

	// The attacker starts a sign-in and captures the callback URL before
	// their browser follows it.
	attacker := e.browser()
	attacker.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if strings.HasPrefix(req.URL.String(), e.rp.URL+"/auth/callback") {
			return http.ErrUseLastResponse
		}
		return nil
	}
	resp, err := attacker.client.Get(e.rp.URL + "/auth/oidc/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	callback := resp.Header.Get("Location")
	if !strings.Contains(callback, "code=") {
		t.Fatalf("expected the callback URL, got %q", callback)
	}

	// Login CSRF: the victim's browser is tricked into opening that URL.
	victim := e.browser()
	resp, err = victim.client.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != appURL+"/login?error=sso_failed" {
		t.Errorf("callback in another browser should be rejected, got %s", loc)
	}
	if victim.hasSession() {
		t.Error("victim's browser was signed in to the attacker's account")
	}

	// The legitimate browser can complete it once…
	attacker.client.CheckRedirect = e.browser().client.CheckRedirect
	resp, _ = attacker.client.Get(callback)
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != appURL+"/onboarding" {
		t.Fatalf("legitimate callback failed: %s", loc)
	}
	// …but not replay it.
	resp, _ = attacker.client.Get(callback)
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != appURL+"/login?error=sso_failed" {
		t.Errorf("replayed callback should be rejected, got %s", loc)
	}
}

func TestSSO_ProviderErrorAndMissingConfiguration(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	resp, _ := b.client.Get(e.rp.URL + "/auth/callback?error=access_denied&state=x")
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != appURL+"/login?error=sso_denied" {
		t.Errorf("provider error: got %s", loc)
	}

	noSSO := newEnv(t, func(o *auth.Options) { o.OIDC = auth.OIDCConfig{} })
	for _, path := range []string{"/auth/oidc/login", "/auth/callback?code=x&state=y"} {
		resp, _ := noSSO.browser().client.Get(noSSO.rp.URL + path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s without SSO configured: expected 404, got %d", path, resp.StatusCode)
		}
	}
	_, cfg := noSSO.browser().do(http.MethodGet, "/auth/config", "")
	if cfg["sso"] != false || cfg["dev_login"] != false || cfg["signup"] != true {
		t.Errorf("unexpected config: %v", cfg)
	}
}

func TestSSO_DiscoveryIssuerMustMatch(t *testing.T) {
	e := newEnv(t, func(o *auth.Options) { o.OIDC.Issuer += "/other-realm" })
	if dest := e.browser().signIn(ana); dest != "/login?error=sso_unavailable" {
		t.Errorf("mismatched discovery should stop the sign-in, went to %s", dest)
	}
}

// ── Organizations ──

func TestOrganizations_SignupCanBeDisabled(t *testing.T) {
	e := newEnv(t, func(o *auth.Options) { o.AllowSignup = false })
	b := e.browser()
	b.signIn(ana)
	if status, _ := b.do(http.MethodPost, "/auth/organizations", `{"name":"Mi empresa"}`); status != http.StatusForbidden {
		t.Errorf("signup disabled: expected 403, got %d", status)
	}
	_, session := b.do(http.MethodGet, "/auth/session", "")
	if session["can_create_organization"] != false {
		t.Errorf("session should say organizations cannot be created: %v", session)
	}
}

func TestOrganizations_Validation(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	if status, _ := b.do(http.MethodPost, "/auth/organizations", `{"name":"Mi empresa"}`); status != http.StatusUnauthorized {
		t.Errorf("without session: expected 401, got %d", status)
	}
	b.signIn(ana)
	for _, body := range []string{`{"name":" "}`, `{"name":"` + strings.Repeat("x", 121) + `"}`} {
		if status, _ := b.do(http.MethodPost, "/auth/organizations", body); status != http.StatusUnprocessableEntity {
			t.Errorf("body %.20s: expected 422, got %d", body, status)
		}
	}
	if status, _ := b.do(http.MethodPost, "/auth/organizations", `{`); status != http.StatusBadRequest {
		t.Errorf("invalid JSON: expected 400, got %d", status)
	}
}

func TestOrganizations_SwitchOnlyAmongOwnMemberships(t *testing.T) {
	e := newEnv(t)
	consultant := e.browser()
	consultant.signIn(ana)
	_, first := consultant.do(http.MethodPost, "/auth/organizations", `{"name":"Cliente Uno"}`)
	_, second := consultant.do(http.MethodPost, "/auth/organizations", `{"name":"Cliente Dos"}`)
	firstID := first["user"].(map[string]any)["tenant_id"].(string)
	secondID := second["user"].(map[string]any)["tenant_id"].(string)

	if orgs := second["organizations"].([]any); len(orgs) != 2 {
		t.Fatalf("expected 2 organizations, got %v", orgs)
	}
	_, who := consultant.do(http.MethodGet, "/api/whoami", "")
	if who["tenant"] != secondID {
		t.Errorf("after creating the second organization the session should be in it: %v", who)
	}

	status, _ := consultant.do(http.MethodPost, "/auth/switch", `{"tenant_id":"`+firstID+`"}`)
	_, who = consultant.do(http.MethodGet, "/api/whoami", "")
	if status != http.StatusOK || who["tenant"] != firstID {
		t.Errorf("switch: %d %v", status, who)
	}

	// Another person's organization is out of reach.
	other := e.browser()
	other.signIn(oidctest.Person{Subject: "sub-otro", Email: "otro@x.co", EmailVerified: true})
	_, foreign := other.do(http.MethodPost, "/auth/organizations", `{"name":"Ajena"}`)
	foreignID := foreign["user"].(map[string]any)["tenant_id"].(string)

	for _, id := range []string{foreignID, "99999999-9999-4999-8999-999999999999"} {
		if status, _ := consultant.do(http.MethodPost, "/auth/switch", `{"tenant_id":"`+id+`"}`); status != http.StatusForbidden {
			t.Errorf("switch to %s: expected 403, got %d", id, status)
		}
	}
	if status, _ := consultant.do(http.MethodPost, "/auth/switch", `{"tenant_id":"nope"}`); status != http.StatusBadRequest {
		t.Errorf("malformed tenant: expected 400, got %d", status)
	}
	if _, who = consultant.do(http.MethodGet, "/api/whoami", ""); who["tenant"] != firstID {
		t.Errorf("failed switches must not move the session: %v", who)
	}
}

// ── Membership is checked on every request ──

func TestMiddleware_RoleChangeAndRemovalApplyImmediately(t *testing.T) {
	e := newEnv(t)
	owner := e.browser()
	owner.signIn(ana)
	_, session := owner.do(http.MethodPost, "/auth/organizations", `{"name":"Empresa A"}`)
	tenantID := session["user"].(map[string]any)["tenant_id"].(string)

	invited, _ := e.dir.Invite(context.Background(), tenantID, "luis@empresa.co", "", auth.RoleEditor)
	luis := e.browser()
	luis.signIn(oidctest.Person{Subject: "sub-luis", Email: "luis@empresa.co", EmailVerified: true})
	if _, who := luis.do(http.MethodGet, "/api/whoami", ""); who["role"] != "editor" {
		t.Fatalf("expected editor, got %v", who)
	}

	if _, err := e.dir.SetRole(context.Background(), tenantID, invited.ID, auth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	if _, who := luis.do(http.MethodGet, "/api/whoami", ""); who["role"] != "viewer" {
		t.Errorf("role change should apply to the existing session, got %v", who)
	}

	if err := e.dir.Remove(context.Background(), tenantID, invited.ID); err != nil {
		t.Fatal(err)
	}
	status, body := luis.do(http.MethodGet, "/api/whoami", "")
	if status != http.StatusForbidden || body["code"] != "no_organization" {
		t.Errorf("removed member should lose access at once: %d %v", status, body)
	}
	if _, s := luis.do(http.MethodGet, "/auth/session", ""); s["needs_organization"] != true || s["user"] != nil {
		t.Errorf("session of a removed member: %v", s)
	}
}

func TestMiddleware_RequiresSession(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	if status, _ := b.do(http.MethodGet, "/api/whoami", ""); status != http.StatusUnauthorized {
		t.Errorf("no session: expected 401, got %d", status)
	}
	req, _ := http.NewRequest(http.MethodGet, e.rp.URL+"/api/whoami", nil)
	req.Header.Set("Authorization", "Bearer forged-token")
	req.Header.Set("X-Tenant-ID", devTenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("forged token: expected 401, got %d", resp.StatusCode)
	}
}

func TestLogout_EndsTheSession(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	b.signIn(ana)
	if !b.hasSession() {
		t.Fatal("expected a session after sign-in")
	}
	if status, _ := b.do(http.MethodPost, "/auth/logout", ""); status != http.StatusNoContent {
		t.Errorf("logout: expected 204, got %d", status)
	}
	if b.hasSession() {
		t.Error("session survived logout")
	}
}

// ── Development login ──

func devEnv(t *testing.T) *env {
	return newEnv(t, func(o *auth.Options) { o.DevMode = true; o.SecureCookies = true })
}

func TestDevLogin_DisabledByDefault(t *testing.T) {
	e := newEnv(t)
	status, _ := e.browser().do(http.MethodPost, "/auth/login", `{"email":"a@b.com","tenant_id":"`+devTenant+`"}`)
	if status != http.StatusNotImplemented {
		t.Errorf("dev login without DevMode: expected 501, got %d", status)
	}
}

func TestDevLogin_CreatesMemberAndSecureCookie(t *testing.T) {
	e := devEnv(t)

	resp, err := http.Post(e.rp.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"Admin@Test.com","password":"x","tenant_id":"`+strings.ToUpper(devTenant)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var session struct {
		Token string     `json:"token"`
		User  *auth.User `json:"user"`
	}
	json.NewDecoder(resp.Body).Decode(&session)
	if resp.StatusCode != http.StatusOK || session.Token == "" || session.User == nil {
		t.Fatalf("login: %d %+v", resp.StatusCode, session)
	}
	// First member of a fresh organization is its owner.
	if session.User.Email != "admin@test.com" || session.User.TenantID != devTenant || session.User.Role != auth.RoleOwner {
		t.Errorf("unexpected user: %+v", session.User)
	}

	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "sgsi_session" || !cookies[0].HttpOnly || !cookies[0].Secure ||
		cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected cookies: %+v", cookies)
	}

	// Bearer token works for non-browser clients, and the tenant always
	// comes from the session, whatever header the client sends.
	req, _ := http.NewRequest(http.MethodGet, e.rp.URL+"/api/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+session.Token)
	req.Header.Set("X-Tenant-ID", "ffffffff-ffff-4fff-8fff-ffffffffffff")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var who map[string]string
	json.NewDecoder(res.Body).Decode(&who)
	if who["tenant"] != devTenant || who["tenant_header"] != devTenant {
		t.Errorf("tenant must come from the session: %v", who)
	}

	// A second person in the same organization defaults to admin; an
	// explicit role is honoured.
	b := e.browser()
	_, second := b.do(http.MethodPost, "/auth/login", `{"email":"otra@test.com","tenant_id":"`+devTenant+`"}`)
	if role := second["user"].(map[string]any)["role"]; role != "admin" {
		t.Errorf("second member should default to admin, got %v", role)
	}
	_, third := b.do(http.MethodPost, "/auth/login", `{"email":"lector@test.com","tenant_id":"`+devTenant+`","role":"viewer"}`)
	if role := third["user"].(map[string]any)["role"]; role != "viewer" {
		t.Errorf("explicit role ignored, got %v", role)
	}
}

func TestDevLogin_Validation(t *testing.T) {
	e := devEnv(t)
	b := e.browser()
	for name, body := range map[string]string{
		"missing email":  `{"tenant_id":"` + devTenant + `"}`,
		"not an email":   `{"email":"admin","tenant_id":"` + devTenant + `"}`,
		"missing tenant": `{"email":"a@b.com"}`,
		"bad tenant":     `{"email":"a@b.com","tenant_id":"t1"}`,
		"unknown role":   `{"email":"a@b.com","tenant_id":"` + devTenant + `","role":"root"}`,
		"invalid JSON":   `{`,
	} {
		if status, _ := b.do(http.MethodPost, "/auth/login", body); status != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", name, status)
		}
	}
}
