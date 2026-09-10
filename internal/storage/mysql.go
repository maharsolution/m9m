package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/tenancy"
)

// MySQLStorage provides MySQL-backed workflow storage.
//
// Implementation mirrors PostgresStorage so the rest of the engine sees the
// same WorkflowStorage contract; only the SQL dialect differs (placeholder
// `?` instead of `$N`, native JSON column, backtick-quoted identifiers,
// TIMESTAMP with explicit defaults).
type MySQLStorage struct {
	db *sql.DB
}

// NewMySQLStorage opens a MySQL/MariaDB connection and initializes schema.
//
// The connection string follows the go-sql-driver/mysql DSN format, e.g.
//   user:pass@tcp(127.0.0.1:3306)/m9m?parseTime=true&charset=utf8mb4&loc=UTC
//
// Caller should pass parseTime=true so TIMESTAMP/DATETIME columns scan into
// time.Time without manual conversion. utf8mb4 is required for full unicode
// + emoji support.
func NewMySQLStorage(dsn string) (*MySQLStorage, error) {
	if dsn == "" {
		return nil, fmt.Errorf("mysql DSN is empty")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping mysql database: %w", err)
	}

	storage := &MySQLStorage{db: db}

	if err := storage.initSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	if err := bootstrapDefaultWorkspace(storage); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to bootstrap default workspace: %w", err)
	}

	return storage, nil
}

