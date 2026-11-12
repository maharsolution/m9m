package credentials

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// TestEncryptDecryptCredentialData_RoundTrip is the cipher round-trip
// the export/import path relies on: encrypt a credential data map and
// decrypt it again - the original document must come back unchanged.
func TestEncryptDecryptCredentialData_RoundTrip(t *testing.T) {
	cipher := NewN8NCipher("Default_Secure_Key_Change_Me_!!!")

	cases := []map[string]interface{}{
		{},
		{"password": "admin123"},
		{"brokers": "broker1:9092,broker2:9092", "username": "svc", "password": "s3cret"},
		{"connectionString": "mongodb://user:pass@host:27017", "database": "billing"},
		// A nested object (OAuth token data) must survive the JSON
		// round-trip as well.
		{"oauthTokenData": map[string]interface{}{"access_token": "token-value"}},
	}

	for _, data := range cases {
		blob, err := EncryptCredentialData(cipher, data)
		if err != nil {
			t.Fatalf("EncryptCredentialData failed: %v", err)
		}
		if blob == "" {
			t.Fatal("EncryptCredentialData returned an empty blob")
		}

		// The blob must be the n8n "Salted__" envelope, base64 encoded.
		raw, decodeErr := base64.StdEncoding.DecodeString(blob)
		if decodeErr != nil {
			t.Fatalf("the blob is not base64: %v", decodeErr)
		}
		if len(raw) < 16 || string(raw[:8]) != "Salted__" {
			t.Fatalf("the blob is not an n8n Salted__ envelope")
		}

		back, err := DecryptCredentialData(cipher, blob)
		if err != nil {
			t.Fatalf("DecryptCredentialData failed: %v", err)
		}
		if len(back) != len(data) {
			t.Fatalf("round-trip lost fields: got %d want %d", len(back), len(data))
		}
		for key, want := range data {
			got, ok := back[key]
			if !ok {
				t.Fatalf("round-trip lost the field %q", key)
			}
			if nested, isMap := want.(map[string]interface{}); isMap {
				gotMap, gotIsMap := got.(map[string]interface{})
				if !gotIsMap {
					t.Fatalf("the field %q changed its type", key)
				}
				for nestedKey, nestedValue := range nested {
					if gotMap[nestedKey] != nestedValue {
						t.Fatalf("the nested field %q changed", nestedKey)
					}
				}
				continue
			}
			if got != want {
				t.Fatalf("the field %q changed: got %v want %v", key, got, want)
			}
		}
	}
}

// TestDecryptCredentialData_N8NKnownVector decrypts a credential blob
// that was extracted live from an n8n database - the interoperability
// canary of the export format.
func TestDecryptCredentialData_N8NKnownVector(t *testing.T) {
	const knownCiphertext = "U2FsdGVkX1+GCnOeCceRL5YG3P2WdgSi5o3aCVTCtvAkl4RUIyGFRw489kC2eCstTGSGeFxLwI3rGsJLBzC6ZA=="
	cipher := NewN8NCipher("Default_Secure_Key_Change_Me_!!!")

	data, err := DecryptCredentialData(cipher, knownCiphertext)
	if err != nil {
		t.Fatalf("DecryptCredentialData failed: %v", err)
	}
	if data["user"] != "admin" || data["password"] != "admin123" {
		t.Fatalf("unexpected decrypted payload: %v", data)
	}
}

// TestDecryptCredentialData_WrongKeyFails makes sure a blob of another
// instance is rejected instead of silently producing garbage.
func TestDecryptCredentialData_WrongKeyFails(t *testing.T) {
	blob, err := EncryptCredentialData(NewN8NCipher("Default_Secure_Key_Change_Me_!!!"),
		map[string]interface{}{"password": "admin123"})
	if err != nil {
		t.Fatalf("EncryptCredentialData failed: %v", err)
	}

	if _, err := DecryptCredentialData(NewN8NCipher("another-instance-key-0000000000000"), blob); err == nil {
		t.Fatal("decrypting with a foreign key must fail")
	}
}

// TestInstanceCipher_RequiresKey documents the behaviour of an
// instance that runs without N8N_ENCRYPTION_KEY: the export and import
// paths report an explicit error instead of writing an unreadable
// blob.
func TestInstanceCipher_RequiresKey(t *testing.T) {
	t.Setenv("N8N_ENCRYPTION_KEY", "")
	if _, err := InstanceCipher(); !errors.Is(err, ErrNoInstanceKey) {
		t.Fatalf("expected ErrNoInstanceKey, got %v", err)
	}

	t.Setenv("N8N_ENCRYPTION_KEY", "Default_Secure_Key_Change_Me_!!!")
	cipher, err := InstanceCipher()
	if err != nil {
		t.Fatalf("InstanceCipher failed: %v", err)
	}
	if cipher == nil {
		t.Fatal("InstanceCipher returned no cipher")
	}
}

// TestExportCredentialShape pins the n8n wire format of one exported
// credential: id, name, type and the encrypted data blob.
func TestExportCredentialShape(t *testing.T) {
	blob := "U2FsdGVkX1+GCnOeCceRL5YG3P2WdgSi5o3aCVTCtvAkl4RUIyGFRw489kC2eCstTGSGeFxLwI3rGsJLBzC6ZA=="
	exported := ExportCredential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: blob,
	}

	if exported.ID != "cred-1" || exported.Name != "MongoDB account" || exported.Type != "mongoDb" {
		t.Fatalf("unexpected export metadata: %+v", exported)
	}
	if !strings.HasPrefix(exported.Data, "U2FsdGVkX1") {
		t.Fatalf("the exported data is not an n8n blob: %q", exported.Data)
	}
}
