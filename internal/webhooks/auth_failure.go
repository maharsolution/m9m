package webhooks

import "net/http"

// authFailure is an authentication rejection that carries n8n's wire
// shape: the HTTP status, the plain-text body and the challenge
// header n8n sends.
//
// n8n answers a rejected webhook request with a short plain-text
// message, no Content-Type header and a `WWW-Authenticate` challenge:
//
//	basicAuth, no Authorization header    → 401 "Authorization is required!"
//	basicAuth, wrong credentials          → 401 "Authentication data is wrong!"
//	jwtAuth, no token                     → 401 "No token provided"
//	jwtAuth, token present but not valid  → 403 "invalid token"
//	headerAuth, header missing or wrong   → 403 "Authorization data is wrong!"
//
// Verified live against http://187.77.113.218:5678/webhook/{basic_auth,
// token_auth,auth_apikey} on 2026-11-11.
//
// The 401 / 403 split is n8n's and it is meaningful: 401 means "you
// did not present credentials", 403 means "you presented credentials
// and they are not acceptable". m9m used to answer every rejection
// with 401 + a JSON envelope, which broke parity on both the status
// code and the Content-Type.
//
// The JSON envelope m9m uses for the other webhook error paths (404
// not registered, 400 / 422 bad body) is NOT affected — n8n sends the
// very same {code,message,hint} JSON there.
type authFailure struct {
	status  int
	message string
}

// Error implements the error interface.
func (e *authFailure) Error() string { return e.message }

// newAuthFailure builds a rejection with n8n's status and message.
func newAuthFailure(status int, message string) *authFailure {
	return &authFailure{status: status, message: message}
}

// n8n's authentication rejection messages.
const (
	// authMessageRequired is n8n's answer when no credentials were
	// presented at all (basic auth without an Authorization header).
	authMessageRequired = "Authorization is required!"
	// authMessageBadCredentials is n8n's answer when basic auth
	// credentials were presented but did not match.
	authMessageBadCredentials = "Authentication data is wrong!"
	// authMessageWrongData is n8n's answer when a header / API key
	// was expected and did not match.
	authMessageWrongData = "Authorization data is wrong!"
	// authMessageNoToken is n8n's answer when a bearer token was
	// required but not supplied.
	authMessageNoToken = "No token provided"
	// authMessageBadToken is n8n's answer when a bearer token was
	// supplied but is not valid.
	authMessageBadToken = "invalid token"
)

// writeAuthFailure writes an authentication rejection in n8n's wire
// shape.
//
// The Content-Type header is suppressed on purpose: n8n sends the
// message without one, and Go's net/http would otherwise sniff the
// body and add "text/plain; charset=utf-8". Setting the header entry
// to nil keeps the key present (so the sniffer stays off) while
// writing nothing.
//
// The `WWW-Authenticate: Basic realm="Webhook"` challenge is what n8n
// sends on every auth rejection, whatever the configured auth type.
func writeAuthFailure(w http.ResponseWriter, failure *authFailure) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Webhook"`)
	w.Header()["Content-Type"] = nil
	w.WriteHeader(failure.status)
	_, _ = w.Write([]byte(failure.message))
}
