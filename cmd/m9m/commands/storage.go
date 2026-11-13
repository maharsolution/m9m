package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neul-labs/m9m/internal/storage"
	"github.com/neul-labs/m9m/internal/workspace"
)

// storageOptions describes how the storage backend is selected.
//
// The resolution order matches `m9m serve` exactly, so an operator tool
// always talks to the same database the server uses:
//
//  1. an explicit --db-type (or M9M_DB_TYPE) forces a backend
//  2. otherwise the DSNs decide: --mysql / M9M_MYSQL_DSN first, then
//     --postgres / M9M_POSTGRES_URL / M9M_POSTGRES_DSN
//  3. --memory selects the in-memory backend
//  4. SQLite is the default
type storageOptions struct {
	// DBType forces a backend: sqlite|memory|postgres|mysql.
	DBType string
	// DBPath is the SQLite database path.
	DBPath string
	// PostgresURL is the PostgreSQL connection URL.
	PostgresURL string
	// MySQLDSN is the MySQL connection DSN.
	MySQLDSN string
	// Memory selects the in-memory backend.
	Memory bool
	// Workspace is the workspace whose SQLite file is used when DBPath
	// is empty.
	Workspace string
}

// openStorage opens the configured storage backend.
//
// It returns the store plus a human readable description of the backend
// ("MySQL", "PostgreSQL", "SQLite (<path>)", "in-memory") so the caller can
// print it without having to re-derive the choice.
//
// The caller owns the returned store and has to Close() it.
func openStorage(opts storageOptions) (storage.WorkflowStorage, string, error) {
	dbType := strings.ToLower(strings.TrimSpace(firstNonEmpty(opts.DBType, os.Getenv("M9M_DB_TYPE"))))
	mysqlDSN := firstNonEmpty(opts.MySQLDSN, os.Getenv("M9M_MYSQL_DSN"))
	postgresURL := firstNonEmpty(opts.PostgresURL, os.Getenv("M9M_POSTGRES_URL"), os.Getenv("M9M_POSTGRES_DSN"))

	switch {
	case dbType == "mysql" || (dbType == "" && mysqlDSN != ""):
		if mysqlDSN == "" {
			return nil, "", fmt.Errorf("MySQL storage requested but no DSN provided. Set --mysql or M9M_MYSQL_DSN")
		}
		store, err := storage.NewMySQLStorage(mysqlDSN)
		if err != nil {
			return nil, "", err
		}
		return store, "MySQL", nil

	case dbType == "postgres" || dbType == "postgresql" || (dbType == "" && postgresURL != ""):
		if postgresURL == "" {
			return nil, "", fmt.Errorf("PostgreSQL storage requested but no URL provided. Set --postgres, M9M_POSTGRES_URL, or M9M_POSTGRES_DSN")
		}
		store, err := storage.NewPostgresStorage(postgresURL)
		if err != nil {
			return nil, "", err
		}
		return store, "PostgreSQL", nil

	case dbType == "memory" || opts.Memory:
		return storage.NewMemoryStorage(), "in-memory", nil

	case dbType == "" || dbType == "sqlite":
		dbPath := opts.DBPath
		if dbPath == "" {
			// Check workspace first.
			if opts.Workspace != "" {
				mgr, _ := workspace.NewManager()
				if mgr != nil {
					dbPath, _ = mgr.GetStoragePath(opts.Workspace)
				}
			}
			// Default to the data directory.
			if dbPath == "" {
				homeDir, _ := os.UserHomeDir()
				dataDir := filepath.Join(homeDir, ".m9m", "data")
				_ = os.MkdirAll(dataDir, 0755)
				dbPath = filepath.Join(dataDir, "m9m.db")
			}
		}
		store, err := storage.NewSQLiteStorage(dbPath)
		if err != nil {
			return nil, "", err
		}
		return store, fmt.Sprintf("SQLite (%s)", dbPath), nil

	default:
		return nil, "", fmt.Errorf("unknown --db-type %q (supported: sqlite, memory, postgres, mysql)", dbType)
	}
}
