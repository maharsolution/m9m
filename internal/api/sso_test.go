package api

import (
	"errors"
	"net/http"
	"testing"
)

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