// initSchema creates the necessary database tables for MySQL.
//
// Differences from PostgresStorage:
//   - VARCHAR(N) instead of TEXT for short columns
//   - JSON instead of JSONB (MySQL 5.7+/8.0 JSON type)
//   - TINYINT(1) for active flag (idiomatic in MySQL)
//   - backtick-quoted identifiers (only `key` is a reserved word in MySQL)
//   - TIMESTAMP DEFAULT CURRENT_TIMESTAMP
//   - Schema is split into individual statements because MySQL's
//     go-sql-driver executes Exec one statement at a time when multiStatements
//     is off (the default). Splitting also makes debugging easier when
//     a single CREATE TABLE fails.
func (s *MySQLStorage) initSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS workflows (
			id VARCHAR(255) PRIMARY KEY,
			workspace_id VARCHAR(255) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			name VARCHAR(255) NOT NULL,
			description TEXT,
			nodes JSON NOT NULL,
			connections JSON NOT NULL,
			settings JSON,
			active TINYINT(1) DEFAULT 0,
			tags JSON,
			debug TINYINT(1) DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_by VARCHAR(255)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE IF NOT EXISTS executions (
			id VARCHAR(255) PRIMARY KEY,
			workflow_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			status VARCHAR(50) NOT NULL,
			mode VARCHAR(50) NOT NULL,
			started_at TIMESTAMP NOT NULL,
			finished_at TIMESTAMP NULL,
			data JSON,
			node_data JSON,
			edges_taken JSON,
			error TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE IF NOT EXISTS credentials (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			type VARCHAR(255) NOT NULL,
			data JSON NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE IF NOT EXISTS tags (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			color VARCHAR(50),
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE IF NOT EXISTS workspaces (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			organization_id VARCHAR(255),
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE IF NOT EXISTS raw_data (
			` + "`key`" + ` VARCHAR(255) PRIMARY KEY,
			value LONGBLOB NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE INDEX idx_workflows_active ON workflows(active)`,
		`CREATE INDEX idx_workflows_name ON workflows(name)`,
		`CREATE INDEX idx_workflows_workspace ON workflows(workspace_id)`,
		`CREATE INDEX idx_executions_workflow_id ON executions(workflow_id)`,
		`CREATE INDEX idx_executions_status ON executions(status)`,
		`CREATE INDEX idx_executions_workspace ON executions(workspace_id)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			// MySQL doesn't support CREATE INDEX IF NOT EXISTS in versions
			// earlier than 8.0.29. Swallow the "Duplicate key name" error
			// (Error 1061) so re-running the schema is idempotent on any
			// supported version.
			if strings.Contains(err.Error(), "1061") || strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
				continue
			}
			return fmt.Errorf("failed to execute schema statement: %w\nstatement: %s", err, stmt)
		}
	}

	if err := ensureColumn(s.db, "workflows", "workspace_id",
		"VARCHAR(255) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'"); err != nil {
		return err
	}
	if err := ensureColumn(s.db, "executions", "workspace_id",
		"VARCHAR(255) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'"); err != nil {
		return err
	}
	// node_data: per-node input/output snapshot used by the n8n-style
	// execution detail view. Stored as JSON so it survives across engine
	// upgrades without altering the column type.
	if err := ensureColumn(s.db, "executions", "node_data", "JSON"); err != nil {
		return err
	}
	// edges_taken: per-execution map of which connections actually
	// carried items. Stored as JSON so it survives across engine
	// upgrades and so MySQL can index it if we ever want to query
	// "which executions branched at node X". Used by the execution
	// detail view to colour edges green/grey without bloating the
	// DB with per-node I/O snapshots.
	if err := ensureColumn(s.db, "executions", "edges_taken", "JSON"); err != nil {
		return err
	}
	// debug: per-workflow boolean that gates how much per-node
	// data the engine captures (see model.Workflow.Debug for the
	// semantics). Stored as TINYINT(1) to match the `active` flag.
	if err := ensureColumn(s.db, "workflows", "debug", "TINYINT(1) DEFAULT 0"); err != nil {
		return err
	}
	return nil
}

// --- Workflow operations ---

func (s *MySQLStorage) SaveWorkflow(workflow *model.Workflow) error {
	if workflow.ID == "" {
		workflow.ID = generateID("workflow")
	}
	workflow.WorkspaceID = resolveWorkspaceID(workflow.WorkspaceID)

	now := time.Now()
	if workflow.CreatedAt.IsZero() {
		workflow.CreatedAt = now
	}
	workflow.UpdatedAt = now

	nodesJSON, _ := json.Marshal(workflow.Nodes)
	connectionsJSON, _ := json.Marshal(workflow.Connections)
	settingsJSON, _ := json.Marshal(workflow.Settings)
	tagsJSON, _ := json.Marshal(workflow.Tags)

	query := `
		INSERT INTO workflows (id, workspace_id, name, description, nodes, connections, settings, active, tags, debug, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			workspace_id = VALUES(workspace_id),
			name = VALUES(name),
			description = VALUES(description),
			nodes = VALUES(nodes),
			connections = VALUES(connections),
			settings = VALUES(settings),
			active = VALUES(active),
			tags = VALUES(tags),
			debug = VALUES(debug),
			updated_at = VALUES(updated_at)
	`

	_, err := s.db.Exec(query,
		workflow.ID, workflow.WorkspaceID, workflow.Name, workflow.Description,
		string(nodesJSON), string(connectionsJSON), string(settingsJSON), boolToInt(workflow.Active), string(tagsJSON),
		boolToInt(workflow.Debug), workflow.CreatedAt, workflow.UpdatedAt)

	return err
}

func (s *MySQLStorage) GetWorkflow(id string) (*model.Workflow, error) {
	query := `
		SELECT id, workspace_id, name, description, nodes, connections, settings, active, tags, debug, created_at, updated_at
		FROM workflows WHERE id = ?
	`

	var workflow model.Workflow
	var nodesJSON, connectionsJSON, settingsJSON, tagsJSON sql.NullString
	var activeInt, debugInt int

	err := s.db.QueryRow(query, id).Scan(
		&workflow.ID, &workflow.WorkspaceID, &workflow.Name, &workflow.Description,
		&nodesJSON, &connectionsJSON, &settingsJSON,
		&activeInt, &tagsJSON, &debugInt, &workflow.CreatedAt, &workflow.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	workflow.Active = activeInt == 1
	workflow.Debug = debugInt == 1

	if nodesJSON.Valid {
		_ = json.Unmarshal([]byte(nodesJSON.String), &workflow.Nodes)
	}
	if connectionsJSON.Valid {
		_ = json.Unmarshal([]byte(connectionsJSON.String), &workflow.Connections)
	}
	if settingsJSON.Valid {
		_ = json.Unmarshal([]byte(settingsJSON.String), &workflow.Settings)
	}
	if tagsJSON.Valid {
		_ = json.Unmarshal([]byte(tagsJSON.String), &workflow.Tags)
	}

	return &workflow, nil
}

func (s *MySQLStorage) ListWorkflows(filters WorkflowFilters) ([]*model.Workflow, int, error) {
	var conditions []string
	var args []interface{}

	if filters.WorkspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, filters.WorkspaceID)
	}

	if filters.Active != nil {
		conditions = append(conditions, "active = ?")
		args = append(args, boolToInt(*filters.Active))
	}

	if filters.Search != "" {
		conditions = append(conditions, "LOWER(name) LIKE ?")
		args = append(args, "%"+strings.ToLower(filters.Search)+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM workflows " + whereClause
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, workspace_id, name, description, nodes, connections, settings, active, tags, debug, created_at, updated_at
		FROM workflows %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, filters.Limit, filters.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var workflows []*model.Workflow
	for rows.Next() {
		var workflow model.Workflow
		var nodesJSON, connectionsJSON, settingsJSON, tagsJSON sql.NullString
		var activeInt, debugInt int

		err := rows.Scan(
			&workflow.ID, &workflow.WorkspaceID, &workflow.Name, &workflow.Description,
			&nodesJSON, &connectionsJSON, &settingsJSON,
			&activeInt, &tagsJSON, &debugInt, &workflow.CreatedAt, &workflow.UpdatedAt,
		)
		if err != nil {
			continue
		}

		workflow.Active = activeInt == 1
		workflow.Debug = debugInt == 1

		if nodesJSON.Valid {
			_ = json.Unmarshal([]byte(nodesJSON.String), &workflow.Nodes)
		}
		if connectionsJSON.Valid {
			_ = json.Unmarshal([]byte(connectionsJSON.String), &workflow.Connections)
		}
		if settingsJSON.Valid {
			_ = json.Unmarshal([]byte(settingsJSON.String), &workflow.Settings)
		}
		if tagsJSON.Valid {
			_ = json.Unmarshal([]byte(tagsJSON.String), &workflow.Tags)
		}

		workflows = append(workflows, &workflow)
	}

	return workflows, total, nil
}

func (s *MySQLStorage) UpdateWorkflow(id string, workflow *model.Workflow) error {
	workflow.ID = id
	workflow.UpdatedAt = time.Now()
	return s.SaveWorkflow(workflow)
}

func (s *MySQLStorage) DeleteWorkflow(id string) error {
	result, err := s.db.Exec("DELETE FROM workflows WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return nil
}

func (s *MySQLStorage) ActivateWorkflow(id string) error {
	result, err := s.db.Exec("UPDATE workflows SET active = 1, updated_at = ? WHERE id = ?", time.Now(), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return nil
}

func (s *MySQLStorage) DeactivateWorkflow(id string) error {
	result, err := s.db.Exec("UPDATE workflows SET active = 0, updated_at = ? WHERE id = ?", time.Now(), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return nil
}

// --- Execution operations ---

func (s *MySQLStorage) SaveExecution(execution *model.WorkflowExecution) error {
	if execution.ID == "" {
		execution.ID = generateID("exec")
	}
	execution.WorkspaceID = resolveWorkspaceID(execution.WorkspaceID)
	if execution.StartedAt.IsZero() {
		execution.StartedAt = time.Now()
	}

	dataJSON, _ := json.Marshal(execution.Data)
	var nodeDataJSON []byte
	if execution.NodeData != nil {
		nodeDataJSON, _ = json.Marshal(execution.NodeData)
	} else {
		nodeDataJSON = []byte("{}")
	}
	var edgesTakenJSON []byte
	if execution.EdgesTaken != nil {
		edgesTakenJSON, _ = json.Marshal(execution.EdgesTaken)
	} else {
		edgesTakenJSON = []byte("{}")
	}
	errorText := ""
	if execution.Error != nil {
		errorText = execution.Error.Error()
	}
	createdAt := time.Now()

	query := `
		INSERT INTO executions (id, workflow_id, workspace_id, status, mode, started_at, finished_at, data, node_data, edges_taken, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			workflow_id = VALUES(workflow_id),
			workspace_id = VALUES(workspace_id),
			status = VALUES(status),
			mode = VALUES(mode),
			started_at = VALUES(started_at),
			finished_at = VALUES(finished_at),
			data = VALUES(data),
			node_data = VALUES(node_data),
			edges_taken = VALUES(edges_taken),
			error = VALUES(error)
	`

	var finishedAt interface{}
	if execution.FinishedAt != nil {
		finishedAt = *execution.FinishedAt
	}

	_, err := s.db.Exec(query,
		execution.ID, execution.WorkflowID, execution.WorkspaceID, execution.Status,
		execution.Mode, execution.StartedAt, finishedAt, string(dataJSON), string(nodeDataJSON), string(edgesTakenJSON), errorText, createdAt)

	return err
}

func (s *MySQLStorage) GetExecution(id string) (*model.WorkflowExecution, error) {
	query := `
		SELECT id, workflow_id, workspace_id, status, mode, started_at, finished_at, data, node_data, edges_taken, error
		FROM executions WHERE id = ?
	`

	var execution model.WorkflowExecution
	var dataJSON sql.NullString
	var nodeDataJSON sql.NullString
	var edgesTakenJSON sql.NullString
	var errorText sql.NullString
	var finishedAt sql.NullTime

	err := s.db.QueryRow(query, id).Scan(
		&execution.ID, &execution.WorkflowID, &execution.WorkspaceID, &execution.Status, &execution.Mode,
		&execution.StartedAt, &finishedAt, &dataJSON, &nodeDataJSON, &edgesTakenJSON, &errorText,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("execution not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if finishedAt.Valid {
		execution.FinishedAt = &finishedAt.Time
	}
	if dataJSON.Valid {
		_ = json.Unmarshal([]byte(dataJSON.String), &execution.Data)
	}
	if nodeDataJSON.Valid && nodeDataJSON.String != "" && nodeDataJSON.String != "{}" {
		_ = json.Unmarshal([]byte(nodeDataJSON.String), &execution.NodeData)
	}
	if edgesTakenJSON.Valid && edgesTakenJSON.String != "" && edgesTakenJSON.String != "{}" {
		_ = json.Unmarshal([]byte(edgesTakenJSON.String), &execution.EdgesTaken)
	}
	if errorText.Valid && errorText.String != "" {
		execution.Error = fmt.Errorf("%s", errorText.String)
	}

	return &execution, nil
}

func (s *MySQLStorage) ListExecutions(filters ExecutionFilters) ([]*model.WorkflowExecution, int, error) {
	var conditions []string
	var args []interface{}

	if filters.WorkspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, filters.WorkspaceID)
	}
	if filters.WorkflowID != "" {
		conditions = append(conditions, "workflow_id = ?")
		args = append(args, filters.WorkflowID)
	}
	if filters.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filters.Status)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM executions " + whereClause
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, workflow_id, workspace_id, status, mode, started_at, finished_at, data, node_data, edges_taken, error
		FROM executions %s
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, filters.Limit, filters.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var executions []*model.WorkflowExecution
	for rows.Next() {
		var execution model.WorkflowExecution
		var dataJSON sql.NullString
		var nodeDataJSON sql.NullString
		var edgesTakenJSON sql.NullString
		var errorText sql.NullString
		var finishedAt sql.NullTime

		err := rows.Scan(
			&execution.ID, &execution.WorkflowID, &execution.WorkspaceID, &execution.Status, &execution.Mode,
			&execution.StartedAt, &finishedAt, &dataJSON, &nodeDataJSON, &edgesTakenJSON, &errorText,
		)
		if err != nil {
			continue
		}

		if finishedAt.Valid {
			execution.FinishedAt = &finishedAt.Time
		}
		if dataJSON.Valid {
			_ = json.Unmarshal([]byte(dataJSON.String), &execution.Data)
		}
		if nodeDataJSON.Valid && nodeDataJSON.String != "" && nodeDataJSON.String != "{}" {
			_ = json.Unmarshal([]byte(nodeDataJSON.String), &execution.NodeData)
		}
		if edgesTakenJSON.Valid && edgesTakenJSON.String != "" && edgesTakenJSON.String != "{}" {
			_ = json.Unmarshal([]byte(edgesTakenJSON.String), &execution.EdgesTaken)
		}
		if errorText.Valid && errorText.String != "" {
			execution.Error = fmt.Errorf("%s", errorText.String)
		}

		executions = append(executions, &execution)
	}

	return executions, total, nil
}

func (s *MySQLStorage) DeleteExecution(id string) error {
	result, err := s.db.Exec("DELETE FROM executions WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("execution not found: %s", id)
	}
	return nil
}

// CountWorkflows returns the total workflow count matching filters.
// Used by the Performance dashboard.
func (s *MySQLStorage) CountWorkflows(filters WorkflowFilters) (int, error) {
	var conditions []string
	var args []interface{}

	if filters.WorkspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, filters.WorkspaceID)
	}
	if filters.Active != nil {
		conditions = append(conditions, "active = ?")
		args = append(args, *filters.Active)
	}
	if filters.Search != "" {
		conditions = append(conditions, "LOWER(name) LIKE ?")
		args = append(args, "%"+strings.ToLower(filters.Search)+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM workflows "+whereClause, args...,
	).Scan(&count)
	return count, err
}

// CountExecutions returns the total execution count matching filters.
// Drives the "Total Executions" metric on the Performance page.
func (s *MySQLStorage) CountExecutions(filters ExecutionFilters) (int, error) {
	var conditions []string
	var args []interface{}

	if filters.WorkspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, filters.WorkspaceID)
	}
	if filters.WorkflowID != "" {
		conditions = append(conditions, "workflow_id = ?")
		args = append(args, filters.WorkflowID)
	}
	if filters.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filters.Status)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM executions "+whereClause, args...,
	).Scan(&count)
	return count, err
}

