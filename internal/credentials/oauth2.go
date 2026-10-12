package credentials

// OAuth2 credential surface for m9m (BUG-SEC-2026-10-12-002).
//
// n8n ships a generic `oauth2Api` credential type that any node
// can opt into — Slack, Google Sheets, GitHub, HubSpot, the
// works. The credential stores `{ clientId, clientSecret,
// accessToken, refreshToken, expiresIn, scope, authUrl, accessTokenUrl,
// grantType }`. At request time the node reaches for `oauth2Api`,
// m9m sees the access token is within 60s of expiry, swaps it for
// a fresh one using the refresh token, and injects the new pair
// back into the encrypted store.
//
// This file provides the rotation helper, the n8n-style schema
// definition (so the UI can render the form), and the typed
// errors. It does NOT itself perform HTTP — the caller (the
// node, or a shared `OAuth2Client`) supplies a `TokenExchanger`
// interface. Keeping I/O out of this file makes it trivially
// testable in isolation.

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// OAuth2Schema is the n8n-style credential schema definition
// used by the m8m UI to render the form. n8n ships exactly
// these field names; the frontend maps them 1:1.
var OAuth2Schema = CredentialSchema{
	Type:    "oauth2Api",
	Display: "OAuth2 API",
	Properties: []CredentialSchemaField{
		{Name: "clientId", Display: "Client ID", Type: "string", Required: true},
		{Name: "clientSecret", Display: "Client Secret", Type: "password", Required: true},
		{Name: "authUrl", Display: "Authorization URL", Type: "string", Required: true, Placeholder: "https://example.com/oauth/authorize"},
		{Name: "accessTokenUrl", Display: "Access Token URL", Type: "string", Required: true, Placeholder: "https://example.com/oauth/token"},
		{Name: "scope", Display: "Scope", Type: "string", Required: false},
		{Name: "authQueryParameters", Display: "Additional Auth Query Parameters", Type: "string", Required: false},
		{Name: "accessToken", Display: "Access Token", Type: "password", Required: false},
		{Name: "refreshToken", Display: "Refresh Token", Type: "password", Required: false},
		{Name: "expiresIn", Display: "Expires In (s, from issuer)", Type: "number", Required: false},
		{Name: "grantType", Display: "Grant Type", Type: "options", Options: []string{"authorizationCode", "clientCredentials", "pkce"}, Required: false, Default: "authorizationCode"},
	},
}

// OAuth2Credentials is the in-memory shape of an `oauth2Api`
// credential after `Resolve`. Field names mirror the schema
// above and the keys the wire format uses.
type OAuth2Credentials struct {
	ClientID         string
	ClientSecret     string
	AuthURL          string
	AccessTokenURL   string
	Scope            string
	AccessToken      string
	RefreshToken     string
	ExpiresAt        time.Time
	GrantType        string
	AuthQueryParams  string
	raw              map[string]interface{}
}

// FromMap decodes the raw credential Data map (as persisted by
// `CredentialStore`) into a typed `OAuth2Credentials` struct.
// Unknown fields are kept in `raw` so a future schema bump
// doesn't silently drop them.
func (o *OAuth2Credentials) FromMap(m map[string]interface{}) error {
	if m == nil {
		return errors.New("oauth2: nil credential data")
	}
	o.raw = m
	if v, ok := m["clientId"].(string); ok {
		o.ClientID = v
	}
	if v, ok := m["clientSecret"].(string); ok {
		o.ClientSecret = v
	}
	if v, ok := m["authUrl"].(string); ok {
		o.AuthURL = v
	}
	if v, ok := m["accessTokenUrl"].(string); ok {
		o.AccessTokenURL = v
	}
	if v, ok := m["scope"].(string); ok {
		o.Scope = v
	}
	if v, ok := m["accessToken"].(string); ok {
		o.AccessToken = v
	}
	if v, ok := m["refreshToken"].(string); ok {
		o.RefreshToken = v
	}
	if v, ok := m["grantType"].(string); ok {
		o.GrantType = v
	} else {
		o.GrantType = "authorizationCode"
	}
	if v, ok := m["authQueryParameters"].(string); ok {
		o.AuthQueryParams = v
	}
	switch exp := m["expiresIn"].(type) {
	case float64:
		o.ExpiresAt = time.Now().Add(time.Duration(exp) * time.Second)
	case int:
		o.ExpiresAt = time.Now().Add(time.Duration(exp) * time.Second)
	}
	// Required-field gate. The UI is expected to enforce this
	// too, but we re-check at the credential-resolve boundary
	// so a malformed `n8n-export.json` never makes it to a
	// node's HTTP layer.
	if strings.TrimSpace(o.ClientID) == "" {
		return errors.New("oauth2: clientId is required")
	}
	if strings.TrimSpace(o.AccessTokenURL) == "" {
		return errors.New("oauth2: accessTokenUrl is required")
	}
	return nil
}

