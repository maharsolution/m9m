package api

// Hermetic tests for BUG-SEC-2026-10-12-003 — OIDC provider.
//
// Strategy: stand up a tiny `httptest.NewServer` that serves
// both `.well-known/openid-configuration` and `/jwks.json`.
// Sign the ID token with a real RSA key, embed the public
// half in the JWKS, point the provider at the test server.
// No external network.

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type oidcFixture struct {
	server   *httptest.Server
	issuer   string
	key      *rsa.PrivateKey
	keyID    string
	provider *OIDCProvider
	clientID string
}

// setupOIDC spins up the fixture. `sign` and `claims` are
// applied to mint the ID token under test.
func setupOIDC(t *testing.T, mutate func(claims jwt.MapClaims)) *oidcFixture {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa: %v", err)
	}
	const kid = "test-key-1"
	const clientID = "m9m-test"

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 server.URL,
			"jwks_uri":               server.URL + "/jwks.json",
			"authorization_endpoint": server.URL + "/authorize",
			"token_endpoint":         server.URL + "/token",
		})
	})
	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// JWKS with one RSA key.
		n := base64.RawURLEncoding.EncodeToString(priv.PublicKey.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.PublicKey.E)).Bytes())
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
				"n": n, "e": e,
			}},
		})
	})

	provider, err := NewOIDCProvider(OIDCConfig{
		IssuerURL: server.URL,
		ClientID:  clientID,
	})
	if err != nil {
		t.Fatalf("NewOIDCProvider: %v", err)
	}

	claims := jwt.MapClaims{
		"iss":   server.URL,
		"aud":   clientID,
		"sub":   "user-123",
		"email": "user@example.com",
		"name":  "Test User",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"groups": []string{"editors", "viewers"},
	}
	if mutate != nil {
		mutate(claims)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	if _, err = tok.SignedString(priv); err != nil {
		t.Fatalf("sign: %v", err)
	}

	// The fixture returns the URL + private key so callers
	// can mint additional tokens.
	return &oidcFixture{
		server:   server,
		issuer:   server.URL,
		key:      priv,
		keyID:    kid,
		provider: provider,
		clientID: clientID,
	}
}

func (f *oidcFixture) Close() { f.server.Close() }

// signWithClaims mints a fresh ID token against the same
// key/issuer so each test starts from a known-good state.
func (f *oidcFixture) signWithClaims(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.keyID
	signed, err := tok.SignedString(f.key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func TestOIDC_Verify_HappyPath(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	id, err := f.provider.Verify(mintIDTokenFromFixture(t, f))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", id.Subject)
	}
	if id.Email != "user@example.com" {
		t.Errorf("Email = %q", id.Email)
	}
	if id.DisplayName != "Test User" {
		t.Errorf("DisplayName = %q", id.DisplayName)
	}
	if id.Provider != "oidc" {
		t.Errorf("Provider = %q, want oidc", id.Provider)
	}
	wantRoles := map[string]bool{"editors": true, "viewers": true}
	for _, r := range id.Roles {
		if !wantRoles[r] {
			t.Errorf("unexpected role %q", r)
		}
		delete(wantRoles, r)
	}
	if len(wantRoles) != 0 {
		t.Errorf("missing roles: %v", wantRoles)
	}
}

func TestOIDC_Verify_RejectsBadSignature(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	// Mint a token signed with a different key.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	claims := jwt.MapClaims{
		"iss": f.issuer, "aud": f.clientID, "sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.keyID
	signed, _ := tok.SignedString(other)
	if _, err := f.provider.Verify(signed); err == nil {
		t.Fatal("expected bad signature rejection")
	}
}

func TestOIDC_Verify_RejectsBadIssuer(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	claims := jwt.MapClaims{
		"iss": "https://evil.example.com",
		"aud": f.clientID, "sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if _, err := f.provider.Verify(f.signWithClaims(t, claims)); err == nil {
		t.Fatal("expected iss-mismatch rejection")
	}
}

func TestOIDC_Verify_RejectsBadAudience(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	claims := jwt.MapClaims{
		"iss": f.issuer, "aud": "other-client", "sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if _, err := f.provider.Verify(f.signWithClaims(t, claims)); err == nil {
		t.Fatal("expected aud-mismatch rejection")
	}
}

func TestOIDC_Verify_RejectsExpired(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	claims := jwt.MapClaims{
		"iss": f.issuer, "aud": f.clientID, "sub": "x",
		"exp": time.Now().Add(-time.Hour).Unix(),
	}
	if _, err := f.provider.Verify(f.signWithClaims(t, claims)); err == nil {
		t.Fatal("expected expired rejection")
	}
}

func TestOIDC_Verify_RejectsUnknownKID(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	// Sign with the right key but with a kid the JWKS
	// doesn't know about.
	claims := jwt.MapClaims{
		"iss": f.issuer, "aud": f.clientID, "sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "not-in-jwks"
	signed, _ := tok.SignedString(f.key)
	if _, err := f.provider.Verify(signed); err == nil {
		t.Fatal("expected kid-miss rejection")
	}
}

func TestOIDC_Verify_RejectsEmptySubject(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	claims := jwt.MapClaims{
		"iss": f.issuer, "aud": f.clientID, "sub": "",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if _, err := f.provider.Verify(f.signWithClaims(t, claims)); !IsSSOError(err) {
		t.Fatalf("expected SSOError on empty subject, got %v", err)
	}
}

func TestOIDC_Verify_RejectsAlgNone(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	// `alg=none` is rejected by the parser's `WithValidMethods`
	// list. Mint a token with alg=none — golang-jwt won't let
	// us sign with it, so we hand-craft the wire format.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT","kid":"test-key-1"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"iss":%q,"aud":%q,"sub":"x","exp":%d}`,
		f.issuer, f.clientID, time.Now().Add(time.Hour).Unix(),
	)))
	tok := header + "." + payload + "."
	if _, err := f.provider.Verify(tok); err == nil {
		t.Fatal("expected alg=none rejection")
	}
}

