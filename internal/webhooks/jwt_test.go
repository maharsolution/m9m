package webhooks

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
	"sync"
	"testing"
	"time"
)

// sharedTestKey is a lazily-initialised 1024-bit RSA keypair shared
// by every test in this package. Generating a fresh key in every
// test is expensive (~5s per keygen on this CI box) and unnecessary
// because these tokens are never trusted by an external system.
// TestVerifyJWT_BadSignature needs a separate key, so we cache one
// extra "wrong" key alongside.
var (
	sharedKeyOnce sync.Once
	sharedPubPEM  string
	sharedPriv    *rsa.PrivateKey

	wrongKeyOnce sync.Once
	wrongPubPEM  string
	wrongPriv    *rsa.PrivateKey
)

func initSharedKey(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	sharedKeyOnce.Do(func() {
		priv, err := rsa.GenerateKey(rand.Reader, 1024)
		if err != nil {
			t.Fatalf("rsa.GenerateKey: %v", err)
		}
		pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		if err != nil {
			t.Fatalf("MarshalPKIXPublicKey: %v", err)
		}
		sharedPubPEM = string(pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pubBytes,
		}))
		sharedPriv = priv
	})
	return sharedPubPEM, sharedPriv
}

func initWrongKey(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	wrongKeyOnce.Do(func() {
		priv, err := rsa.GenerateKey(rand.Reader, 1024)
		if err != nil {
			t.Fatalf("rsa.GenerateKey: %v", err)
		}
		pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		if err != nil {
			t.Fatalf("MarshalPKIXPublicKey: %v", err)
		}
		wrongPubPEM = string(pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pubBytes,
		}))
		wrongPriv = priv
	})
	return wrongPubPEM, wrongPriv
}

// signTestJWT builds a JWS using `alg=RS256` against the supplied
// private key. `claims` is the JSON object to embed as payload.
func signTestJWT(t *testing.T, alg string, priv *rsa.PrivateKey, hmacSecret string,
	header map[string]interface{}, claims map[string]interface{}) string {
	t.Helper()
	hdrJSON, _ := json.Marshal(header)
	clmJSON, _ := json.Marshal(claims)
	hdrB64 := base64.RawURLEncoding.EncodeToString(hdrJSON)
	clmB64 := base64.RawURLEncoding.EncodeToString(clmJSON)
	signingInput := hdrB64 + "." + clmB64

	var sig []byte
	switch alg {
	case "RS256":
		digest := sha256.Sum256([]byte(signingInput))
		s, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatalf("rsa.SignPKCS1v15: %v", err)
		}
		sig = s
	case "HS256":
		// (not used by these tests, but keep the shape for clarity)
		mac := hmac.New(sha256.New, []byte(hmacSecret))
		mac.Write([]byte(signingInput))
		sig = mac.Sum(nil)
	default:
		t.Fatalf("unsupported alg %q", alg)
	}
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64
}

func TestVerifyJWT_ValidRS256(t *testing.T) {
	pubPEM, priv := initSharedKey(t)

	now := time.Now().Unix()
	claims := map[string]interface{}{
		"sub": "1234567890",
		"name": "Tester",
		"iat": now,
		"exp": now + 3600,
	}
	tok := signTestJWT(t, "RS256", priv, "", map[string]interface{}{
		"alg": "RS256", "typ": "JWT",
	}, claims)

	authData := map[string]interface{}{"publicKey": pubPEM}
	if err := verifyJWT(tok, authData); err != nil {
		t.Fatalf("verifyJWT(valid): %v", err)
	}
}

func TestVerifyJWT_Expired(t *testing.T) {
	pubPEM, priv := initSharedKey(t)
	// exp set 1 second in the past to guarantee now >= exp
	exp := time.Now().Unix() - 1
	claims := map[string]interface{}{
		"sub": "1234567890",
		"exp": exp,
	}
	tok := signTestJWT(t, "RS256", priv, "", map[string]interface{}{
		"alg": "RS256", "typ": "JWT",
	}, claims)

	authData := map[string]interface{}{"publicKey": pubPEM}
	err := verifyJWT(tok, authData)
	if err == nil {
		t.Fatal("verifyJWT(expired) returned nil; expected errJWTExpired")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("verifyJWT(expired) = %v; want errJWTExpired", err)
	}
}

func TestVerifyJWT_NotYetValid(t *testing.T) {
	pubPEM, priv := initSharedKey(t)
	nbf := time.Now().Unix() + 3600
	claims := map[string]interface{}{
		"sub": "1234567890",
		"nbf": nbf,
	}
	tok := signTestJWT(t, "RS256", priv, "", map[string]interface{}{
		"alg": "RS256", "typ": "JWT",
	}, claims)

	authData := map[string]interface{}{"publicKey": pubPEM}
	err := verifyJWT(tok, authData)
	if err == nil || !strings.Contains(err.Error(), "not yet valid") {
		t.Fatalf("verifyJWT(not-yet-valid) = %v; want errJWTNotYetValid", err)
	}
}

func TestVerifyJWT_BadSignature(t *testing.T) {
	_, priv := initSharedKey(t)

	now := time.Now().Unix()
	claims := map[string]interface{}{"sub": "x", "exp": now + 60}
	tok := signTestJWT(t, "RS256", priv, "", map[string]interface{}{
		"alg": "RS256", "typ": "JWT",
	}, claims)

	// Verify against a DIFFERENT keypair; signature won't match.
	wrongPubPEM, _ := initWrongKey(t)
	authData := map[string]interface{}{"publicKey": wrongPubPEM}
	if err := verifyJWT(tok, authData); err == nil {
		t.Fatal("verifyJWT(bad signature) = nil; expected error")
	}
}

func TestVerifyJWT_Malformed(t *testing.T) {
	cases := []string{
		"",
		"not.a.jwt",
		"only.two", // 2 parts
		"a.b.c.d",  // 4 parts
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if err := verifyJWT(c, map[string]interface{}{}); err == nil {
				t.Fatalf("verifyJWT(%q) = nil; want error", c)
			}
		})
	}
}

func TestParseRSAPublicKey_AcceptsN8NPaddedPEM(t *testing.T) {
	pubPEM, _ := initSharedKey(t)
	// n8n prepends "=" to the PEM body on copy-paste. The internal
	// representation still has newlines — only the leading "=" is
	// extra. We strip that single leading "=" and verify parsing
	// succeeds against the multi-line PEM (which is what n8n's
	// credential storage actually emits).
	withPad := "=" + pubPEM
	if _, err := parseRSAPublicKey(withPad); err != nil {
		t.Fatalf("parseRSAPublicKey with leading = : %v", err)
	}
}

func TestParseRSAPublicKey_RejectsGarbage(t *testing.T) {
	if _, err := parseRSAPublicKey("not a pem at all"); err == nil {
		t.Fatal("parseRSAPublicKey(garbage) = nil; expected error")
	}
	if _, err := parseRSAPublicKey(""); err == nil {
		t.Fatal("parseRSAPublicKey(empty) = nil; expected error")
	}
}
