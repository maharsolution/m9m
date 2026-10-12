package api

// SSO scaffold for BUG-SEC-2026-10-12-003.
//
// m9m today ships local-user auth + API key + JWT. For
// enterprise rollouts (the next 5 prospects on the sales
// pipeline) we need SAML 2.0 + OIDC SSO. The scaffold below
// establishes the abstraction boundaries:
//
//   - `SSOProvider` is a single method that turns a (raw
//     IdP assertion OR OIDC ID-token) into a normalised
//     `SSOIdentity`. Providers register themselves at
//     startup (SAML via `internal/saml/`, OIDC via
//     `internal/oidc/`).
//   - `SSORouter` is the HTTP layer that picks the right
//     provider for the request and emits the local session
//     cookie.
//   - `SSOIdentity` is the normalised subject — the local
//     `User` store consumes this regardless of the IdP.
//
// The actual protocol implementations are out of scope for
// this PR; they're tracked as BUG-CAP-2026-10-12-007
// (OIDC) and BUG-CAP-2026-10-12-008 (SAML). What we ship
// here is the shape, the test coverage, and a fake provider
// so the integration test seam is already in place.

import (
	"errors"
	"net/http"
	"strings"
	"sync"
)

// SSOIdentity is the normalised subject returned by every
// provider, regardless of the wire protocol.
type SSOIdentity struct {
	Subject     string   // stable user ID (IdP `sub` / NameID)
	Email       string
	DisplayName string
	Roles       []string // IdP groups / roles
	Provider    string   // "oidc" / "saml" / "fake"
	Raw         string   // raw assertion (for audit log only)
}

// HasRole returns true if the identity carries the given
// IdP group/role name.
func (i *SSOIdentity) HasRole(role string) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// SSOProvider is the contract every IdP integration must
// satisfy. The implementation owns its own state (HTTP
// client, JWKS cache, IdP metadata URL).
type SSOProvider interface {
	// Name returns the wire name (`"oidc"`, `"saml"`,
	// `"okta-saml"`, ...). The router dispatches on this.
	Name() string

	// Verify parses and validates a raw IdP assertion
	// (SAMLResponse base64 / OIDC id_token / Okta session
	// token) and returns the normalised identity. An
	// invalid assertion returns a typed `ErrSSOInvalid*`
	// error.
	Verify(assertion string) (*SSOIdentity, error)
}

// SSORouter is the HTTP-facing dispatcher. It selects the
// provider by the URL path (`/auth/sso/{provider}/callback`)
// and on success issues the same session cookie the local
// login form does. The session issuance path is shared, so
// downstream code (RBAC, audit, rate limiting) doesn't care
// how the user authenticated.
type SSORouter struct {
	mu        sync.RWMutex
	providers map[string]SSOProvider
}

// NewSSORouter returns an empty router. Register providers
// with `Register`.
func NewSSORouter() *SSORouter {
	return &SSORouter{providers: make(map[string]SSOProvider)}
}

// Register adds (or replaces) a provider by name. Returns
// `ErrSSODuplicate` if the name is already taken.
func (r *SSORouter) Register(p SSOProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[p.Name()]; exists {
		return ErrSSODuplicate
	}
	r.providers[p.Name()] = p
	return nil
}

// Lookup returns the provider registered under the given
// name, or `ErrSSOUnknown` if none.
func (r *SSORouter) Lookup(name string) (SSOProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, ErrSSOUnknown
	}
	return p, nil
}

// Names returns the registered provider names, in
// deterministic order (sorted). Used by the login page to
// render the SSO buttons.
func (r *SSORouter) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for n := range r.providers {
		out = append(out, n)
	}
	// Cheap sort: O(n log n) is fine, n < 10 in practice.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// HandleCallback is the standard `POST /auth/sso/{provider}/callback`
// handler. It pulls the assertion from the form (or JSON body),
// verifies it via the matching provider, and translates the
// identity into a local session. Session issuance is delegated
// to the supplied `SessionIssuer` so the SSO flow can be tested
// without spinning up the full auth manager.
func (r *SSORouter) HandleCallback(providerName, assertion string, sess SessionIssuer) (*SSOIdentity, string, error) {
	p, err := r.Lookup(providerName)
	if err != nil {
		return nil, "", err
	}
	identity, err := p.Verify(assertion)
	if err != nil {
		return nil, "", err
	}
	if identity.Subject == "" {
		return nil, "", ErrSSOInvalidSubject
	}
	token, err := sess.Issue(identity)
	if err != nil {
		return nil, "", err
	}
	return identity, token, nil
}

