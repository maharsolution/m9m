package database

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the SQL Database nodes (mysql, postgres, sqlite). The
// `sqlProperties` helper is shared across the three.

func sqlProperties() []base.NodeProperty {
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

func sqlConnectorProperties() []base.NodeProperty {
	return sqlProperties()
}
