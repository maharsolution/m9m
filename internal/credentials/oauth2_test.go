package credentials

// Tests for BUG-SEC-2026-10-12-002 — OAuth2 credential surface
// and refresh-token rotation helper.

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeExchanger captures the last call and returns a canned
// response. Production code uses `internal/api/oauth2_client.go`
// (not yet landed — BUG-CAP-2026-10-12-005); for now the
// interface boundary lets us test rotation end-to-end.
type fakeExchanger struct {
	resp  *RefreshTokenResponse
	err   error
	calls int
	last  struct {
		Refresh, ClientID, ClientSecret, TokenURL string
	}
}

func (f *fakeExchanger) ExchangeRefreshToken(rt, cid, cs, url string) (*RefreshTokenResponse, error) {
	f.calls++
	f.last.Refresh = rt
	f.last.ClientID = cid
	f.last.ClientSecret = cs
	f.last.TokenURL = url
	return f.resp, f.err
}

func TestOAuth2_FromMap_RequiredFields(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]interface{}
		want string
	}{
		{
			name: "missing clientId",
			m: map[string]interface{}{
				"accessTokenUrl": "https://idp/token",
			},
			want: "clientId",
		},
		{
			name: "missing accessTokenUrl",
			m: map[string]interface{}{
				"clientId": "abc",
			},
			want: "accessTokenUrl",
		},
		{
			name: "all required set",
			m: map[string]interface{}{
				"clientId":       "abc",
				"accessTokenUrl": "https://idp/token",
			},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := &OAuth2Credentials{}
			err := o.FromMap(c.m)
			if c.want == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error mentioning %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected error mentioning %q, got %q", c.want, err.Error())
			}
		})
	}
}

