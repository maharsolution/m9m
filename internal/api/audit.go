package api

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"time"
)

// Audit actions of the secret-bearing operations.
const (
	// AuditActionCredentialReadData is written when a caller reads the
	// secret payload of a credential.
	AuditActionCredentialReadData = "credential.read-data"
	// AuditActionCredentialExport is written when credentials are
	// exported.
	AuditActionCredentialExport = "credential.export"
	// AuditActionCredentialImport is written when credentials are
	// imported.
	AuditActionCredentialImport = "credential.import"
)

// AuditEvent is one structured audit line.
//
// It NEVER carries a secret value: only the identity of the caller,
// where the call came from, which credential NAMES were touched and
// when it happened. The credential names are metadata - the payload of
// a credential never reaches an audit line.
type AuditEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	Action     string    `json:"action"`
	Result     string    `json:"result"`
	UserID     string    `json:"userId,omitempty"`
	Role       string    `json:"role,omitempty"`
	APIKeyID   string    `json:"apiKeyId,omitempty"`
	AuthMethod string    `json:"authMethod,omitempty"`
	IP         string    `json:"ip,omitempty"`
	Path       string    `json:"path,omitempty"`
	Names      []string  `json:"names,omitempty"`
	Count      int       `json:"count,omitempty"`
}

// audit writes one structured audit line.
//
// The line is a single JSON object on one line - greppable, and never
// carrying a secret.
func (s *APIServer) audit(r *http.Request, event AuditEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Result == "" {
		event.Result = "allowed"
	}
	if r != nil {
		event.Path = r.URL.Path
		event.IP = clientIP(r)
		if authCtx := AuthFromContext(r.Context()); authCtx != nil {
			event.UserID = authCtx.UserID
			event.Role = authCtx.Role
			event.APIKeyID = authCtx.APIKeyID
			event.AuthMethod = authCtx.AuthMethod
		}
	}

	line, err := json.Marshal(event)
	if err != nil {
		return
	}

	writer := s.auditWriter
	if writer == nil {
		writer = os.Stderr
	}

	_, _ = writer.Write(append(line, '\n'))
}

// clientIP returns the IP of the caller without the ephemeral port.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// The first entry is the original client.
		for i := 0; i < len(forwarded); i++ {
			if forwarded[i] == ',' {
				return forwarded[:i]
			}
		}
		return forwarded
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
