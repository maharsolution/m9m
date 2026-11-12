/*
Package credentials — n8n-compatible credential export / import.

This file adds the two helpers the HTTP API needs for the n8n CLI
parity of `n8n export:workflow --separate` / `n8n import:credentials`:

  - ExportCredential is the n8n wire format of one credential,
  - EncryptCredentialData / DecryptCredentialData move a credential
    payload through the existing n8n cipher (n8n_cipher.go).

CIPHER FORMAT (exact, byte-compatible with n8n)
------------------------------------------------
`data` is the base64 encoding of the OpenSSL "Salted__" envelope:

	"Salted__"            8 bytes, ASCII magic
	|| salt               8 random bytes
	|| ciphertext         AES-256-CBC(PKCS#7(JSON(data)), key, iv)

The key (32 bytes) and the iv (16 bytes) are derived from the
instance passphrase with EVP_BytesToKey(MD5, 1 iteration) — the exact
function n8n uses in
`packages/core/src/encryption/aes-256-cbc.ts` (CipherAes256CBC).

The passphrase is the RAW value of the `N8N_ENCRYPTION_KEY` environment
variable — the same value n8n itself uses, so:

  - n8n can decrypt a blob produced by m9m (same key), and
  - an m9m instance configured with the same `N8N_ENCRYPTION_KEY` can
    decrypt a blob produced by n8n.

The plaintext that is encrypted is the JSON encoding of the credential
data map (the same document n8n stores in `credentials_entity.data`),
e.g. `{"password":"...","brokers":"..."}`.
*/
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrNoInstanceKey is returned when the instance encryption key is not
// configured. Credential export and import cannot work without it: the
// payload must be encrypted with the instance key (n8n semantics).
var ErrNoInstanceKey = errors.New("N8N_ENCRYPTION_KEY is not set")

// ExportCredential is one credential in the n8n export format.
//
// n8n writes the encrypted payload into `data`; the field carries the
// base64 "Salted__" envelope described at the top of this file.
type ExportCredential struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Data string `json:"data"`
}

// InstanceCipher returns the n8n-compatible cipher of this instance.
//
// The passphrase is the raw `N8N_ENCRYPTION_KEY` value (n8n semantics).
// An instance that runs without that variable cannot export or import
// credentials — the error is explicit so the caller can report it
// instead of writing an unreadable blob.
func InstanceCipher() (*N8NCipher, error) {
	key := os.Getenv("N8N_ENCRYPTION_KEY")
	if strings.TrimSpace(key) == "" {
		return nil, ErrNoInstanceKey
	}
	return NewN8NCipher(key), nil
}

// EncryptCredentialData encrypts a credential data map with the
// instance cipher and returns the base64 blob n8n stores in the
// `data` column.
func EncryptCredentialData(cipher *N8NCipher, data map[string]interface{}) (string, error) {
	if cipher == nil {
		return "", errors.New("credentials: no instance cipher configured")
	}
	if data == nil {
		data = map[string]interface{}{}
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("credentials: failed to encode the credential data: %w", err)
	}

	blob, err := cipher.Encrypt(string(payload))
	if err != nil {
		return "", fmt.Errorf("credentials: failed to encrypt the credential data: %w", err)
	}
	return blob, nil
}

// DecryptCredentialData decrypts an n8n credential blob and returns the
// credential data map it carries.
func DecryptCredentialData(cipher *N8NCipher, blob string) (map[string]interface{}, error) {
	if cipher == nil {
		return nil, errors.New("credentials: no instance cipher configured")
	}

	plaintext, err := cipher.Decrypt(blob)
	if err != nil {
		return nil, fmt.Errorf("credentials: failed to decrypt the credential data: %w", err)
	}

	data := map[string]interface{}{}
	if err := json.Unmarshal([]byte(plaintext), &data); err != nil {
		return nil, fmt.Errorf("credentials: failed to decode the credential data: %w", err)
	}
	return data, nil
}
