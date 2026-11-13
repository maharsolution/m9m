package api

import (
	"net/http"
	"net/url"
	"strings"
)

// originAllowed decides whether a WebSocket handshake may be accepted.
//
// The rules are:
//
//   - a handshake without an Origin header (a non-browser client) is
//     allowed: there is no browser origin model to enforce, and the
//     push ticket is the credential;
//   - a handshake from the SAME origin as the request (the host:port
//     the request was addressed to) is allowed;
//   - a handshake from an origin listed in M9M_ALLOWED_ORIGINS is
//     allowed;
//   - everything else is rejected.
//
// The check never opens up "any origin" on its own: only an explicit
// "*" in M9M_ALLOWED_ORIGINS (which is what --dev sets) does that.
func originAllowed(r *http.Request, allowedOrigins []string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	// Same origin: the host:port of the Origin header must match the
	// host:port the request was addressed to. The scheme is not
	// compared - a TLS-terminating proxy in front of the server makes
	// the request look like plain HTTP while the browser talks HTTPS.
	if r.Host != "" && strings.EqualFold(parsed.Host, r.Host) {
		return true
	}

	for _, allowed := range allowedOrigins {
		candidate := strings.TrimSpace(allowed)
		if candidate == "" {
			continue
		}
		if candidate == "*" {
			return true
		}
		if strings.EqualFold(strings.TrimSuffix(candidate, "/"), strings.TrimSuffix(origin, "/")) {
			return true
		}
	}
	return false
}
