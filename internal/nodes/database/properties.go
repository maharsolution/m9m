package database

import "github.com/neul-labs/m9m/internal/nodes/base"

// sqlProperties is the shared property descriptor set for the SQL
// database nodes (Postgres, MySQL, SQLite). The shape mirrors
// n8n's INodeTypeDescription.properties so the n8n-style Parameters
// tab renders the same form across all three.
func sqlProperties() []base.NodeProperty {
	operations := []base.Option{
		{Name: "Execute Query", Value: "executeQuery"},
		{Name: "Insert", Value: "insert"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
		{Name: "Select", Value: "select"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Operation", "operation", "executeQuery", "Which SQL operation to perform.", operations, true),
		base.TextProp("Query", "query", "", "SQL query to run. Use `?` placeholders for parameters.", "SELECT * FROM users WHERE id = ?", 6),
		base.CollectionProp("Query Parameters", "queryParameters", "Positional or named parameters for the query.", nil),
		base.NumberProp("Connection timeout (s)", "options.connectionTimeout", 10, "How long to wait when opening the connection.", false),
		base.NumberProp("Query timeout (s)", "options.queryTimeout", 60, "Maximum query runtime before the connection is killed.", false),
	}
	return append(props, base.CommonSettings()...)
}