// NeedsRefresh returns true when the access token is missing,
// empty, or within `skew` of expiry. Default skew should be
// 60s — enough to cover clock drift between m9m and the IdP.
func (o *OAuth2Credentials) NeedsRefresh(skew time.Duration) bool {
	if o.AccessToken == "" {
		return true
	}
	if o.ExpiresAt.IsZero() {
		return false
	}
	return time.Until(o.ExpiresAt) < skew
}

// CredentialSchema is a minimal local copy of the schema the
// `CredentialSchema` we don't have a separate type for in this
// package yet. The full schema lives in `internal/credentials/`
// already; this stub keeps the `oauth2.go` file decoupled and
// unit-testable. The web form builder maps these fields 1:1.
type CredentialSchema struct {
	Type       string                 `json:"type"`
	Display    string                 `json:"display"`
	Properties []CredentialSchemaField `json:"properties,omitempty"`
}

// CredentialSchemaField is the per-field definition rendered by
// the NDV / credential editor.
type CredentialSchemaField struct {
	Name        string   `json:"name"`
	Display     string   `json:"display"`
	Type        string   `json:"type"`
	Required    bool     `json:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// RefreshTokenResponse is the body the IdP returns on a
// successful refresh. m9m uses standard RFC 6749 field names.
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
}

// TokenExchanger is the boundary between the rotation logic and
// the network. The node (or a shared `OAuth2Client`) supplies an
// implementation; tests inject a fake. We accept an interface
// here so a future `internal/api/oauth2_client.go` can plug in
// retry / circuit-breaker / metrics without changing the
// rotation logic.
type TokenExchanger interface {
	ExchangeRefreshToken(refreshToken, clientID, clientSecret, tokenURL string) (*RefreshTokenResponse, error)
}

// Refresh rotates the access token using the refresh token, then
// mutates `o` in place and returns the new state. The caller is
// responsible for re-persisting the credential via the store
// (so the rotated pair is available to the next run / the
// next node).
//
// Pre-conditions:
//   - o.RefreshToken != "" (or `NeedsRefresh` returned true for
//     some other reason — e.g. a one-shot access token
//     consumed by `clientCredentials`).
//   - The `exchanger` implementation is configured with the
//     correct clientId/secret/URL (m9m passes them through).
func (o *OAuth2Credentials) Refresh(exchanger TokenExchanger) error {
	if exchanger == nil {
		return errors.New("oauth2: nil TokenExchanger")
	}
	if o.RefreshToken == "" {
		return errors.New("oauth2: no refresh token to rotate")
	}
	if o.AccessTokenURL == "" {
		return errors.New("oauth2: accessTokenUrl is required")
	}
	resp, err := exchanger.ExchangeRefreshToken(o.RefreshToken, o.ClientID, o.ClientSecret, o.AccessTokenURL)
	if err != nil {
		return fmt.Errorf("oauth2: refresh failed: %w", err)
	}
	if resp == nil || resp.AccessToken == "" {
		return errors.New("oauth2: refresh returned empty access_token")
	}
	o.AccessToken = resp.AccessToken
	if resp.RefreshToken != "" {
		// Most IdPs rotate the refresh token; some (Slack)
		// don't. We accept whatever the issuer hands back.
		o.RefreshToken = resp.RefreshToken
	}
	if resp.ExpiresIn > 0 {
		o.ExpiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	} else {
		// No expiry: assume the issuer's longest-lived token
		// class (1 hour) so `NeedsRefresh` stays sane.
		o.ExpiresAt = time.Now().Add(time.Hour)
	}
	// Persist the rotation back into the raw map so
	// `ToMap` round-trips cleanly.
	if o.raw != nil {
		o.raw["accessToken"] = o.AccessToken
		if o.RefreshToken != "" {
			o.raw["refreshToken"] = o.RefreshToken
		}
		o.raw["expiresIn"] = int(time.Until(o.ExpiresAt).Seconds())
	}
	return nil
}

// ToMap returns the credential as a generic map (the shape
// `CredentialStore` persists). After a successful `Refresh`,
// the map is up-to-date and ready to write back to disk.
func (o *OAuth2Credentials) ToMap() map[string]interface{} {
	if o.raw != nil {
		return o.raw
	}
	return map[string]interface{}{
		"clientId":        o.ClientID,
		"clientSecret":    o.ClientSecret,
		"authUrl":         o.AuthURL,
		"accessTokenUrl":  o.AccessTokenURL,
		"scope":           o.Scope,
		"accessToken":     o.AccessToken,
		"refreshToken":    o.RefreshToken,
		"grantType":       o.GrantType,
		"expiresIn":       int(time.Until(o.ExpiresAt).Seconds()),
		"authQueryParameters": o.AuthQueryParams,
	}
}
