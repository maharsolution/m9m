package webhooks

// JWT fuzzing harness for BUG-SEC-2026-10-12-001.
//
// Goal: never panic, never log a secret, always return a typed
// error from `verifyJWT` regardless of the input shape. The
// handler maps the typed errors to n8n's wire shape (403 + body
// "jwt expired" / "jwt not yet valid"); unknown errors map to
// 401 with body "unauthorized". The harness exercises the
// negative cases end-to-end so a future refactor can't
// regress them.

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

// makeHS256 mints a valid HS256 token for the given claims and
// secret. Used as the "control" — every negative test starts
// from a known-good token and mutates one byte / claim.
func makeHS256(t *testing.T, claims map[string]interface{}, secret string) string {
	t.Helper()
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(cb)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// makeRSAPublicKeyPEM serialises an RSA public key to PEM so
// the production verifyJWT (which calls parseRSAPublicKey) can
// load it.
func makeRSAPublicKeyPEM(t *testing.T, pub *rsa.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

// TestVerifyJWT_Fuzz_HappyPath — a control: with a valid token,
// verifyJWT returns nil. Guards against false positives in the
// rest of the suite.
func TestVerifyJWT_Fuzz_HappyPath(t *testing.T) {
	secret := "this-is-a-32-char-shared-secret!"
	tok := makeHS256(t, map[string]interface{}{
		"sub": "alice",
		"iss": "m9m-test",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}, secret)
	authData := map[string]interface{}{"algorithm": "HS256", "secret": secret}
	if err := verifyJWT(tok, authData); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// TestVerifyJWT_Fuzz_Expired — past `exp` must surface as
// errJWTExpired, not as a generic "malformed" / "invalid" /
// nil. The handler depends on the sentinel for its 403 response.
func TestVerifyJWT_Fuzz_Expired(t *testing.T) {
	secret := "this-is-a-32-char-shared-secret!"
	tok := makeHS256(t, map[string]interface{}{
		"sub": "alice",
		"exp": time.Now().Add(-time.Hour).Unix(),
	}, secret)
	err := verifyJWT(tok, map[string]interface{}{"algorithm": "HS256", "secret": secret})
	if err == nil {
		t.Fatal("expected errJWTExpired, got nil")
	}
	if err != errJWTExpired {
		t.Fatalf("expected errJWTExpired sentinel, got %v", err)
	}
}

// TestVerifyJWT_Fuzz_NotYetValid — future `nbf` returns
// errJWTNotYetValid.
func TestVerifyJWT_Fuzz_NotYetValid(t *testing.T) {
	secret := "this-is-a-32-char-shared-secret!"
	tok := makeHS256(t, map[string]interface{}{
		"sub": "alice",
		"nbf": time.Now().Add(time.Hour).Unix(),
	}, secret)
	err := verifyJWT(tok, map[string]interface{}{"algorithm": "HS256", "secret": secret})
	if err != errJWTNotYetValid {
		t.Fatalf("expected errJWTNotYetValid, got %v", err)
	}
}

// TestVerifyJWT_Fuzz_MalformedSegments — wrong number of
// segments returns a clean "malformed" error, no panic.
func TestVerifyJWT_Fuzz_MalformedSegments(t *testing.T) {
	cases := []string{
		"",
		"only.one",
		"a.b.c.d",
		"...",
		"no-dots-at-all",
		strings.Repeat("a", 1024),
	}
	for _, c := range cases {
		if err := verifyJWT(c, map[string]interface{}{"algorithm": "HS256", "secret": "x"}); err == nil {
			t.Errorf("expected error for input %q, got nil", c)
		}
	}
}

// TestVerifyJWT_Fuzz_BadBase64 — a header / payload / signature
// segment that isn't valid base64url must error, not panic.
func TestVerifyJWT_Fuzz_BadBase64(t *testing.T) {
	// "***" is not valid base64url.
	tok := "***.***.***"
	err := verifyJWT(tok, map[string]interface{}{"algorithm": "HS256", "secret": "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestVerifyJWT_Fuzz_BadHeaderJSON — header is valid base64url
// but the bytes don't unmarshal to {alg, typ}. Must error, not
// panic.
func TestVerifyJWT_Fuzz_BadHeaderJSON(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x"}`))
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	tok := header + "." + payload + "." + sig
	err := verifyJWT(tok, map[string]interface{}{"algorithm": "HS256", "secret": "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestVerifyJWT_Fuzz_BadPayloadJSON — header is fine, payload
// is not JSON. Must error, not panic.
func TestVerifyJWT_Fuzz_BadPayloadJSON(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	tok := header + "." + payload + "." + sig
	err := verifyJWT(tok, map[string]interface{}{"algorithm": "HS256", "secret": "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestVerifyJWT_Fuzz_UnsupportedAlg — alg=none, alg=RS512
// (only RS256 / HS256 supported), alg=" " all error.
func TestVerifyJWT_Fuzz_UnsupportedAlg(t *testing.T) {
	cases := []string{"none", "RS512", "ES256", "PS256", "", " "}
	for _, alg := range cases {
		hb, _ := json.Marshal(map[string]string{"alg": alg, "typ": "JWT"})
		pb, _ := json.Marshal(map[string]string{"sub": "x"})
		tok := base64.RawURLEncoding.EncodeToString(hb) + "." +
			base64.RawURLEncoding.EncodeToString(pb) + "." +
			base64.RawURLEncoding.EncodeToString([]byte("sig"))
		err := verifyJWT(tok, map[string]interface{}{"algorithm": "HS256", "secret": "x"})
		if err == nil {
			t.Errorf("expected error for alg=%q, got nil", alg)
		}
	}
}

// TestVerifyJWT_Fuzz_AlgorithmConfusion — classic CVE-2015-9235:
// an RS256 token (header claims RS256) is verified against the
// HS256 secret path. m9m must reject this. We don't have a real
// RS256 key in this fuzz; we just check that passing a
// `{"alg":"RS256"}` header with only an HS256 secret returns
// "missing public key", not nil.
func TestVerifyJWT_Fuzz_AlgorithmConfusion(t *testing.T) {
	hb, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	pb, _ := json.Marshal(map[string]string{"sub": "x"})
	tok := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(pb) + "." +
		base64.RawURLEncoding.EncodeToString([]byte("sig"))
	err := verifyJWT(tok, map[string]interface{}{"algorithm": "HS256", "secret": "x"})
	if err == nil {
		t.Fatal("expected error (no public key), got nil")
	}
	if !strings.Contains(err.Error(), "public key") {
		t.Fatalf("expected 'public key' error, got %v", err)
	}
}

// TestVerifyJWT_Fuzz_WrongSecret — a valid token signed with a
// different secret must error.
func TestVerifyJWT_Fuzz_WrongSecret(t *testing.T) {
	tok := makeHS256(t, map[string]interface{}{"sub": "alice"}, "right-secret")
	err := verifyJWT(tok, map[string]interface{}{"algorithm": "HS256", "secret": "wrong-secret"})
	if err == nil {
		t.Fatal("expected signature verification failure, got nil")
	}
}

// TestVerifyJWT_Fuzz_RS256_Valid — a real RSA-signed token
// verifies successfully. Requires generating an RSA key per
// test, so we keep it short.
func TestVerifyJWT_Fuzz_RS256_Valid(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pemPub := makeRSAPublicKeyPEM(t, &priv.PublicKey)

	claims := map[string]interface{}{
		"sub": "alice",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	hb, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	cb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(cb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	tok := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	err = verifyJWT(tok, map[string]interface{}{"algorithm": "RS256", "publicKey": pemPub})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestVerifyJWT_Fuzz_RS256_BadSignature — valid header, valid
// claims, but the signature was produced with a different key.
// Must error.
func TestVerifyJWT_Fuzz_RS256_BadSignature(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemPub := makeRSAPublicKeyPEM(t, &other.PublicKey)

	claims := map[string]interface{}{"sub": "alice", "exp": time.Now().Add(time.Hour).Unix()}
	hb, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	cb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(cb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest[:])
	tok := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	err := verifyJWT(tok, map[string]interface{}{"algorithm": "RS256", "publicKey": pemPub})
	if err == nil {
		t.Fatal("expected signature verification failure, got nil")
	}
}

// TestVerifyJWT_Fuzz_EmptyAuth — empty authData (e.g. the
// credential lookup silently returned nothing) errors, never
// silently passes.
func TestVerifyJWT_Fuzz_EmptyAuth(t *testing.T) {
	secret := "any"
	tok := makeHS256(t, map[string]interface{}{"sub": "x"}, secret)
	if err := verifyJWT(tok, map[string]interface{}{}); err == nil {
		t.Fatal("expected error (missing secret), got nil")
	}
	if err := verifyJWT(tok, nil); err == nil {
		t.Fatal("expected error (nil authData), got nil")
	}
}