func TestOAuth2_NeedsRefresh(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		o     OAuth2Credentials
		skew  time.Duration
		want  bool
	}{
		{name: "no access token", o: OAuth2Credentials{RefreshToken: "r"}, skew: 60 * time.Second, want: true},
		{name: "fresh token, far expiry", o: OAuth2Credentials{AccessToken: "x", ExpiresAt: now.Add(time.Hour)}, skew: 60 * time.Second, want: false},
		{name: "expired token", o: OAuth2Credentials{AccessToken: "x", ExpiresAt: now.Add(-time.Minute)}, skew: 60 * time.Second, want: true},
		{name: "within skew window", o: OAuth2Credentials{AccessToken: "x", ExpiresAt: now.Add(30 * time.Second)}, skew: 60 * time.Second, want: true},
		{name: "no expiry set", o: OAuth2Credentials{AccessToken: "x"}, skew: 60 * time.Second, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.o.NeedsRefresh(c.skew); got != c.want {
				t.Errorf("NeedsRefresh = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOAuth2_Refresh_RotatesToken(t *testing.T) {
	o := &OAuth2Credentials{
		ClientID:       "client-1",
		ClientSecret:   "secret-1",
		AccessTokenURL: "https://idp.example/oauth/token",
		RefreshToken:   "old-refresh",
		AccessToken:    "old-access",
		ExpiresAt:      time.Now().Add(-time.Hour),
		raw:            map[string]interface{}{"clientId": "client-1"},
	}
	ex := &fakeExchanger{resp: &RefreshTokenResponse{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresIn:    3600,
		TokenType:    "Bearer",
	}}
	if err := o.Refresh(ex); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if o.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want new-access", o.AccessToken)
	}
	if o.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q, want new-refresh", o.RefreshToken)
	}
	if o.NeedsRefresh(time.Minute) {
		t.Errorf("token should not need refresh after rotation")
	}
	// Verify the exchanger got the right values.
	if ex.calls != 1 {
		t.Errorf("exchanger calls = %d, want 1", ex.calls)
	}
	if ex.last.Refresh != "old-refresh" {
		t.Errorf("exchanger got refresh=%q, want old-refresh", ex.last.Refresh)
	}
	if ex.last.ClientID != "client-1" || ex.last.ClientSecret != "secret-1" {
		t.Errorf("exchanger got clientId/secret = %q/%q", ex.last.ClientID, ex.last.ClientSecret)
	}
	parsed, _ := url.Parse(ex.last.TokenURL)
	if parsed == nil || parsed.Host != "idp.example" {
		t.Errorf("exchanger got tokenURL=%q", ex.last.TokenURL)
	}
}

func TestOAuth2_Refresh_PreservesRefreshTokenWhenIdPDoesntRotate(t *testing.T) {
	// Some IdPs (Slack, older GitHub Apps) return the same
	// refresh token. We must keep the old one in that case.
	o := &OAuth2Credentials{
		ClientID: "c", ClientSecret: "s",
		AccessTokenURL: "https://idp/token",
		RefreshToken:   "keep-me",
		AccessToken:    "old",
		raw:            map[string]interface{}{},
	}
	ex := &fakeExchanger{resp: &RefreshTokenResponse{
		AccessToken: "new",
		ExpiresIn:   3600,
	}}
	if err := o.Refresh(ex); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if o.RefreshToken != "keep-me" {
		t.Errorf("RefreshToken = %q, want keep-me", o.RefreshToken)
	}
}

func TestOAuth2_Refresh_RequiresRefreshToken(t *testing.T) {
	o := &OAuth2Credentials{
		ClientID: "c", ClientSecret: "s",
		AccessTokenURL: "https://idp/token",
		raw:            map[string]interface{}{},
	}
	if err := o.Refresh(&fakeExchanger{}); err == nil {
		t.Fatal("expected error when no refresh token, got nil")
	}
}

func TestOAuth2_Refresh_NilExchanger(t *testing.T) {
	o := &OAuth2Credentials{RefreshToken: "r"}
	if err := o.Refresh(nil); err == nil {
		t.Fatal("expected error with nil exchanger, got nil")
	}
}

func TestOAuth2_Refresh_ExchangerError(t *testing.T) {
	ex := &fakeExchanger{err: errors.New("idp 503")}
	o := &OAuth2Credentials{
		ClientID: "c", ClientSecret: "s",
		AccessTokenURL: "https://idp/token",
		RefreshToken:   "r",
		raw:            map[string]interface{}{},
	}
	if err := o.Refresh(ex); err == nil {
		t.Fatal("expected error from exchanger failure, got nil")
	}
}

func TestOAuth2_Refresh_EmptyAccessTokenRejected(t *testing.T) {
	ex := &fakeExchanger{resp: &RefreshTokenResponse{AccessToken: ""}}
	o := &OAuth2Credentials{
		ClientID: "c", ClientSecret: "s",
		AccessTokenURL: "https://idp/token",
		RefreshToken:   "r",
		raw:            map[string]interface{}{},
	}
	if err := o.Refresh(ex); err == nil {
		t.Fatal("expected error on empty access_token, got nil")
	}
}

func TestOAuth2_Schema_HasN8nFieldNames(t *testing.T) {
	// The schema MUST keep the exact n8n field names so
	// exported workflow JSON round-trips. If a future refactor
	// renames `clientId` → `client_id` the migration will
	// silently break every existing workflow.
	want := map[string]bool{
		"clientId":        false,
		"clientSecret":    false,
		"authUrl":         false,
		"accessTokenUrl":  false,
		"scope":           false,
		"accessToken":     false,
		"refreshToken":    false,
		"expiresIn":       false,
		"grantType":       false,
		"authQueryParameters": false,
	}
	for _, f := range OAuth2Schema.Properties {
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("OAuth2Schema missing n8n field %q", name)
		}
	}
	if OAuth2Schema.Type != "oauth2Api" {
		t.Errorf("OAuth2Schema.Type = %q, want oauth2Api", OAuth2Schema.Type)
	}
}

func TestOAuth2_ToMap_RoundTrip(t *testing.T) {
	raw := map[string]interface{}{
		"clientId":        "c",
		"clientSecret":    "s",
		"authUrl":         "https://idp/auth",
		"accessTokenUrl":  "https://idp/token",
		"scope":           "read",
		"grantType":       "authorizationCode",
		"authQueryParameters": "audience=https://api",
	}
	o := &OAuth2Credentials{}
	if err := o.FromMap(raw); err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	round := o.ToMap()
	for k, v := range raw {
		if round[k] != v {
			t.Errorf("ToMap[%q] = %v, want %v", k, round[k], v)
		}
	}
}
