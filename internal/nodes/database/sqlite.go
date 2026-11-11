/*
Package database provides database-related node implementations for m9m.
*/
package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// SQLiteNode implements the SQLite node functionality
type SQLiteNode struct {
	*base.BaseNode
}

// NewSQLiteNode creates a new SQLite node
func NewSQLiteNode() *SQLiteNode {
	description := base.NodeDescription{
		Name:        "SQLite",
		Description: "Executes queries against SQLite databases",
		Category:    "Database",
		Group:       []string{"database"},
		Properties:  sqliteProperties(),
		Inputs:      []string{"main"},
		Outputs:     []string{"main"},
	}

	return &SQLiteNode{
		BaseNode: base.NewBaseNode(description),
	}
}

// Description returns the node description
func (s *SQLiteNode) Description() base.NodeDescription {
	return s.BaseNode.Description()
}

// ValidateParameters validates SQLite node parameters
func (s *SQLiteNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return s.CreateError("parameters cannot be nil", nil)
	}

	// Check required parameters
	filename := s.GetStringParameter(params, "filename", "")
	if filename == "" {
		return s.CreateError("filename is required", nil)
	}

	// Check if operation is provided
	operation := s.GetStringParameter(params, "operation", "")
	if operation == "" {
		return s.CreateError("operation is required", nil)
	}

	validOperations := map[string]bool{
		"executeQuery": true,
		"insert":       true,
		"update":       true,
		"delete":       true,
	}

	if !validOperations[operation] {
		return s.CreateError(fmt.Sprintf("invalid operation: %s", operation), nil)
	}

	// For executeQuery operation, query is required
	if operation == "executeQuery" {
		query := s.GetStringParameter(params, "query", "")
		if query == "" {
			return s.CreateError("query is required for executeQuery operation", nil)
		}
	}

	return nil
}

// Execute processes the SQLite node operation
func (s *SQLiteNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	// Get filename parameter
	filename := s.GetStringParameter(nodeParams, "filename", "")

	// Connect to database
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, s.CreateError(fmt.Sprintf("failed to connect to database: %v", err), nil)
	}
	defer db.Close()

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, s.CreateError(fmt.Sprintf("failed to ping database: %v", err), nil)
	}

	// Get operation
	operation := s.GetStringParameter(nodeParams, "operation", "")

	// Process each input data item
	result := make([]model.DataItem, 0, len(inputData))

	for _, item := range inputData {
		switch operation {
		case "executeQuery":
			queryResults, err := s.executeQuery(db, nodeParams, item)
			if err != nil {
				return nil, s.CreateError(fmt.Sprintf("failed to execute query: %v", err), nil)
			}
			// n8n returns one item per row for executeQuery.
			result = append(result, queryResults...)

		default:
			// For other operations, just pass through the data
			result = append(result, item)
		}
	}

	return result, nil
}

// executeQuery executes a SELECT query and returns one item per row
func (s *SQLiteNode) executeQuery(db *sql.DB, nodeParams map[string]interface{}, item model.DataItem) ([]model.DataItem, error) {
	query := s.GetStringParameter(nodeParams, "query", "")

	// Execute query
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %v", err)
	}
	defer rows.Close()

	// Process rows — one item per row, like n8n.
	items, err := scanRows(rows)
	if err != nil {
		return nil, fmt.Errorf("error during row iteration: %v", err)
	}

	return items, nil
}