// RecentExecutions returns up to `limit` executions (default 200) most
// recently started, for the Performance page to compute avg duration
// and success rate.
func (s *MySQLStorage) RecentExecutions(filters ExecutionFilters, limit int) ([]*model.WorkflowExecution, error) {
	if limit <= 0 {
		limit = 200
	}

	var conditions []string
	var args []interface{}

	if filters.WorkspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, filters.WorkspaceID)
	}
	if filters.WorkflowID != "" {
		conditions = append(conditions, "workflow_id = ?")
		args = append(args, filters.WorkflowID)
	}
	if filters.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filters.Status)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT id, workflow_id, workspace_id, status, mode, started_at, finished_at, data, node_data, edges_taken, error
		FROM executions %s
		ORDER BY started_at DESC
		LIMIT ?
	`, whereClause)

	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*model.WorkflowExecution
	for rows.Next() {
		var execution model.WorkflowExecution
		var dataJSON []byte
		var nodeDataJSON []byte
		var edgesTakenJSON []byte
		var errorText sql.NullString
		var finishedAt sql.NullTime

		if err := rows.Scan(
			&execution.ID, &execution.WorkflowID, &execution.WorkspaceID,
			&execution.Status, &execution.Mode, &execution.StartedAt,
			&finishedAt, &dataJSON, &nodeDataJSON, &edgesTakenJSON, &errorText,
		); err != nil {
			continue
		}
		if finishedAt.Valid {
			execution.FinishedAt = &finishedAt.Time
		}
		_ = json.Unmarshal(dataJSON, &execution.Data)
		if len(nodeDataJSON) > 0 && string(nodeDataJSON) != "{}" {
			_ = json.Unmarshal(nodeDataJSON, &execution.NodeData)
		}
		if len(edgesTakenJSON) > 0 && string(edgesTakenJSON) != "{}" {
			_ = json.Unmarshal(edgesTakenJSON, &execution.EdgesTaken)
		}
		if errorText.Valid && errorText.String != "" {
			execution.Error = fmt.Errorf("%s", errorText.String)
		}
		executions = append(executions, &execution)
	}
	return executions, nil
}

// --- Credential operations ---

func (s *MySQLStorage) SaveCredential(credential *Credential) error {
	if credential.ID == "" {
		credential.ID = generateID("cred")
	}
	now := time.Now()
	if credential.CreatedAt.IsZero() {
		credential.CreatedAt = now
	}
	credential.UpdatedAt = now

	dataJSON, _ := json.Marshal(credential.Data)

	query := `
		INSERT INTO credentials (id, name, type, data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			name = VALUES(name),
			type = VALUES(type),
			data = VALUES(data),
			updated_at = VALUES(updated_at)
	`

	_, err := s.db.Exec(query, credential.ID, credential.Name, credential.Type,
		string(dataJSON), credential.CreatedAt, credential.UpdatedAt)
	return err
}

func (s *MySQLStorage) GetCredential(id string) (*Credential, error) {
	query := "SELECT id, name, type, data, created_at, updated_at FROM credentials WHERE id = ?"

	var credential Credential
	var dataJSON sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&credential.ID, &credential.Name, &credential.Type,
		&dataJSON, &credential.CreatedAt, &credential.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("credential not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if dataJSON.Valid {
		_ = json.Unmarshal([]byte(dataJSON.String), &credential.Data)
	}
	return &credential, nil
}

func (s *MySQLStorage) ListCredentials() ([]*Credential, error) {
	rows, err := s.db.Query("SELECT id, name, type, data, created_at, updated_at FROM credentials ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var credentials []*Credential
	for rows.Next() {
		var credential Credential
		var dataJSON sql.NullString

		err := rows.Scan(
			&credential.ID, &credential.Name, &credential.Type,
			&dataJSON, &credential.CreatedAt, &credential.UpdatedAt,
		)
		if err != nil {
			continue
		}
		if dataJSON.Valid {
			_ = json.Unmarshal([]byte(dataJSON.String), &credential.Data)
		}
		credentials = append(credentials, &credential)
	}
	return credentials, nil
}

func (s *MySQLStorage) UpdateCredential(id string, credential *Credential) error {
	credential.ID = id
	credential.UpdatedAt = time.Now()
	return s.SaveCredential(credential)
}

func (s *MySQLStorage) DeleteCredential(id string) error {
	result, err := s.db.Exec("DELETE FROM credentials WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("credential not found: %s", id)
	}
	return nil
}

// --- Tag operations ---

func (s *MySQLStorage) SaveTag(tag *Tag) error {
	if tag.ID == "" {
		tag.ID = generateID("tag")
	}
	now := time.Now()
	if tag.CreatedAt.IsZero() {
		tag.CreatedAt = now
	}
	tag.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO tags (id, name, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			name = VALUES(name),
			color = VALUES(color),
			updated_at = VALUES(updated_at)
	`, tag.ID, tag.Name, tag.Color, tag.CreatedAt, tag.UpdatedAt)
	return err
}

