/*
Package database provides database-related node implementations for m9m.
*/
package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// PostgresNode implements the PostgreSQL node functionality
type PostgresNode struct {
	*base.BaseNode
}

// NewPostgresNode creates a new PostgreSQL node
func NewPostgresNode() *PostgresNode {
	description := base.NodeDescription{
		Name:        "PostgreSQL",
		Description: "Executes queries against PostgreSQL databases",
		Category:    "Database",
		Properties:  postgresProperties(),
		Inputs:      []string{"main"},
		Outputs:     []string{"main"},
	}

	return &PostgresNode{
		BaseNode: base.NewBaseNode(description),
	}
}

// postgresProperties returns the property descriptors for the
// PostgreSQL node. Same shape as MySQL / SQLite — see
// sqlProperties in properties.go.
func postgresProperties() []base.NodeProperty {
	return sqlProperties()
}

// Description returns the node description
func (p *PostgresNode) Description() base.NodeDescription {
	return p.BaseNode.Description()
}

// ValidateParameters validates PostgreSQL node parameters
func (p *PostgresNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return p.CreateError("parameters cannot be nil", nil)
	}
	
	// Check required parameters
	connectionURL := p.GetStringParameter(params, "connectionUrl", "")
	if connectionURL == "" {
		// Accept explicit node-level keys OR credential-injected
		// `postgres_<key>` values (the engine's CredentialManager
		// flattens the credential envelope at execution time, so a
		// node with no in-place connection settings can still
		// resolve via the synced credential).
		host := p.GetStringParameter(params, "host", "")
		if host == "" {
			host = p.GetStringParameter(params, "postgres_host", "")
		}
		database := p.GetStringParameter(params, "database", "")
		if database == "" {
			database = p.GetStringParameter(params, "postgres_database", "")
		}

		if host == "" {
			return p.CreateError("either connectionUrl or host is required", nil)
		}

		if database == "" {
			return p.CreateError("database is required", nil)
		}
	}
	
	// Check if operation is provided
	operation := p.GetStringParameter(params, "operation", "")
	if operation == "" {
		return p.CreateError("operation is required", nil)
	}
	
	validOperations := map[string]bool{
		"executeQuery": true,
		"insert":       true,
		"update":       true,
		"delete":       true,
	}
	
	if !validOperations[operation] {
		return p.CreateError(fmt.Sprintf("invalid operation: %s", operation), nil)
	}
	
	// For executeQuery operation, query is required
	if operation == "executeQuery" {
		query := p.GetStringParameter(params, "query", "")
		if query == "" {
			return p.CreateError("query is required for executeQuery operation", nil)
		}
	}
	
	return nil
}

// Execute processes the PostgreSQL node operation
func (p *PostgresNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}
	
	// Get connection parameters
	connectionURL := p.GetStringParameter(nodeParams, "connectionUrl", "")

	// If connection URL is not provided, build it from individual
	// parameters. The credPrefix "postgres" matches n8n's credential
	// type for this node — the engine's CredentialManager flattens
	// the credential envelope into `postgres_host`, `postgres_user`,
	// etc. on nodeParams at execution time, so a node with no
	// explicit connection settings can still resolve them via the
	// synced credential.
	if connectionURL == "" {
		host, port, database, user, password, _ := resolveConnectionParams(p.BaseNode, nodeParams, "postgres", 5432)

		// SECURITY: Get SSL mode from parameters, default to "require" for encrypted connections
		sslMode := p.GetStringParameter(nodeParams, "sslMode", "require")

		// SECURITY: Validate SSL mode
		validSSLModes := map[string]bool{
			"disable": true, "allow": true, "prefer": true, "require": true,
			"verify-ca": true, "verify-full": true,
		}
		if !validSSLModes[sslMode] {
			return nil, p.CreateError(fmt.Sprintf("invalid sslMode: %s", sslMode), nil)
		}

		// SECURITY: Warn if SSL is disabled
		if sslMode == "disable" {
			log.Printf("SECURITY WARNING: PostgreSQL connection to %s using sslmode=disable. This is insecure for production.", host)
		}

		// SECURITY: Use URL format with proper escaping for credentials
		u := &url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(user, password),
			Host:   fmt.Sprintf("%s:%d", host, port),
			Path:   "/" + database,
		}
		q := u.Query()
		q.Set("sslmode", sslMode)
		u.RawQuery = q.Encode()
		connectionURL = u.String()
	} else {
		// SECURITY: Warn if provided URL contains sslmode=disable
		if strings.Contains(connectionURL, "sslmode=disable") {
			log.Printf("SECURITY WARNING: PostgreSQL connection URL contains sslmode=disable. This is insecure for production.")
		}
	}
	
	// Connect to database
	db, err := sql.Open("postgres", connectionURL)
	if err != nil {
		return nil, p.CreateError(fmt.Sprintf("failed to connect to database: %v", err), nil)
	}
	defer db.Close()
	
	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, p.CreateError(fmt.Sprintf("failed to ping database: %v", err), nil)
	}
	
	// Get operation
	operation := p.GetStringParameter(nodeParams, "operation", "")
	
	// Process each input data item
	result := make([]model.DataItem, len(inputData))
	
	for i, item := range inputData {
		var newItem model.DataItem
		
		switch operation {
		case "executeQuery":
			queryResult, err := p.executeQuery(db, nodeParams, item)
			if err != nil {
				return nil, p.CreateError(fmt.Sprintf("failed to execute query: %v", err), nil)
			}
			newItem = queryResult
			
		default:
			// For other operations, just pass through the data
			newItem = item
		}
		
		result[i] = newItem
	}
	
	return result, nil
}

