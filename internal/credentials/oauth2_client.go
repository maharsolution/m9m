package credentials

// Real HTTP implementation of `TokenExchanger` for BUG-SEC-2026-10-12-002.
//
// `OAuth2Credentials.Refresh` (oauth2.go) takes a `TokenExchanger`
// interface so the rotation logic stays testable. This file is
// the production binding: it speaks RFC 6749 form-encoded
// `application/x-www-form-urlencoded` POSTs to the IdP's
// `/oauth/token` endpoint, with retries, timeouts, and a
// structured error path.
//
// We deliberately avoid `golang.org/x/oauth2` to keep the
// dependency surface small — the surface area we need is
// exactly the refresh-token grant, and pulling in `x/oauth2`
// would also pull its `golang.org/x/oauth2/jws` JWT handling
// which we don't need (m9m's own JWT layer is in
// `internal/webhooks/jwt.go`).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPClient is the minimal subset of `*http.Client` we depend on.
// Production code passes `http.DefaultClient` (with a Timeout
// configured) or a custom client; tests inject a fake.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// OAuth2Client is the production TokenExchanger.
type OAuth2Client struct {
	// HTTP is the HTTP client. Defaults to `&http.Client{
	// Timeout: 15*time.Second}` if nil. Production callers
	// should set a Timeout to avoid hanging the executor
	// goroutine on a slow IdP.
	HTTP HTTPClient

	// Timeout overrides the HTTP client's default timeout
	// per call. Zero means "use HTTP.Timeout".
	Timeout time.Duration

	// MaxRetries is the number of additional attempts on a
	// 5xx or network error. Zero = 1 attempt (no retry).
	MaxRetries int

	// RetryBackoff is the base backoff between retries.
	// Each retry doubles the wait (exp). Defaults to 200ms.
	RetryBackoff time.Duration
}

// NewOAuth2Client returns an OAuth2Client with sane defaults:
// 15s timeout, 2 retries (3 attempts total), 200ms backoff.
func NewOAuth2Client() *OAuth2Client {
	return &OAuth2Client{
		HTTP:         &http.Client{Timeout: 15 * time.Second},
		Timeout:      15 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 200 * time.Millisecond,
	}
}

// ErrOAuth2HTTP is returned when the IdP returns a non-2xx
// response. The `Status` and `Body` fields are populated for
// logging; `Code` is the RFC 6749 `error` field when present.
type ErrOAuth2HTTP struct {
	Status int
	Code   string
	Body   string
}

func (e *ErrOAuth2HTTP) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("oauth2: idp returned %d (%s): %s", e.Status, e.Code, truncateForLog(e.Body, 256))
	}
	return fmt.Sprintf("oauth2: idp returned %d: %s", e.Status, truncateForLog(e.Body, 256))
}

// IsOAuth2HTTPError reports whether err is an *ErrOAuth2HTTP.
func IsOAuth2HTTPError(err error) bool {
	_, ok := err.(*ErrOAuth2HTTP)
	return ok
}

// ExchangeRefreshToken implements the TokenExchanger contract.
// Sends a `grant_type=refresh_token` POST to `tokenURL` per
// RFC 6749 §6. The `clientId` / `clientSecret` are sent in the
// POST body (not in the `Authorization` header) because most
// IdPs (Slack, GitHub Apps, HubSpot) require the body form.
func (c *OAuth2Client) ExchangeRefreshToken(refreshToken, clientID, clientSecret, tokenURL string) (*RefreshTokenResponse, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("oauth2: refresh token is empty")
	}
	if strings.TrimSpace(tokenURL) == "" {
		return nil, errors.New("oauth2: token URL is empty")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, errors.New("oauth2: client ID is empty")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := c.RetryBackoff << (attempt - 1) // 200, 400, 800…
			time.Sleep(wait)
		}
		resp, err := c.do(tokenURL, form)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// Don't retry on 4xx — those are deterministic
		// (bad client_id, revoked refresh_token, etc.).
		var httpErr *ErrOAuth2HTTP
		if errors.As(err, &httpErr) && httpErr.Status >= 400 && httpErr.Status < 500 {
			return nil, err
		}
	}
	return nil, fmt.Errorf("oauth2: refresh failed after %d attempts: %w", c.MaxRetries+1, lastErr)
}

// do is the single-attempt HTTP call. Extracted so the retry
// loop stays a flat `for` over `do`.
func (c *OAuth2Client) do(tokenURL string, form url.Values) (*RefreshTokenResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeoutOrDefault())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("oauth2: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// Some IdPs reject requests without a UA. Real clients send
	// a UA; m9m follows suit.
	req.Header.Set("User-Agent", "m9m-oauth2/0.4.0")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: c.timeoutOrDefault()}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth2: http: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("oauth2: read body: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// RFC 6749 §5.2 says the error response is JSON with
		// `{"error": "...", "error_description": "..."}`. Some
		// IdPs (Slack) return form-encoded instead. Try JSON
		// first; fall back to the raw body.
		var oauthErr struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		code := ""
		if json.Unmarshal(body, &oauthErr) == nil && oauthErr.Error != "" {
			code = oauthErr.Error
		}
		return nil, &ErrOAuth2HTTP{Status: res.StatusCode, Code: code, Body: string(body)}
	}

	// Success — RFC 6749 §5.1: `access_token` required, the
	// rest optional. We accept the response in JSON form.
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token,omitempty"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope,omitempty"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("oauth2: parse response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, &ErrOAuth2HTTP{Status: res.StatusCode, Body: string(body)}
	}
	return &RefreshTokenResponse{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		TokenType:    tokenResp.TokenType,
		Scope:        tokenResp.Scope,
	}, nil
}

func (c *OAuth2Client) timeoutOrDefault() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	if httpClient, ok := c.HTTP.(*http.Client); ok && httpClient.Timeout > 0 {
		return httpClient.Timeout
	}
	return 15 * time.Second
}

// truncateForLog limits error-body size in the log line to
// avoid leaking a 100 KB error description into the executor
// log.
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
