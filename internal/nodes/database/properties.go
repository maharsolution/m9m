package database

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the SQL Database nodes (mysql, postgres, sqlite). The
// `sqlProperties` helper is shared across the three.

// sqlProperties is the shared parameter surface for the MySQL,
// PostgreSQL, and SQLite nodes. postgres.go aliases it as
// postgresProperties. Field names match what Execute and
// ValidateParameters actually read (connectionUrl / host / port /
// database / user / password / filename / operation / query /
// sslMode / tls).
func sqlProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Execute Query", Value: "executeQuery"},
		{Name: "Insert", Value: "insert"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
	}
	tlsModes := []base.Option{
		{Name: "Preferred", Value: "preferred"},
		{Name: "True", Value: "true"},
		{Name: "False", Value: "false"},
		{Name: "Skip verify", Value: "skip-verify"},
	}
	sslModes := []base.Option{
		{Name: "Disable", Value: "disable"},
		{Name: "Allow", Value: "allow"},
		{Name: "Prefer", Value: "prefer"},
		{Name: "Require", Value: "require"},
		{Name: "Verify CA", Value: "verify-ca"},
		{Name: "Verify full", Value: "verify-full"},
	}
	props := []base.NodeProperty{
		base.StringProp("Connection URL", "connectionUrl", "", "Full connection URL. When set, the individual host fields are ignored.", "postgres://user:pass@localhost:5432/db", false),
		base.StringProp("Host", "host", "localhost", "Database host.", "localhost", false),
		base.NumberProp("Port", "port", 5432, "Database port (3306 MySQL, 5432 PostgreSQL).", false),
		base.StringProp("Database", "database", "", "Database name.", "mydb", false),
		base.StringProp("User", "user", "", "Database user.", "dbuser", false),
		base.StringProp("Password", "password", "", "Database password.", "", false),
		base.StringProp("Filename", "filename", "", "SQLite database file path.", "/data/app.db", false),
		base.StringOpt("Operation", "operation", "executeQuery", "SQL operation to perform.", ops, true),
		base.TextProp("Query", "query", "", "SQL query. Required for executeQuery.", "SELECT 1", 6),
		base.StringOpt("SSL Mode", "sslMode", "disable", "PostgreSQL sslmode.", sslModes, false),
		base.StringOpt("TLS", "tls", "preferred", "MySQL TLS mode.", tlsModes, false),
	}
	return append(props, base.CommonSettings()...)
}

func sqlConnectorProperties() []base.NodeProperty {
	return sqlProperties()
}
