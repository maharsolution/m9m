package credentials

import (
	"encoding/base64"
	"testing"
)

// TestN8NCipherRoundTrip ensures an arbitrary plaintext can be encrypted
// and then decrypted back to its original bytes.
func TestN8NCipherRoundTrip(t *testing.T) {
	cases := []string{
		"",
		"admin",
		`{"user":"admin","password":"admin123"}`,
		"long payload that exercises AES block alignment " +
			"----------------------------------------------------------------------",
		"x",   // 1 byte — PKCS#7 should still produce a full block
		"0123456789abcdef", // exactly 16 bytes (one block)
	}
	c := NewN8NCipher("Default_Secure_Key_Change_Me_!!!")
	for _, pt := range cases {
		got, err := c.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q) failed: %v", pt, err)
		}
		back, err := c.Decrypt(got)
		if err != nil {
			t.Fatalf("Decrypt round-trip failed for %q: %v", pt, err)
		}
		if back != pt {
			t.Fatalf("Round-trip mismatch: got %q want %q", back, pt)
		}
	}
}

// TestN8NCipherMatchesKnownVector is a byte-level vector check against
// an n8n-encrypted "basic_credential" payload that was extracted live
// from the n8n Postgres DB on 2026-09-09. If this ever breaks, the
// credential bridge to n8n has been broken too — this is the canary.
func TestN8NCipherMatchesKnownVector(t *testing.T) {
	const knownCiphertext = "U2FsdGVkX1+GCnOeCceRL5YG3P2WdgSi5o3aCVTCtvAkl4RUIyGFRw489kC2eCstTGSGeFxLwI3rGsJLBzC6ZA=="
	const knownPassword = "Default_Secure_Key_Change_Me_!!!" // 32 chars; n8n's actual key
	const expectedPlaintext = `{"user":"admin","password":"admin123"}`

	c := NewN8NCipher(knownPassword)
	got, err := c.Decrypt(knownCiphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if got != expectedPlaintext {
		t.Fatalf("plaintext mismatch: got %q want %q", got, expectedPlaintext)
	}
}

// TestN8NCipherFormat ensures the on-wire format matches n8n's
// "Salted__" envelope exactly — required for round-trip with n8n's
// own storage format.
func TestN8NCipherFormat(t *testing.T) {
	c := NewN8NCipher("k")
	ct, err := c.Encrypt(`{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 16 {
		t.Fatalf("ciphertext too short: %d", len(raw))
	}
	if string(raw[:8]) != "Salted__" {
		t.Fatalf("missing Salted__ magic: %q", string(raw[:8]))
	}
	// 8 bytes magic + 8 bytes salt + AES(plaintext=10B → 16B padded) = 32B ciphertext
	if len(raw) != 32 {
		t.Fatalf("unexpected ciphertext length: %d (want 32)", len(raw))
	}
}