func TestOIDC_Verify_RejectsBadHeader(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	// Header isn't base64url-decodable JSON.
	bogus := "***.***.***"
	if _, err := f.provider.Verify(bogus); err == nil {
		t.Fatal("expected malformed-token rejection")
	}
}

func TestOIDC_NewOIDCProvider_ValidatesConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  OIDCConfig
	}{
		{"empty issuer", OIDCConfig{ClientID: "x"}},
		{"empty client id", OIDCConfig{IssuerURL: "https://x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewOIDCProvider(c.cfg); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestOIDC_Verify_AudienceArray(t *testing.T) {
	// Some IdPs send `aud` as an array. The provider must
	// match against any element.
	f := setupOIDC(t, nil)
	defer f.Close()
	claims := jwt.MapClaims{
		"iss": f.issuer, "aud": []string{"other", f.clientID}, "sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if _, err := f.provider.Verify(f.signWithClaims(t, claims)); err != nil {
		t.Errorf("expected success with array aud, got %v", err)
	}
}

func TestOIDC_GroupsClaim_Configurable(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	f.provider.cfg.GroupsClaim = "roles"
	claims := jwt.MapClaims{
		"iss": f.issuer, "aud": f.clientID, "sub": "x",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []string{"admin", "ops"},
	}
	id, err := f.provider.Verify(f.signWithClaims(t, claims))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(id.Roles) != 2 || id.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin ops]", id.Roles)
	}
}

func TestOIDC_Refresh_ClearsCache(t *testing.T) {
	f := setupOIDC(t, nil)
	defer f.Close()
	// First Verify populates the cache.
	if _, err := f.provider.Verify(mintIDTokenFromFixture(t, f)); err != nil {
		t.Fatalf("first Verify: %v", err)
	}
	// Drop the server — Refresh should error.
	f.server.Close()
	// Manually set short TTL so the next call would need
	// to refetch, but Refresh forces it regardless.
	if err := f.provider.Refresh(); err == nil {
		t.Fatal("expected Refresh to error after server shutdown")
	}
}

func TestOIDC_DecodeJWTHeader(t *testing.T) {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"abc","typ":"JWT"}`))
	tok := h + ".e30.x"
	hdr, err := decodeJWTHeader(tok)
	if err != nil {
		t.Fatalf("decodeJWTHeader: %v", err)
	}
	if hdr.Alg != "RS256" || hdr.Kid != "abc" {
		t.Errorf("header = %+v", hdr)
	}
	if _, err := decodeJWTHeader("not.a.jwt"); err == nil {
		t.Errorf("expected error on 4-part token")
	}
	if _, err := decodeJWTHeader("only.two"); err == nil {
		t.Errorf("expected error on 2-part token")
	}
}

func TestOIDC_AudienceMatches(t *testing.T) {
	if !audienceMatches("m9m", "m9m") {
		t.Error("string aud should match")
	}
	if audienceMatches("m9m", "other") {
		t.Error("different strings should not match")
	}
	if !audienceMatches([]interface{}{"a", "m9m", "b"}, "m9m") {
		t.Error("array containing match should match")
	}
	if audienceMatches([]interface{}{1, 2}, "m9m") {
		t.Error("non-string array should not match")
	}
}

func TestOIDC_ExtractGroups(t *testing.T) {
	if got := extractGroups(jwt.MapClaims{"groups": []interface{}{"a", "b"}}, "groups"); len(got) != 2 {
		t.Errorf("array groups = %v", got)
	}
	if got := extractGroups(jwt.MapClaims{"groups": "single"}, "groups"); len(got) != 1 {
		t.Errorf("string groups = %v", got)
	}
	if got := extractGroups(jwt.MapClaims{}, "groups"); got != nil {
		t.Errorf("missing groups = %v, want nil", got)
	}
}

// mintIDTokenFromFixture mints the canonical happy-path ID
// token against the fixture (used by the HappyPath and
// Refresh tests).
func mintIDTokenFromFixture(t *testing.T, f *oidcFixture) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":    f.issuer,
		"aud":    f.clientID,
		"sub":    "user-123",
		"email":  "user@example.com",
		"name":   "Test User",
		"exp":    time.Now().Add(time.Hour).Unix(),
		"iat":    time.Now().Unix(),
		"groups": []string{"editors", "viewers"},
	}
	return f.signWithClaims(t, claims)
}

// avoid unused-import warnings
var _ = strings.NewReader