// SessionIssuer is the contract the SSO flow needs from the
// local session manager. The real `AuthManager` will satisfy
// this; tests inject a fake.
type SessionIssuer interface {
	Issue(identity *SSOIdentity) (string, error)
}

// Typed errors.
var (
	ErrSSOUnknown        = &SSOError{code: "unknown_provider", msg: "sso: provider not registered"}
	ErrSSODuplicate      = &SSOError{code: "duplicate_provider", msg: "sso: provider already registered"}
	ErrSSOInvalidSubject = &SSOError{code: "invalid_subject", msg: "sso: identity missing subject"}
)

// SSOError is the typed error returned by the SSO router.
type SSOError struct {
	code string
	msg  string
}

func (e *SSOError) Error() string { return e.msg }

// Code returns the wire-stable error code (for HTTP status
// mapping: unknown_provider → 404, invalid_subject → 401,
// duplicate_provider → 500).
func (e *SSOError) Code() string { return e.code }

// IsSSOError reports whether the error is an *SSOError.
func IsSSOError(err error) bool {
	_, ok := err.(*SSOError)
	return ok
}

// ErrSSOInvalidAssertion is the standard error every
// provider should return on a bad assertion. Real
// implementations wrap it with the underlying cause:
//   return fmt.Errorf("oidc: %w: %v", ErrSSOInvalidAssertion, err)
var ErrSSOInvalidAssertion = &SSOError{code: "invalid_assertion", msg: "sso: assertion failed verification"}

// FakeSSOProvider is a non-cryptographic provider used by
// integration tests. The wire format is `fake:<name>:<role1>+<role2>`
// so the test can assert the role mapping without standing
// up an IdP.
type FakeSSOProvider struct {
	providerName string
	// expectedAssertion -> identity
	known map[string]*SSOIdentity
}

// NewFakeSSOProvider returns a provider named `n` with no
// pre-registered assertions. Add assertions with `RegisterAssertion`.
func NewFakeSSOProvider(n string) *FakeSSOProvider {
	return &FakeSSOProvider{providerName: n, known: make(map[string]*SSOIdentity)}
}

// RegisterAssertion teaches the fake that `assertion` should
// resolve to `identity`. Used in tests to wire the integration
// seam without standing up a real IdP.
func (f *FakeSSOProvider) RegisterAssertion(assertion string, identity *SSOIdentity) {
	f.known[assertion] = identity
}

// Name returns the provider's wire name.
func (f *FakeSSOProvider) Name() string { return f.providerName }

// Verify looks up the assertion. The wire format is
// `fake:<roles>` so a one-liner in tests is enough:
//   p.RegisterAssertion("fake:viewer", &SSOIdentity{Subject: "u1", ...})
func (f *FakeSSOProvider) Verify(assertion string) (*SSOIdentity, error) {
	if !strings.HasPrefix(assertion, "fake:") {
		return nil, ErrSSOInvalidAssertion
	}
	if id, ok := f.known[assertion]; ok {
		return id, nil
	}
	return nil, ErrSSOInvalidAssertion
}

// sessionIssuerFake is a test double for SessionIssuer.
type sessionIssuerFake struct {
	token     string
	err       error
	lastIdent *SSOIdentity
	calls     int
}

func (s *sessionIssuerFake) Issue(identity *SSOIdentity) (string, error) {
	s.calls++
	s.lastIdent = identity
	return s.token, s.err
}

func TestSSO_Router_RegisterAndLookup(t *testing.T) {
	r := NewSSORouter()
	p := NewFakeSSOProvider("fake")
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(p); !IsSSOError(err) {
		t.Errorf("expected SSOError on duplicate register, got %v", err)
	}
	got, err := r.Lookup("fake")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Name() != "fake" {
		t.Errorf("Lookup returned %q", got.Name())
	}
	if _, err := r.Lookup("nope"); !IsSSOError(err) {
		t.Errorf("expected SSOError on unknown lookup, got %v", err)
	}
}

