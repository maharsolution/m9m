package database

import (
	"strconv"

	"github.com/neul-labs/m9m/internal/nodes/base"
)

// resolveConnectionParams reads MySQL/Postgres connection settings from
// node parameters, with credential-injected values taking precedence.
//
// n8n stores connection details on a credential envelope (e.g.
// `{mySql: {host, user, database, password}}`). The engine's
// CredentialManager flattens that envelope into the node params under
// `<credType>_<key>` (e.g. `mySql_host`, `mySql_user`). Explicit
// `host`/`user`/`database`/`password` keys on the node itself still win
// so users can override credentials in the node UI the same way n8n does.
//
// The caller is responsible for validating that the returned values
// make sense (non-empty host, etc.) — this helper just resolves.
func resolveConnectionParams(b *base.BaseNode, nodeParams map[string]interface{}, credPrefix string, defaultPort int) (host string, port int, database string, user string, password string, tls string) {
	host = b.GetStringParameter(nodeParams, "host", "")
	if host == "" && credPrefix != "" {
		host = b.GetStringParameter(nodeParams, credPrefix+"_host", "")
	}

	// Resolve port. We try the explicit node key first (as int), then
	// the credential-injected key (which arrives as int64 / float64 /
	// string depending on JSON source), then fall back to defaultPort.
	port = b.GetIntParameter(nodeParams, "port", 0)
	if port == 0 && credPrefix != "" {
		port = readPortish(b.GetStringParameter(nodeParams, credPrefix+"_port", ""), defaultPort)
	}
	if port == 0 {
		port = defaultPort
	}

	database = b.GetStringParameter(nodeParams, "database", "")
	if database == "" && credPrefix != "" {
		database = b.GetStringParameter(nodeParams, credPrefix+"_database", "")
	}

	user = b.GetStringParameter(nodeParams, "user", "")
	if user == "" && credPrefix != "" {
		user = b.GetStringParameter(nodeParams, credPrefix+"_user", "")
	}

	password = b.GetStringParameter(nodeParams, "password", "")
	if password == "" && credPrefix != "" {
		password = b.GetStringParameter(nodeParams, credPrefix+"_password", "")
	}

	tls = b.GetStringParameter(nodeParams, "tls", "")
	if tls == "" && credPrefix != "" {
		tls = b.GetStringParameter(nodeParams, credPrefix+"_tls", "")
	}

	return
}

// readPortish parses a port from a string-encoded credential value. If
// the input is empty, fallback is returned. Anything else that doesn't
// parse as an int returns the fallback — we never want a bogus port to
// silently downgrade to 0.
func readPortish(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return fallback
}
