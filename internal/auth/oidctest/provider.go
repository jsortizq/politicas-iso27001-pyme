// Package oidctest is a minimal OpenID Connect provider for tests.
//
// It implements just enough of the protocol to exercise the relying party:
// discovery, an authorization endpoint that signs in whoever is configured
// as the current person (no credentials), a token endpoint that enforces
// PKCE and client authentication, and a JWKS endpoint. Tests can tamper with
// the tokens it issues to check that the relying party rejects them.
//
// Never use it outside tests: it authenticates nobody.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"
)

// Person is who the provider signs in on the next authorization request.
type Person struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// Provider is a running fake identity provider.
type Provider struct {
	Server       *httptest.Server
	ClientID     string
	ClientSecret string

	mu      sync.Mutex
	key     *rsa.PrivateKey
	kid     string
	person  Person
	codes   map[string]grant
	mutate  func(claims map[string]any)
	signAlg string
	signKey *rsa.PrivateKey
}

type grant struct {
	person      Person
	nonce       string
	challenge   string
	redirectURI string
}

// New starts a provider. Close it with p.Server.Close().
func New(clientID, clientSecret string) *Provider {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	p := &Provider{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		key:          key,
		kid:          "test-key-1",
		codes:        make(map[string]grant),
		signAlg:      "RS256",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /authorize", p.authorize)
	mux.HandleFunc("POST /token", p.token)
	mux.HandleFunc("GET /jwks", p.jwks)
	p.Server = httptest.NewServer(mux)
	return p
}

// Issuer is the provider's issuer URL.
func (p *Provider) Issuer() string { return p.Server.URL }

// SignInAs sets who the next authorization request signs in.
func (p *Provider) SignInAs(person Person) {
	p.mu.Lock()
	p.person = person
	p.mu.Unlock()
}

// MutateClaims lets a test alter the ID token claims before signing.
func (p *Provider) MutateClaims(fn func(claims map[string]any)) {
	p.mu.Lock()
	p.mutate = fn
	p.mu.Unlock()
}

// SignWithUnknownKey makes the provider sign ID tokens with a key that is
// not published in its JWKS, as an attacker forging tokens would.
func (p *Provider) SignWithUnknownKey() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	p.mu.Lock()
	p.signKey = key
	p.mu.Unlock()
}

// UseAlgorithm overrides the "alg" header of issued tokens ("none" issues
// an unsigned token).
func (p *Provider) UseAlgorithm(alg string) {
	p.mu.Lock()
	p.signAlg = alg
	p.mu.Unlock()
}

func (p *Provider) discovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                 p.Issuer(),
		"authorization_endpoint": p.Issuer() + "/authorize",
		"token_endpoint":         p.Issuer() + "/token",
		"jwks_uri":               p.Issuer() + "/jwks",
	})
}

func (p *Provider) jwks(w http.ResponseWriter, r *http.Request) {
	pub := p.key.PublicKey
	writeJSON(w, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": p.kid,
		"n": b64(pub.N.Bytes()),
		"e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

// authorize signs in the configured person immediately and redirects back
// with a code, like a provider where the person already has a session.
func (p *Provider) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != p.ClientID || q.Get("response_type") != "code" ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		http.Error(w, "invalid authorization request", http.StatusBadRequest)
		return
	}
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || redirect.Host == "" {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}

	code := randomString()
	p.mu.Lock()
	p.codes[code] = grant{
		person:      p.person,
		nonce:       q.Get("nonce"),
		challenge:   q.Get("code_challenge"),
		redirectURI: q.Get("redirect_uri"),
	}
	p.mu.Unlock()

	back := redirect.Query()
	back.Set("code", code)
	back.Set("state", q.Get("state"))
	redirect.RawQuery = back.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if p.ClientSecret != "" {
		id, secret, ok := r.BasicAuth()
		if !ok || id != url.QueryEscape(p.ClientID) || secret != url.QueryEscape(p.ClientSecret) {
			http.Error(w, "invalid_client", http.StatusUnauthorized)
			return
		}
	}

	p.mu.Lock()
	g, ok := p.codes[r.PostForm.Get("code")]
	delete(p.codes, r.PostForm.Get("code")) // codes are single-use
	mutate, alg, signKey := p.mutate, p.signAlg, p.signKey
	p.mu.Unlock()

	verifier := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || r.PostForm.Get("grant_type") != "authorization_code" ||
		r.PostForm.Get("redirect_uri") != g.redirectURI ||
		b64(verifier[:]) != g.challenge {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
		return
	}

	now := time.Now()
	claims := map[string]any{
		"iss":            p.Issuer(),
		"sub":            g.person.Subject,
		"aud":            p.ClientID,
		"exp":            now.Add(5 * time.Minute).Unix(),
		"iat":            now.Unix(),
		"nonce":          g.nonce,
		"email":          g.person.Email,
		"email_verified": g.person.EmailVerified,
		"name":           g.person.Name,
	}
	if mutate != nil {
		mutate(claims)
	}
	if signKey == nil {
		signKey = p.key
	}
	writeJSON(w, map[string]any{
		"access_token": "opaque-access-token",
		"token_type":   "Bearer",
		"id_token":     sign(claims, alg, p.kid, signKey),
	})
}

func sign(claims map[string]any, alg, kid string, key *rsa.PrivateKey) string {
	header, _ := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	signingInput := b64(header) + "." + b64(payload)
	if alg == "none" {
		return signingInput + "."
	}
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		panic(err)
	}
	return signingInput + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func randomString() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b64(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