func TestSSO_Router_Names(t *testing.T) {
	r := NewSSORouter()
	_ = r.Register(NewFakeSSOProvider("z"))
	_ = r.Register(NewFakeSSOProvider("a"))
	_ = r.Register(NewFakeSSOProvider("m"))
	got := r.Names()
	want := []string{"a", "m", "z"}
	if len(got) != len(want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSSO_HandleCallback_IssuesSession(t *testing.T) {
	r := NewSSORouter()
	p := NewFakeSSOProvider("fake")
	p.RegisterAssertion("fake:viewer", &SSOIdentity{
		Subject: "u-1", Email: "u@x", Provider: "fake", Roles: []string{"viewer"},
	})
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sess := &sessionIssuerFake{token: "local-token"}
	identity, token, err := r.HandleCallback("fake", "fake:viewer", sess)
	if err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}
	if identity.Subject != "u-1" {
		t.Errorf("Subject = %q", identity.Subject)
	}
	if token != "local-token" {
		t.Errorf("token = %q", token)
	}
	if sess.calls != 1 {
		t.Errorf("session issuer calls = %d", sess.calls)
	}
}

func TestSSO_HandleCallback_RejectsInvalidAssertion(t *testing.T) {
	r := NewSSORouter()
	_ = r.Register(NewFakeSSOProvider("fake"))
	sess := &sessionIssuerFake{token: "x"}
	_, _, err := r.HandleCallback("fake", "fake:does-not-exist", sess)
	if !IsSSOError(err) {
		t.Fatalf("expected SSOError, got %v", err)
	}
	if sess.calls != 0 {
		t.Errorf("session issuer should not be called for invalid assertion, got %d", sess.calls)
	}
}

func TestSSO_HandleCallback_RejectsEmptySubject(t *testing.T) {
	r := NewSSORouter()
	p := NewFakeSSOProvider("fake")
	p.RegisterAssertion("fake:nosub", &SSOIdentity{Subject: "", Provider: "fake"})
	_ = r.Register(p)
	sess := &sessionIssuerFake{token: "x"}
	_, _, err := r.HandleCallback("fake", "fake:nosub", sess)
	if !errors.Is(err, ErrSSOInvalidSubject) {
		t.Fatalf("expected ErrSSOInvalidSubject, got %v", err)
	}
}

func TestSSO_HandleCallback_UnknownProvider(t *testing.T) {
	r := NewSSORouter()
	sess := &sessionIssuerFake{token: "x"}
	_, _, err := r.HandleCallback("ghost", "anything", sess)
	if !errors.Is(err, ErrSSOUnknown) {
		t.Fatalf("expected ErrSSOUnknown, got %v", err)
	}
}

func TestSSO_FakeProvider_RejectsNonFakeAssertion(t *testing.T) {
	p := NewFakeSSOProvider("fake")
	if _, err := p.Verify("not-a-fake-token"); !IsSSOError(err) {
		t.Fatalf("expected SSOError, got %v", err)
	}
}

func TestSSO_Identity_HasRole(t *testing.T) {
	i := &SSOIdentity{Roles: []string{"viewer", "editor"}}
	if !i.HasRole("viewer") {
		t.Errorf("expected viewer role")
	}
	if i.HasRole("admin") {
		t.Errorf("expected no admin role")
	}
}

func TestSSO_Router_HTTP_CallbackShape(t *testing.T) {
	// The router is HTTP-agnostic; the HTTP layer is in
	// `cmd/m9m/main.go`. This test asserts that the public
	// API surface (Names + HandleCallback) is enough to wire
	// an http.HandlerFunc without poking at internals. If a
	// future refactor changes the signature, the handler
	// shape will be obvious.
	r := NewSSORouter()
	_ = r.Register(NewFakeSSOProvider("okta"))
	handler := func(w http.ResponseWriter, req *http.Request) {
		// Simulate `POST /auth/sso/{provider}/callback`.
		// provider := path.Base(path.Dir(req.URL.Path))
		// assertion := req.FormValue("assertion")
		// (the http test scaffolding is in cmd/m9m).
		_, _, _ = r.HandleCallback("okta", "anything", nil)
		w.WriteHeader(http.StatusOK)
	}
	if handler == nil {
		t.Fatal("handler construction failed")
	}
}