// executeQuery executes a SELECT query and returns results
func (p *PostgresNode) executeQuery(db *sql.DB, nodeParams map[string]interface{}, item model.DataItem) (model.DataItem, error) {
	query := p.GetStringParameter(nodeParams, "query", "")

	// Resolve `{{ ... }}` expressions against the inbound data item so
	// parameterised queries like
	// `select * from {{ $json.body.table_name }}` resolve before the
	// driver sees them. Without this, PostgreSQL receives the raw
	// template and returns a syntax error.
	resolved, err := resolveQueryTemplate(query, item)
	if err != nil {
		return model.DataItem{}, err
	}
	query = resolved

	// SECURITY: Basic validation - reject obviously dangerous patterns
	// Note: This is defense-in-depth, not a complete SQL injection prevention
	dangerousPatterns := []string{
		"--",           // SQL comment
		";",            // Statement terminator (could chain queries)
		"/*",           // Block comment start
		"*/",           // Block comment end
		"xp_",          // SQL Server extended procedures
		"EXEC ",        // Execute
		"EXECUTE ",     // Execute
		"sp_",          // Stored procedures
		"DROP ",        // Drop statement
		"TRUNCATE ",    // Truncate statement
		"ALTER ",       // Alter statement
		"CREATE ",      // Create statement
		"GRANT ",       // Grant privileges
		"REVOKE ",      // Revoke privileges
	}

	upperQuery := strings.ToUpper(query)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(upperQuery, pattern) {
			// Log for security monitoring
			log.Printf("SECURITY WARNING: Potentially dangerous SQL pattern detected: %s", pattern)
			// Allow but warn - in strict mode, you might want to reject
		}
	}

	// Execute query
	rows, err := db.Query(query)
	if err != nil {
		return model.DataItem{}, fmt.Errorf("failed to execute query: %v", err)
	}
	defer rows.Close()
	
	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return model.DataItem{}, fmt.Errorf("failed to get columns: %v", err)
	}
	
	// Process rows
	var results []map[string]interface{}
	
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		
		// Scan the row
		if err := rows.Scan(valuePtrs...); err != nil {
			return model.DataItem{}, fmt.Errorf("failed to scan row: %v", err)
		}
		
		// Create a map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			// Handle nil values
			if values[i] == nil {
				rowMap[col] = nil
			} else {
				rowMap[col] = values[i]
			}
		}
		
		results = append(results, rowMap)
	}
	
	// Check for errors after iteration
	if err := rows.Err(); err != nil {
		return model.DataItem{}, fmt.Errorf("error during row iteration: %v", err)
	}
	
	// Create result item
	resultItem := model.DataItem{
		JSON: map[string]interface{}{
			"rows": results,
		},
	}
	
	return resultItem, nil
}