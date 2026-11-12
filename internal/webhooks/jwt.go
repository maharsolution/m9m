package webhooks

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// jwtHeader is the minimal subset of the JOSE header m9m needs to
// dispatch verification. Other fields (`kid`, `jku`, etc.) are
// ignored — n8n's parity-test workflow uses static credentials.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// jwtClaims is the subset of RFC 7519 claims m9m checks. Anything
// else is preserved in the parsed payload so future code paths
// (e.g. role-based authorization) can read them without re-parsing.
type jwtClaims struct {
	Sub string      `json:"sub,omitempty"`
	Iss string      `json:"iss,omitempty"`
	Aud interface{} `json:"aud,omitempty"`
	Exp int64       `json:"exp,omitempty"`
	Nbf int64       `json:"nbf,omitempty"`
	Iat int64       `json:"iat,omitempty"`
	Jti string      `json:"jti,omitempty"`
}

// verifyJWT validates the signature and time-based claims of the
// supplied JWT against the credential stored on the Webhook node.
//
// `authData` is the same shape `resolveCredentialAuthData` produces
// for `jwtAuth` credentials (see internal/webhooks/manager.go):
//
//	{"algorithm": "RS256"|"HS256",
//	 "publicKey": "<PEM>",  // for RS256
//	 "secret":    "<secret>", // for HS256 (also used for HMAC
//	                          //  fallback when no public key is
//	                          //  configured)
//	 "keyType":   "pemKey"|"passphrase" }
//
// Errors are returned with enough detail for the handler to render
// an n8n-compatible wire response. In particular, `errJWTExpired`
// and `errJWTNotYetValid` are surfaced verbatim so the handler can
// return the same `403 jwt expired` body n8n sends.
func verifyJWT(token string, authData map[string]interface{}) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		// n8n (jsonwebtoken) rejects a token that does not have three
		// segments with "jwt malformed" — not with a signature error.
		return errJWTMalformed
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("malformed jwt header: %w", err)
	}
	var hdr jwtHeader
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return fmt.Errorf("malformed jwt header json: %w", err)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("malformed jwt payload: %w", err)
	}
	var claims jwtClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return fmt.Errorf("malformed jwt payload json: %w", err)
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("malformed jwt signature: %w", err)
	}

	// A token that declares the "none" algorithm must not carry a
	// signature. jsonwebtoken answers "jwt signature is required" for
	// that shape, and it checks it before the algorithm allow-list.
	if hdr.Alg == "none" && strings.TrimSpace(parts[2]) == "" {
		return errJWTSignatureRequired
	}

	// n8n passes the credential's algorithm to jsonwebtoken as the only
	// allowed one, so a token signed with another algorithm is rejected
	// with "invalid algorithm" — not with a signature error.
	if !jwtAlgorithmAllowed(authData, hdr.Alg) {
		return errJWTInvalidAlgorithm
	}

	// Pick verifier based on header alg. We mirror n8n's parity
	// behaviour: if the credential supplies both an RSA public key
	// and an HMAC secret, the header alg wins.
	switch hdr.Alg {
	case "RS256":
		pub := getAuthData(authData, "publicKey", "")
		if pub == "" {
			return errors.New("jwt auth: missing public key")
		}
		if err := verifyRS256(signingInput, signature, pub); err != nil {
			return err
		}
	case "HS256":
		secret := getAuthData(authData, "secret", "")
		if secret == "" {
			return errors.New("jwt auth: missing secret")
		}
		if err := verifyHS256(signingInput, signature, secret); err != nil {
			return err
		}
	default:
		return fmt.Errorf("jwt auth: unsupported alg %q", hdr.Alg)
	}

	// Time-based claims. n8n rejects expired tokens with HTTP 403
	// body "jwt expired"; m9m does the same by returning a sentinel
	// the handler maps to that wire shape.
	now := time.Now().Unix()
	if claims.Exp > 0 && now >= claims.Exp {
		return errJWTExpired
	}
	if claims.Nbf > 0 && now < claims.Nbf {
		return errJWTNotYetValid
	}

	return nil
}

// verifyRS256 parses the PEM-encoded public key and runs the
// signature check. PEM blocks other than PUBLIC KEY are rejected.
func verifyRS256(signingInput string, signature []byte, pem string) error {
	pub, err := parseRSAPublicKey(pem)
	if err != nil {
		return fmt.Errorf("jwt auth: invalid public key: %w", err)
	}
	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], signature); err != nil {
		return fmt.Errorf("jwt auth: signature verification failed: %w", err)
	}
	return nil
}

// verifyHS256 uses HMAC-SHA256 over the signing input and compares
// in constant time. Constant-time is the only safe option because
// the caller controls the signature bytes; any short-circuit on
// mismatch would leak timing information.
func verifyHS256(signingInput string, signature []byte, secret string) error {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)
	if hmac.Equal(signature, expected) {
		return nil
	}
	return errors.New("jwt auth: signature verification failed")
}

// Sentinel errors so the handler can map to n8n's wire shape.
//
// The bodies are n8n's verbatim. Verified live against
// http://187.77.113.218:5678/webhook/token_auth on 2026-11-12:
//
//	no bearer token            → 401 "No token provided"
//	token without 3 segments   → 403 "jwt malformed"
//	alg "none" without signature → 403 "jwt signature is required"
//	algorithm not allowed      → 403 "invalid algorithm"
//	signature mismatch        → 403 "invalid token"
//	expired                    → 403 "jwt expired"
//	not yet valid              → 403 "jwt not yet valid"
var (
	errJWTExpired           = errors.New("jwt expired")
	errJWTNotYetValid       = errors.New("jwt not yet valid")
	errJWTMalformed         = errors.New("jwt malformed")
	errJWTInvalidAlgorithm  = errors.New("invalid algorithm")
	errJWTSignatureRequired = errors.New("jwt signature is required")
)

// jwtAlgorithmAllowed reports whether the credential allows the
// algorithm named in the JWT header.
//
// The credential's `algorithm` field wins: n8n hands exactly that
// algorithm to jsonwebtoken, so a token signed with a different one is
// rejected with "invalid algorithm". When the credential does not name
// an algorithm the key material decides — a PEM public key means
// RS256, a shared secret means HS256.
func jwtAlgorithmAllowed(authData map[string]interface{}, alg string) bool {
	switch strings.ToUpper(strings.TrimSpace(getAuthData(authData, "algorithm", ""))) {
	case "RS256":
		return alg == "RS256"
	case "HS256":
		return alg == "HS256"
	}

	pub := getAuthData(authData, "publicKey", "")
	secret := getAuthData(authData, "secret", "")
	switch {
	case pub != "" && secret != "":
		return alg == "RS256" || alg == "HS256"
	case pub != "":
		return alg == "RS256"
	case secret != "":
		return alg == "HS256"
	default:
		// No credential material at all: keep the historic behaviour of
		// accepting the two algorithms m9m can verify.
		return alg == "RS256" || alg == "HS256"
	}
}
