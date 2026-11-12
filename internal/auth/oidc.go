package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OpenID Connect sign-in: Authorization Code flow with PKCE.
//
//	GET /auth/oidc/login  → redirects the browser to the identity provider
//	GET /auth/callback    → exchanges the code, verifies the ID token and
//	                        opens a session
//
// The implementation uses only the standard library. ID tokens must be
// signed with RS256, which every major provider supports and most use by
// default; any other algorithm (including "none") is rejected.

// OIDCConfig holds the relying-party settings.
type OIDCConfig struct {
	Issuer       string // e.g. https://idp.example.com/realms/sgsi
	ClientID     string
	ClientSecret string // empty for public clients (PKCE only)
	RedirectURL  string // must be registered at the provider; ends in /auth/callback
	Scopes       []string
}

const (
	oidcStateCookie = "sgsi_oidc_state"
	oidcFlowTTL     = 10 * time.Minute
	oidcMaxPending  = 10000
	clockLeeway     = time.Minute
	jwksMinRefresh  = time.Minute
)

type providerMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type pendingFlow struct {
	nonce    string
	verifier string
	expires  time.Time
}

type oidcProvider struct {
	cfg    OIDCConfig
	client *http.Client
	now    func() time.Time

	mu          sync.Mutex
	meta        *providerMetadata
	keys        map[string]*rsa.PublicKey
	keysFetched time.Time
	pending     map[string]pendingFlow // by state
}

func newOIDCProvider(cfg OIDCConfig) *oidcProvider {
	cfg.Issuer = strings.TrimRight(cfg.Issuer, "/")
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "email", "profile"}
	}
	return &oidcProvider{
		cfg:     cfg,
		client:  &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
		pending: make(map[string]pendingFlow),
	}
}

// ── HTTP handlers ──

func (h *Handler) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		writeAuthError(w, http.StatusNotFound, "single sign-on is not configured", "")
		return
	}
	target, state, err := h.oidc.begin(r.Context())
	if err != nil {
		h.logger.Error("oidc: cannot start sign-in", "error", err)
		h.redirectToLogin(w, r, "sso_unavailable")
		return
	}
	// Bind the flow to this browser: the callback only accepts a state that
	// matches this cookie, so a sign-in started elsewhere cannot be completed here.
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: state, Path: "/auth", MaxAge: int(oidcFlowTTL.Seconds()),
		HttpOnly: true, Secure: h.opts.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		writeAuthError(w, http.StatusNotFound, "single sign-on is not configured", "")
		return
	}
	// The state cookie is single-use.
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: "", Path: "/auth", MaxAge: -1,
		HttpOnly: true, Secure: h.opts.SecureCookies, SameSite: http.SameSiteLaxMode,
	})

	query := r.URL.Query()
	if providerErr := query.Get("error"); providerErr != "" {
		h.logger.Warn("oidc: provider returned an error", "error", providerErr)
		h.redirectToLogin(w, r, "sso_denied")
		return
	}

	state := query.Get("state")
	cookie, err := r.Cookie(oidcStateCookie)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		h.logger.Warn("oidc: state does not match this browser")
		h.redirectToLogin(w, r, "sso_failed")
		return
	}

	identity, err := h.oidc.complete(r.Context(), state, query.Get("code"))
	if err != nil {
		h.logger.Warn("oidc: sign-in rejected", "error", err)
		h.redirectToLogin(w, r, "sso_failed")
		return
	}

	memberships, err := h.dir.Resolve(r.Context(), *identity)
	if err != nil {
		h.logger.Error("oidc: resolve memberships failed", "error", err)
		h.redirectToLogin(w, r, "sso_failed")
		return
	}
	var active *Membership
	if len(memberships) > 0 {
		active = &memberships[0]
	}
	token, err := h.sessions.Create(*identity, active)
	if err != nil {
		h.logger.Error("oidc: create session failed", "error", err)
		h.redirectToLogin(w, r, "sso_failed")
		return
	}
	h.setSessionCookie(w, token)

	destination := "/dashboard"
	if active == nil {
		destination = "/onboarding"
	}
	http.Redirect(w, r, h.opts.AppURL+destination, http.StatusFound)
}

func (h *Handler) redirectToLogin(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, h.opts.AppURL+"/login?error="+url.QueryEscape(code), http.StatusFound)
}