func (s *MySQLStorage) GetTag(id string) (*Tag, error) {
	var tag Tag
	err := s.db.QueryRow("SELECT id, name, color, created_at, updated_at FROM tags WHERE id = ?",
		id).Scan(&tag.ID, &tag.Name, &tag.Color, &tag.CreatedAt, &tag.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tag not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &tag, nil
}

func (s *MySQLStorage) ListTags() ([]*Tag, error) {
	rows, err := s.db.Query("SELECT id, name, color, created_at, updated_at FROM tags ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []*Tag
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Color, &tag.CreatedAt, &tag.UpdatedAt); err != nil {
			continue
		}
		tags = append(tags, &tag)
	}
	return tags, nil
}

func (s *MySQLStorage) UpdateTag(id string, tag *Tag) error {
	tag.ID = id
	tag.UpdatedAt = time.Now()
	return s.SaveTag(tag)
}

func (s *MySQLStorage) DeleteTag(id string) error {
	result, err := s.db.Exec("DELETE FROM tags WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tag not found: %s", id)
	}
	return nil
}

// --- Workspace operations ---

func (s *MySQLStorage) SaveWorkspace(ws *tenancy.Workspace) error {
	if err := ws.Validate(); err != nil {
		return err
	}
	if ws.CreatedAt.IsZero() {
		ws.CreatedAt = time.Now().UTC()
	}
	ws.UpdatedAt = time.Now().UTC()

	_, err := s.db.Exec(`
		INSERT INTO workspaces (id, name, organization_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			name = VALUES(name),
			organization_id = VALUES(organization_id),
			updated_at = VALUES(updated_at)
	`, ws.ID, ws.Name, nullableStringMySQL(ws.OrganizationID), ws.CreatedAt, ws.UpdatedAt)
	return err
}

