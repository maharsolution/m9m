package database

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the SQL Database connector node (the catch-all that runs an
// arbitrary SELECT/INSERT/UPDATE/DELETE on a configured
// connection).

// sqlProperties returns the shared MySQL / PostgreSQL / SQLite
// parameter descriptors. postgresProperties delegates here.
func sqlProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Execute Query", Value: "executeQuery"},
		{Name: "Insert", Value: "insert"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
	}
	props := []base.NodeProperty{
		base.StringProp("Connection URL", "connectionUrl", "", "Database connection URL. Overrides host and database when set.", "", false),
		base.StringProp("Host", "host", "", "Database host.", "localhost", false),
		base.StringProp("Database", "database", "", "Database name.", "", false),
		base.StringOpt("Operation", "operation", "executeQuery", "SQL operation to perform.", ops, true),
		base.TextProp("Query", "query", "", "SQL query. Required for executeQuery.", "SELECT 1", 6),
	}
	return append(props, base.CommonSettings()...)
}

func sqlConnectorProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Select", Value: "select"},
		{Name: "Insert", Value: "insert"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
		{Name: "Execute (raw)", Value: "execute"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Operation", "operation", "select", "SQL operation to perform.", ops, true),
		base.StringProp("Connection String", "connectionString", "", "Override the connection string (defaults to the credential's).", "", false),
		base.StringProp("Database", "database", "", "Database name (defaults to the credential's).", "", false),
		base.TextProp("Query", "query", "", "SQL query to execute. May use n8n expressions.", "SELECT * FROM users WHERE id = {{ $json.id }}", 6),
		base.JsonProp("Params", "params", "[]", "Bind parameters (array)."),
		base.NumberProp("Limit", "limit", 0, "Row limit (0 = no limit, select only).", false),
	}
	return append(props, base.CommonSettings()...)
}
