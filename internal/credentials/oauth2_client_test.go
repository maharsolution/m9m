package credentials

// Tests for BUG-SEC-2026-10-12-002 — production OAuth2 HTTP
// client (real RFC 6749 refresh-token grant). Uses an
// `httptest.NewServer` to stand in for the IdP so the test
// suite stays hermetic.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOAuth2Client_ExchangeRefreshToken_Success(t *testing.T) {
	var calls int32
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Errorf("Content-Type = %q, want form-urlencoded", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := r.FormValue("refresh_token"); got != "old-refresh" {
			t.Errorf("refresh_token = %q, want old-refresh", got)
		}
		if got := r.FormValue("client_id"); got != "cid" {
			t.Errorf("client_id = %q, want cid", got)
		}
		if got := r.FormValue("client_secret"); got != "csecret" {
			t.Errorf("client_secret = %q, want csecret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "new-access",
			"refresh_token": "new-refresh",
			"expires_in": 3600,
			"token_type": "Bearer"
		}`))
	}))
	defer idp.Close()

	client := NewOAuth2Client()
	resp, err := client.ExchangeRefreshToken("old-refresh", "cid", "csecret", idp.URL)
	if err != nil {
		t.Fatalf("ExchangeRefreshToken: %v", err)
	}
	if resp.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want new-access", resp.AccessToken)
	}
	if resp.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q, want new-refresh", resp.RefreshToken)
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn = %d, want 3600", resp.ExpiresIn)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("idp calls = %d, want 1", got)
	}
}

func TestOAuth2Client_RetriesOn5xx(t *testing.T) {
	var calls int32
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "temporarily_unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"recovered","expires_in":60}`))
	}))
	defer idp.Close()

	client := NewOAuth2Client()
	client.RetryBackoff = 5 * time.Millisecond
	resp, err := client.ExchangeRefreshToken("rt", "cid", "cs", idp.URL)
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if resp.AccessToken != "recovered" {
		t.Errorf("AccessToken = %q, want recovered", resp.AccessToken)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("idp calls = %d, want 3 (1 initial + 2 retries)", got)
	}
}

func TestOAuth2Client_NoRetryOn4xx(t *testing.T) {
	var calls int32
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token revoked"}`))
	}))
	defer idp.Close()

	client := NewOAuth2Client()
	_, err := client.ExchangeRefreshToken("revoked", "cid", "cs", idp.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsOAuth2HTTPError(err) {
		t.Errorf("expected ErrOAuth2HTTP, got %T", err)
	}
	httpErr := err.(*ErrOAuth2HTTP)
	if httpErr.Status != 400 {
		t.Errorf("Status = %d, want 400", httpErr.Status)
	}
	if httpErr.Code != "invalid_grant" {
		t.Errorf("Code = %q, want invalid_grant", httpErr.Code)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("idp calls = %d, want 1 (no retry on 4xx)", got)
	}
}

func TestOAuth2Client_EmptyAccessTokenRejected(t *testing.T) {
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"token_type":"Bearer","expires_in":3600}`))
	}))
	defer idp.Close()

	client := NewOAuth2Client()
	_, err := client.ExchangeRefreshToken("rt", "cid", "cs", idp.URL)
	if err == nil {
		t.Fatal("expected error on empty access_token, got nil")
	}
	if !IsOAuth2HTTPError(err) {
		t.Errorf("expected ErrOAuth2HTTP, got %T: %v", err, err)
	}
}

func TestOAuth2Client_TruncatedErrorBody(t *testing.T) {
	// IdP returns a 100 KB error description; the log line
	// must be capped.
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server_error","error_description":"` + strings.Repeat("x", 100*1024) + `"}`))
	}))
	defer idp.Close()

	client := NewOAuth2Client()
	_, err := client.ExchangeRefreshToken("rt", "cid", "cs", idp.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.HasSuffix(err.Error(), "...(truncated)") {
		t.Errorf("error not truncated: %q", err.Error())
	}
}

func TestOAuth2Client_InputValidation(t *testing.T) {
	client := NewOAuth2Client()
	cases := []struct {
		name        string
		rt, cid, cs, url string
	}{
		{"empty refresh token", "", "cid", "cs", "https://idp/token"},
		{"empty token URL", "rt", "cid", "cs", ""},
		{"empty client id", "rt", "", "cs", "https://idp/token"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := client.ExchangeRefreshToken(c.rt, c.cid, c.cs, c.url)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestOAuth2Client_NetworkErrorReturnsWrapped(t *testing.T) {
	// Point at a closed port so the connection fails immediately.
	client := NewOAuth2Client()
	client.MaxRetries = 0
	_, err := client.ExchangeRefreshToken("rt", "cid", "cs", "http://127.0.0.1:1/token")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	// Wrapped, not raw — `errors.Is` would not unwrap to
	// `io.EOF`, but the chain must include "oauth2:".
	if !strings.Contains(err.Error(), "oauth2:") {
		t.Errorf("error chain missing 'oauth2:' prefix: %v", err)
	}
}

func TestOAuth2Client_RealRoundtrip(t *testing.T) {
	// The full rotation round-trip: a fake IdP, an
	// `OAuth2Credentials` with a stale state, and the
	// production `Refresh` method (which calls into the
	// client). Mirrors what an executor does at request time.
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"new","expires_in":7200,"token_type":"Bearer"}`))
	}))
	defer idp.Close()

	cred := &OAuth2Credentials{
		ClientID:       "cid",
		ClientSecret:   "cs",
		AccessTokenURL: idp.URL,
		RefreshToken:   "old",
		AccessToken:    "stale",
		raw:            map[string]interface{}{},
	}
	if err := cred.Refresh(NewOAuth2Client()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if cred.AccessToken != "new" {
		t.Errorf("AccessToken = %q, want new", cred.AccessToken)
	}
	if cred.NeedsRefresh(time.Minute) {
		t.Errorf("expected fresh token after rotation")
	}
	// Persisted back into the raw map.
	if cred.raw["accessToken"] != "new" {
		t.Errorf("raw[accessToken] = %v, want new", cred.raw["accessToken"])
	}
}

// fakeHTTPClient returns canned responses. Used for tests
// that need to drive specific wire shapes without standing
// up a real httptest server.
type fakeHTTPClient struct {
	resp *http.Response
	err  error
}

func (f *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f.resp, f.err
}

// errReader returns an error on Read — used to exercise the
// "read body" error path.
type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errors.New("simulated read error") }
func (errReader) Close() error               { return nil }

var _ = io.Reader(errReader{})
