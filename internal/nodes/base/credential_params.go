/*
Package base provides the base interfaces and types for node implementations.

This file holds the shared helpers for reading credential material out of
the node parameter map.
*/
package base

import (
	"strconv"
	"strings"
)

// CredentialString returns the value of the first key in `keys` that is
// present in `params` and carries a non-empty string.
//
// Why this helper exists: the credential manager flattens a node's
// credential into the node parameter map under keys shaped
// `<credentialType>_<field>` (see internal/credentials/manager.go), e.g.
// the OpenAI credential lands in `openai_apiKey`. A node that only reads
// the legacy plain name (`apiKey`) therefore never sees the credential
// and the editor's credential picker is purely cosmetic. Reading through
// this helper lets a node accept the envelope key first and keep the
// legacy plain key as a fallback, so workflows that inline the secret
// directly into `parameters` keep working.
//
// Contract:
//   - a nil map is safe and yields "";
//   - a key whose value is not a string is ignored — never coerced, so a
//     number can never leak into an Authorization header;
//   - a key whose value is an empty or whitespace-only string counts as
//     absent, so the next fallback key wins;
//   - the returned value is the raw string, byte for byte, so a secret is
//     never silently rewritten.
//
// The helper never logs and never formats the value into an error.
func CredentialString(params map[string]interface{}, keys ...string) string {
	if params == nil {
		return ""
	}

	for _, key := range keys {
		if key == "" {
			continue
		}
		raw, ok := params[key]
		if !ok || raw == nil {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		return value
	}

	return ""
}

// CredentialBool returns the boolean value of the first key in `keys`
// that is present in `params` and carries a boolean-ish value.
//
// The credential envelope always carries strings (the credential manager
// stringifies every field), so "true", "1", "yes" and "on" are read as
// true and "false", "0", "no" and "off" as false. A real bool value is
// taken as-is.
//
// Like CredentialString, an empty / whitespace-only string counts as
// absent so the next fallback key can win, and a nil map is safe.
func CredentialBool(params map[string]interface{}, keys ...string) bool {
	if params == nil {
		return false
	}

	for _, key := range keys {
		if key == "" {
			continue
		}
		raw, ok := params[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case bool:
			return value
		case string:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "true", "1", "yes", "on":
				return true
			case "false", "0", "no", "off":
				return false
			}
		}
	}

	return false
}

// CredentialInt returns the numeric value of the first key in `keys` that
// is present in `params` and carries a number, falling back to
// `fallback` when no key matches.
//
// The credential envelope stringifies every field, so `redis_port`
// arrives as "6379"; a number stored directly in the workflow JSON
// arrives as int, int32, int64 or float64. All of those shapes are
// accepted, and a numeric string is parsed.
//
// Zero is a legitimate value (Redis database index 0), so a numeric zero
// is returned as-is rather than treated as absent. Only a missing key, a
// non-numeric value or an empty string counts as absent.
func CredentialInt(params map[string]interface{}, fallback int, keys ...string) int {
	if params == nil {
		return fallback
	}

	for _, key := range keys {
		if key == "" {
			continue
		}
		raw, ok := params[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case int:
			return value
		case int32:
			return int(value)
		case int64:
			return int(value)
		case float64:
			return int(value)
		case string:
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			parsed, err := strconv.Atoi(trimmed)
			if err != nil {
				continue
			}
			return parsed
		}
	}

	return fallback
}