// ── Flow ──

// begin registers a new sign-in attempt and returns the provider URL to
// send the browser to, plus the state that identifies the attempt.
func (p *oidcProvider) begin(ctx context.Context) (target, state string, err error) {
	meta, err := p.metadata(ctx)
	if err != nil {
		return "", "", err
	}
	state, err = randomURLSafe(32)
	if err != nil {
		return "", "", err
	}
	nonce, err := randomURLSafe(32)
	if err != nil {
		return "", "", err
	}
	verifier, err := randomURLSafe(32) // 43 characters, as RFC 7636 requires
	if err != nil {
		return "", "", err
	}

	p.mu.Lock()
	now := p.now()
	for s, flow := range p.pending {
		if now.After(flow.expires) {
			delete(p.pending, s)
		}
	}
	if len(p.pending) >= oidcMaxPending {
		p.mu.Unlock()
		return "", "", errors.New("oidc: too many sign-ins in progress")
	}
	p.pending[state] = pendingFlow{nonce: nonce, verifier: verifier, expires: now.Add(oidcFlowTTL)}
	p.mu.Unlock()

	challenge := sha256.Sum256([]byte(verifier))
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {p.cfg.ClientID},
		"redirect_uri":          {p.cfg.RedirectURL},
		"scope":                 {strings.Join(p.cfg.Scopes, " ")},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	separator := "?"
	if strings.Contains(meta.AuthorizationEndpoint, "?") {
		separator = "&"
	}
	return meta.AuthorizationEndpoint + separator + params.Encode(), state, nil
}

// complete finishes a sign-in attempt: it redeems the authorization code
// and returns the identity asserted by a valid ID token.
func (p *oidcProvider) complete(ctx context.Context, state, code string) (*Identity, error) {
	p.mu.Lock()
	flow, ok := p.pending[state]
	delete(p.pending, state) // one attempt per state
	p.mu.Unlock()
	if !ok || p.now().After(flow.expires) {
		return nil, errors.New("oidc: unknown or expired sign-in attempt")
	}
	if code == "" {
		return nil, errors.New("oidc: missing authorization code")
	}

	meta, err := p.metadata(ctx)
	if err != nil {
		return nil, err
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.cfg.RedirectURL},
		"client_id":     {p.cfg.ClientID},
		"code_verifier": {flow.verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if p.cfg.ClientSecret != "" {
		req.SetBasicAuth(url.QueryEscape(p.cfg.ClientID), url.QueryEscape(p.cfg.ClientSecret))
	}

	var tokens struct {
		IDToken string `json:"id_token"`
	}
	if err := p.doJSON(req, &tokens); err != nil {
		return nil, fmt.Errorf("oidc: token exchange: %w", err)
	}
	if tokens.IDToken == "" {
		return nil, errors.New("oidc: provider returned no id_token")
	}

	claims, err := p.verifyIDToken(ctx, tokens.IDToken)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(flow.nonce)) != 1 {
		return nil, errors.New("oidc: nonce mismatch")
	}

	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}
	if name == "" {
		name = nameFromEmail(claims.Email)
	}
	return &Identity{
		Subject:       claims.Subject,
		Email:         strings.ToLower(strings.TrimSpace(claims.Email)),
		EmailVerified: bool(claims.EmailVerified),
		Name:          name,
	}, nil
}

// ── ID token verification ──

type idTokenClaims struct {
	Issuer            string   `json:"iss"`
	Subject           string   `json:"sub"`
	Audience          audience `json:"aud"`
	AuthorizedParty   string   `json:"azp"`
	Expiry            int64    `json:"exp"`
	IssuedAt          int64    `json:"iat"`
	Nonce             string   `json:"nonce"`
	Email             string   `json:"email"`
	EmailVerified     flexBool `json:"email_verified"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
}

// audience accepts both forms allowed by the spec: a string or an array.
type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

// flexBool accepts true/false and the "true"/"false" strings some providers send.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch strings.Trim(string(data), `"`) {
	case "true":
		*b = true
	default:
		*b = false
	}
	return nil
}

