package webhooks

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// parseRSAPublicKey accepts a PEM-encoded RSA public key. n8n
// stores credentials with leading "=" padding (an artifact of its
// copy-paste UI), so we strip a leading "=" before attempting to
// decode. Both PKIX (`BEGIN PUBLIC KEY`) and PKCS1 (`BEGIN RSA
// PUBLIC KEY`) headers are recognised.
func parseRSAPublicKey(pemData string) (*rsa.PublicKey, error) {
	// Trim a single leading "=" if present — n8n's UI appends one
	// when copy-pasting PEM bodies, which breaks base64 decoding.
	if len(pemData) > 0 && pemData[0] == '=' {
		pemData = pemData[1:]
	}
	// PEM might come as a single line OR with newlines; normalise
	// by inserting newlines before each "-----BEGIN/END" marker
	// if the source was collapsed.
	if !containsNewline(pemData) {
		pemData = flattenPEM(pemData)
	}

	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("not a PEM-encoded key")
	}

	switch block.Type {
	case "PUBLIC KEY":
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKIX public key: %w", err)
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("PKIX key is not RSA")
		}
		return rsaPub, nil
	case "RSA PUBLIC KEY":
		pub, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS1 public key: %w", err)
		}
		return pub, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
}

func containsNewline(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return true
		}
	}
	return false
}

// flattenPEM normalises a PEM body that has been stripped of
// newlines so that `pem.Decode` can find the BEGIN/END blocks.
//
// PEM blocks always look like:
//
//	-----BEGIN <TYPE>-----   (line A)
//	<base64>                 (line B, may span multiple)
//	-----END <TYPE>-----     (line C)
//
// `pem.Decode` accepts ANY whitespace between lines but requires
// the BEGIN/END markers to start a fresh line. So we only need to
// inject a newline in front of each marker occurrence if the
// preceding character is not already whitespace.
//
// To avoid false positives (the marker `-----BEGIN` is a SUBSTRING
// of `-----BEGIN PUBLIC KEY-----`, but the BEGIN of the next line
// is a different marker), we anchor the insertion on the START of
// the multi-character markers ("-----BEGIN" and "-----END"), which
// appear at most twice in any well-formed PEM.
//
// We do a single forward scan, looking for those markers and
// inserting a newline if the previous char is not whitespace.
func flattenPEM(s string) string {
	const beginMarker = "-----BEGIN"
	const endMarker = "-----END"
	var b strings.Builder
	b.Grow(len(s) + 16)
	var lastChar byte = 0 // 0 = "nothing written yet"
	for i := 0; i < len(s); {
		if i+len(beginMarker) <= len(s) && s[i:i+len(beginMarker)] == beginMarker {
			if lastChar != 0 && lastChar != '\n' && lastChar != ' ' && lastChar != '\t' && lastChar != '\r' {
				b.WriteByte('\n')
				lastChar = '\n'
			}
			b.WriteString(beginMarker)
			lastChar = beginMarker[len(beginMarker)-1]
			i += len(beginMarker)
			continue
		}
		if i+len(endMarker) <= len(s) && s[i:i+len(endMarker)] == endMarker {
			if lastChar != 0 && lastChar != '\n' && lastChar != ' ' && lastChar != '\t' && lastChar != '\r' {
				b.WriteByte('\n')
				lastChar = '\n'
			}
			b.WriteString(endMarker)
			lastChar = endMarker[len(endMarker)-1]
			i += len(endMarker)
			continue
		}
		b.WriteByte(s[i])
		lastChar = s[i]
		i++
	}
	if lastChar != 0 && lastChar != '\n' {
		b.WriteByte('\n')
	}
	return b.String()
}

func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// (left in place intentionally — indexOf is small and may be useful
// for future PEM fixups; go vet won't complain about it because
// unused functions in Go files are not flagged.)
var _ = indexOf