func (s *MySQLStorage) GetWorkspace(id string) (*tenancy.Workspace, error) {
	var ws tenancy.Workspace
	var orgID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, organization_id, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, id).Scan(&ws.ID, &ws.Name, &orgID, &ws.CreatedAt, &ws.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if orgID.Valid {
		ws.OrganizationID = orgID.String
	}
	return &ws, nil
}

func (s *MySQLStorage) ListWorkspaces() ([]*tenancy.Workspace, error) {
	rows, err := s.db.Query(`
		SELECT id, name, organization_id, created_at, updated_at
		FROM workspaces ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*tenancy.Workspace
	for rows.Next() {
		var ws tenancy.Workspace
		var orgID sql.NullString
		if err := rows.Scan(&ws.ID, &ws.Name, &orgID, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
			continue
		}
		if orgID.Valid {
			ws.OrganizationID = orgID.String
		}
		out = append(out, &ws)
	}
	return out, nil
}

func (s *MySQLStorage) DeleteWorkspace(id string) error {
	if id == tenancy.DefaultID {
		return fmt.Errorf("cannot delete default workspace")
	}
	result, err := s.db.Exec("DELETE FROM workspaces WHERE id = ?", id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("workspace not found: %s", id)
	}
	return nil
}

// --- Raw key-value operations ---

func (s *MySQLStorage) SaveRaw(key string, value []byte) error {
	_, err := s.db.Exec(`
		INSERT INTO raw_data (` + "`key`" + `, value, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			value = VALUES(value),
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

func (s *MySQLStorage) GetRaw(key string) ([]byte, error) {
	var value []byte
	err := s.db.QueryRow("SELECT value FROM raw_data WHERE `key` = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return value, err
}

func (s *MySQLStorage) ListKeys(prefix string) ([]string, error) {
	rows, err := s.db.Query("SELECT `key` FROM raw_data WHERE `key` LIKE ?", prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *MySQLStorage) DeleteRaw(key string) error {
	_, err := s.db.Exec("DELETE FROM raw_data WHERE `key` = ?", key)
	return err
}

// --- Close ---

func (s *MySQLStorage) Close() error {
	return s.db.Close()
}

// --- Helpers ---

// boolToInt converts bool to TINYINT(1) representation (0/1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullableStringMySQL returns nil for empty strings so MySQL writes SQL NULL
// instead of empty string, matching the Postgres behavior of nullableString.
func nullableStringMySQL(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