func (p *oidcProvider) verifyIDToken(ctx context.Context, raw string) (*idTokenClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("oidc: malformed id_token")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("oidc: malformed id_token header")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, errors.New("oidc: malformed id_token header")
	}
	// Pin the algorithm: never let the token choose how it is verified.
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("oidc: unsupported id_token algorithm %q (only RS256)", header.Alg)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("oidc: malformed id_token signature")
	}
	key, err := p.signingKey(ctx, header.Kid)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return nil, errors.New("oidc: id_token signature is invalid")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("oidc: malformed id_token payload")
	}
	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.New("oidc: malformed id_token claims")
	}

	now := p.now()
	switch {
	case strings.TrimRight(claims.Issuer, "/") != p.cfg.Issuer:
		return nil, errors.New("oidc: id_token issued by a different provider")
	case !claims.Audience.contains(p.cfg.ClientID):
		return nil, errors.New("oidc: id_token is for a different client")
	case len(claims.Audience) > 1 && claims.AuthorizedParty != p.cfg.ClientID:
		return nil, errors.New("oidc: id_token authorized party mismatch")
	case claims.Expiry == 0 || now.After(time.Unix(claims.Expiry, 0).Add(clockLeeway)):
		return nil, errors.New("oidc: id_token expired")
	case claims.IssuedAt != 0 && time.Unix(claims.IssuedAt, 0).After(now.Add(clockLeeway)):
		return nil, errors.New("oidc: id_token issued in the future")
	case claims.Subject == "":
		return nil, errors.New("oidc: id_token has no subject")
	}
	return &claims, nil
}

func (a audience) contains(v string) bool {
	for _, s := range a {
		if s == v {
			return true
		}
	}
	return false
}

// ── Provider metadata and keys ──

// metadata returns the provider's discovery document, fetching it on first
// use so the API can start while the provider is temporarily unreachable.
func (p *oidcProvider) metadata(ctx context.Context) (*providerMetadata, error) {
	p.mu.Lock()
	meta := p.meta
	p.mu.Unlock()
	if meta != nil {
		return meta, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.Issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	var fetched providerMetadata
	if err := p.doJSON(req, &fetched); err != nil {
		return nil, fmt.Errorf("oidc: discovery: %w", err)
	}
	if strings.TrimRight(fetched.Issuer, "/") != p.cfg.Issuer {
		return nil, fmt.Errorf("oidc: discovery issuer %q does not match OIDC_ISSUER", fetched.Issuer)
	}
	if fetched.AuthorizationEndpoint == "" || fetched.TokenEndpoint == "" || fetched.JWKSURI == "" {
		return nil, errors.New("oidc: discovery document is incomplete")
	}

	p.mu.Lock()
	p.meta = &fetched
	p.mu.Unlock()
	return &fetched, nil
}

// signingKey returns the provider key with the given id. Unknown ids
// trigger a refresh (providers rotate keys), at most once per minute.
func (p *oidcProvider) signingKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if key := p.cachedKey(kid); key != nil {
		return key, nil
	}

	p.mu.Lock()
	recentlyFetched := p.keys != nil && p.now().Sub(p.keysFetched) < jwksMinRefresh
	p.mu.Unlock()
	if recentlyFetched {
		return nil, errors.New("oidc: id_token signed with an unknown key")
	}

	meta, err := p.metadata(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.JWKSURI, nil)
	if err != nil {
		return nil, err
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := p.doJSON(req, &set); err != nil {
		return nil, fmt.Errorf("oidc: fetch keys: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range set.Keys {
		if k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(n) < 256 { // require at least 2048-bit keys
			continue
		}
		exponent := new(big.Int).SetBytes(e)
		if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > 1<<31-1 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent.Int64())}
	}

	p.mu.Lock()
	p.keys = keys
	p.keysFetched = p.now()
	p.mu.Unlock()

	if key := p.cachedKey(kid); key != nil {
		return key, nil
	}
	return nil, errors.New("oidc: id_token signed with an unknown key")
}

func (p *oidcProvider) cachedKey(kid string) *rsa.PublicKey {
	p.mu.Lock()
	defer p.mu.Unlock()
	if key, ok := p.keys[kid]; ok {
		return key
	}
	// Tokens without a key id are acceptable only when there is no ambiguity.
	if kid == "" && len(p.keys) == 1 {
		for _, key := range p.keys {
			return key
		}
	}
	return nil
}

func (p *oidcProvider) doJSON(req *http.Request, v any) error {
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return json.Unmarshal(body, v)
}
