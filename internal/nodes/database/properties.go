package database

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// The SQL nodes are three thin wrappers over the same executor
// surface, but their connection parameters differ (Postgres speaks
// sslmode, MySQL speaks tls, SQLite only needs a file). Each node
// therefore gets its own descriptor set instead of one shared list
// whose defaults would be wrong for two of the three.

// postgresProperties describes the PostgreSQL node.
//
// database.PostgresNode reads `connectionUrl`, `host`, `port`,
// `database`, `user`, `password`, `sslMode`, `operation` and `query`.
// The query is only meaningful for the executeQuery operation, so it
// is shown for that operation alone.
func postgresProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Execute query", Value: "executeQuery"},
		{Name: "Insert", Value: "insert"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
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
		nodeprops.Str("Connection URL", "connectionUrl", "",
			"Full PostgreSQL connection URL. When set, the individual connection fields are ignored.",
			"postgres://user:pass@localhost:5432/mydb", false),
		nodeprops.Str("Host", "host", "localhost",
			"Database host (used when no connection URL is set).",
			"localhost", false),
		nodeprops.Num("Port", "port", 5432,
			"Database port.", false),
		nodeprops.Str("Database", "database", "",
			"Database name.",
			"mydb", false),
		nodeprops.Str("User", "user", "",
			"Database user.",
			"dbuser", false),
		nodeprops.Secret("Password", "password", "",
			"Database password.",
			"", false),
		nodeprops.Opt("SSL mode", "sslMode", "disable",
			"TLS mode used for the connection.",
			sslModes, false),
		nodeprops.Opt("Operation", "operation", "executeQuery",
			"Operation to run. Only \"Execute query\" sends SQL to the server; the others pass the input through.",
			ops, true),
		nodeprops.Required(nodeprops.Text("Query", "query", "",
			"SQL statement to execute. n8n expressions such as {{ $json.id }} are resolved before the query runs.",
			"SELECT * FROM users WHERE id = {{ $json.id }}", 6)).
			WithDisplayOptions(nodeprops.Show("operation", "executeQuery")),
	}
	return append(props, nodeprops.Settings()...)
}

// mysqlProperties describes the MySQL node.
//
// database.MySQLNode reads `connectionUrl`, `host`, `port`,
// `database`, `user`, `password`, `tls`, `operation` and `query`.
func mysqlProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Execute query", Value: "executeQuery"},
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
	props := []base.NodeProperty{
		nodeprops.Str("Connection URL", "connectionUrl", "",
			"Full MySQL connection URL. When set, the individual connection fields are ignored.",
			"mysql://user:pass@localhost:3306/mydb", false),
		nodeprops.Str("Host", "host", "localhost",
			"Database host (used when no connection URL is set).",
			"localhost", false),
		nodeprops.Num("Port", "port", 3306,
			"Database port.", false),
		nodeprops.Str("Database", "database", "",
			"Database name.",
			"mydb", false),
		nodeprops.Str("User", "user", "",
			"Database user.",
			"dbuser", false),
		nodeprops.Secret("Password", "password", "",
			"Database password.",
			"", false),
		nodeprops.Opt("TLS", "tls", "preferred",
			"TLS mode used for the connection.",
			tlsModes, false),
		nodeprops.Opt("Operation", "operation", "executeQuery",
			"Operation to run. Only \"Execute query\" sends SQL to the server; the others pass the input through.",
			ops, true),
		nodeprops.Required(nodeprops.Text("Query", "query", "",
			"SQL statement to execute. n8n expressions such as {{ $json.id }} are resolved before the query runs.",
			"SELECT * FROM users WHERE id = {{ $json.id }}", 6)).
			WithDisplayOptions(nodeprops.Show("operation", "executeQuery")),
	}
	return append(props, nodeprops.Settings()...)
}

// sqliteProperties describes the SQLite node.
//
// database.SQLiteNode reads `filename`, `operation` and `query`.
func sqliteProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Execute query", Value: "executeQuery"},
		{Name: "Insert", Value: "insert"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
	}
	props := []base.NodeProperty{
		nodeprops.Str("Database file", "filename", "",
			"Path of the SQLite database file.",
			"/data/app.db", true),
		nodeprops.Opt("Operation", "operation", "executeQuery",
			"Operation to run. Only \"Execute query\" sends SQL to the server; the others pass the input through.",
			ops, true),
		nodeprops.Required(nodeprops.Text("Query", "query", "",
			"SQL statement to execute. n8n expressions such as {{ $json.id }} are resolved before the query runs.",
			"SELECT * FROM users WHERE id = {{ $json.id }}", 6)).
			WithDisplayOptions(nodeprops.Show("operation", "executeQuery")),
	}
	return append(props, nodeprops.Settings()...)
}

// sqlConnectorProperties describes the generic SQL Database node.
//
// database.SQLConnectorNode reads `operation` (select | insert |
// update | delete | raw), `connectionString`, `database`, `query`,
// `table`, `columnMapping`, `whereColumn`, `whereValue`, `where`,
// `rawQuery`, `params`, `limit`, `databaseType`, `host`, `port`,
// `username`, `password` and `sslMode`.
func sqlConnectorProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Select", Value: "select"},
		{Name: "Insert", Value: "insert"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
		{Name: "Execute (raw)", Value: "raw"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "select",
			"Operation to run.",
			ops, true),
		nodeprops.Str("Connection string", "connectionString", "",
			"Connection string overriding the credential's own connection string.",
			"postgres://user:pass@localhost:5432/mydb", false),
		nodeprops.Str("Database", "database", "",
			"Database name (defaults to the credential's).",
			"mydb", false),
		nodeprops.Str("Database type", "databaseType", "postgres",
			"Driver used for the connection.",
			"postgres", false),
		nodeprops.Required(nodeprops.Text("Query", "query", "",
			"SQL statement to run (select / raw operations). May use n8n expressions.",
			"SELECT * FROM users WHERE id = {{ $json.id }}", 6)).
			WithDisplayOptions(nodeprops.Show("operation", "select")),
		nodeprops.Required(nodeprops.Text("Raw query", "rawQuery", "",
			"SQL statement to run when the operation is \"Execute (raw)\".",
			"UPDATE users SET active = 1", 6)).
			WithDisplayOptions(nodeprops.Show("operation", "raw")),
		nodeprops.Str("Table", "table", "",
			"Table affected by insert / update / delete operations.",
			"users", false).
			WithDisplayOptions(nodeprops.Show("operation", "insert", "update", "delete")),
		nodeprops.JSON("Column mapping", "columnMapping", "{}",
			"Mapping of column names to values (insert / update operations)."),
		nodeprops.Str("Where column", "whereColumn", "",
			"Column the update / delete filter applies to.",
			"id", false).
			WithDisplayOptions(nodeprops.Show("operation", "update", "delete")),
		nodeprops.Str("Where value", "whereValue", "",
			"Value the where column is compared with (n8n expressions allowed).",
			"={{ $json.id }}", false).
			WithDisplayOptions(nodeprops.Show("operation", "update", "delete")),
		nodeprops.JSON("Parameters", "params", "[]",
			"Bind parameters passed with the statement (array)."),
		nodeprops.Num("Limit", "limit", 0,
			"Maximum number of rows to return (0 = no limit, select only).", false).
			WithDisplayOptions(nodeprops.Show("operation", "select")),
	}
	return append(props, nodeprops.Settings()...)
}
