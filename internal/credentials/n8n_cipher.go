/*
Package credentials — n8n-compatible AES-256-CBC cipher.

Implements the exact algorithm n8n uses in
`packages/core/src/encryption/aes-256-cbc.ts` (CipherAes256CBC) so that
m9m can decrypt n8n's `credentials_entity.data` column and re-encrypt to
the same on-disk format. This is the OpenSSL "Salted__" envelope:

	ciphertext = base64("Salted__" || 8-byte salt || AES-256-CBC(plaintext, K, IV))

Where K (32 bytes) and IV (16 bytes) are derived via EVP_BytesToKey with
MD5 (the historical OpenSSL key-stretching function). n8n's source uses
Node's `crypto.createHash("md5")` and `createCipheriv("aes-256-cbc", ...)`
which is byte-identical to what this implementation produces.
*/
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// n8nSaltedMagic is the ASCII "Salted__" prefix (8 bytes). n8n writes
// `Buffer.from("53616c7465645f5f", "hex")` which decodes to this.
var n8nSaltedMagic = []byte("Salted__")

// N8NCipher implements n8n-compatible AES-256-CBC with EVP_BytesToKey(MD5)
// key derivation. It is intentionally side-effect-free: no environment
// lookups, no global state — pass the encryption key explicitly.
type N8NCipher struct {
	password string
}

// NewN8NCipher constructs a cipher using the given passphrase. The
// passphrase must match the one n8n was configured with (N8N_ENCRYPTION_KEY).
func NewN8NCipher(password string) *N8NCipher {
	return &N8NCipher{password: password}
}

// evpBytesToKey replicates OpenSSL's EVP_BytesToKey with MD5 and 1
// iteration — the same algorithm Node's crypto.createCipheriv used via
// legacy "salted" mode and what n8n's CipherAes256CBC.getKeyAndIv does:
//
//	password = key_bytes || salt
//	hash1    = MD5(password)
//	hash2    = MD5(hash1 || password)
//	iv       = MD5(hash2 || password)
//	key      = hash1 || hash2            // 32 bytes total (matches AES-256)
//
// This produces identical bytes to n8n's implementation for any
// (password, salt) pair.
//
// IMPORTANT: each MD5 input is built as a fresh concatenation so we never
// alias the underlying slice of `password` — Go's `append` will mutate
// the backing array in place if there is capacity to spare, which silently
// produces the wrong hash.
func evpBytesToKey(password, salt []byte) (key, iv []byte) {
	h1Input := make([]byte, 0, len(password)+len(salt))
	h1Input = append(h1Input, password...)
	h1Input = append(h1Input, salt...)
	h1 := md5.Sum(h1Input)

	h2Input := make([]byte, 0, len(h1)+len(password)+len(salt))
	h2Input = append(h2Input, h1[:]...)
	h2Input = append(h2Input, password...)
	h2Input = append(h2Input, salt...)
	h2 := md5.Sum(h2Input)

	ivInput := make([]byte, 0, len(h2)+len(password)+len(salt))
	ivInput = append(ivInput, h2[:]...)
	ivInput = append(ivInput, password...)
	ivInput = append(ivInput, salt...)
	ivHash := md5.Sum(ivInput)

	key = make([]byte, 0, len(h1)+len(h2))
	key = append(key, h1[:]...)
	key = append(key, h2[:]...)
	return key, ivHash[:]
}

// Encrypt encrypts plaintext using n8n's exact algorithm. The returned
// string is base64 of:
//
//	"Salted__" || 8 random bytes || AES-256-CBC(plaintext, K, IV)
//
// with PKCS#7 padding.
func (c *N8NCipher) Encrypt(plaintext string) (string, error) {
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("n8n cipher: cannot generate salt: %w", err)
	}
	key, iv := evpBytesToKey([]byte(c.password), salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("n8n cipher: aes.NewCipher failed: %w", err)
	}
	bm := cipher.NewCBCEncrypter(block, iv)
	padded := pkcs7Pad([]byte(plaintext), block.BlockSize())
	ct := make([]byte, len(padded))
	bm.CryptBlocks(ct, padded)

	out := make([]byte, 0, len(n8nSaltedMagic)+len(salt)+len(ct))
	out = append(out, n8nSaltedMagic...)
	out = append(out, salt...)
	out = append(out, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt decrypts an n8n-encrypted credential payload back into the
// original plaintext string.
func (c *N8NCipher) Decrypt(ciphertextB64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("n8n cipher: base64 decode failed: %w", err)
	}
	if len(raw) < len(n8nSaltedMagic)+8+16 {
		return "", errors.New("n8n cipher: ciphertext too short")
	}
	if string(raw[:8]) != string(n8nSaltedMagic) {
		return "", errors.New("n8n cipher: missing Salted__ magic")
	}
	salt := raw[8:16]
	ct := raw[16:]

	key, iv := evpBytesToKey([]byte(c.password), salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("n8n cipher: aes.NewCipher failed: %w", err)
	}
	bm := cipher.NewCBCDecrypter(block, iv)
	pt := make([]byte, len(ct))
	bm.CryptBlocks(pt, ct)
	pt, err = pkcs7Unpad(pt, block.BlockSize())
	if err != nil {
		return "", fmt.Errorf("n8n cipher: invalid PKCS#7 padding: %w", err)
	}
	return string(pt), nil
}

// pkcs7Pad appends PKCS#7 padding so that the result is a multiple of
// blockSize. Always at least one byte of padding is added (RFC 5652 §6.3).
func pkcs7Pad(in []byte, blockSize int) []byte {
	pad := blockSize - (len(in) % blockSize)
	buf := make([]byte, len(in)+pad)
	copy(buf, in)
	for i := len(in); i < len(buf); i++ {
		buf[i] = byte(pad)
	}
	return buf
}

// pkcs7Unpad validates and strips PKCS#7 padding. Returns an error if the
// padding is malformed so we never accidentally accept a garbage block.
func pkcs7Unpad(in []byte, blockSize int) ([]byte, error) {
	if len(in) == 0 || len(in)%blockSize != 0 {
		return nil, errors.New("ciphertext length is not a multiple of block size")
	}
	pad := int(in[len(in)-1])
	if pad == 0 || pad > blockSize {
		return nil, errors.New("invalid padding byte")
	}
	for i := len(in) - pad; i < len(in); i++ {
		if int(in[i]) != pad {
			return nil, errors.New("non-uniform padding bytes")
		}
	}
	return in[:len(in)-pad], nil
}
