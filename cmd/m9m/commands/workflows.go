package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neul-labs/m9m/internal/storage"
)

var (
	workflowsDebugEnable  bool
	workflowsDebugDisable bool
	workflowsDebugAll     bool
	workflowsDebugDryRun  bool

	workflowsDebugDBType   string
	workflowsDebugDB       string
	workflowsDebugPostgres string
	workflowsDebugMySQL    string
	workflowsDebugMemory   bool
)

var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "Manage the workflows stored in the configured database",
	Long: `Manage the workflows stored in the configured database.

The commands below open the same storage backend as 'm9m serve' - the
--mysql / --postgres / --db flags and the M9M_MYSQL_DSN /
M9M_POSTGRES_URL environment variables are honoured with the same
precedence.`,
}

var workflowsDebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Turn the per-workflow Debug flag on or off for the stored workflows",
	Long: `Turn the per-workflow Debug flag on or off for the stored workflows.

The Debug flag controls how much per-node data the execution engine
captures: with Debug=ON every node's input and output is snapshotted into
executions.node_data, with Debug=OFF only the start and the last node are
kept.

This command flips the flag of the workflows that ALREADY exist in the
database. Workflows created afterwards still start with Debug=OFF - the
per-workflow toggle in the editor is unaffected.

The operation is idempotent: a workflow that already carries the target
value is never written, so a second run reports 0 changed workflows. Use
--dry-run first to see the numbers without touching a row.

Examples:
  m9m workflows debug --enable --all --dry-run   Preview the change
  m9m workflows debug --enable --all             Turn Debug ON everywhere
  m9m workflows debug --disable --all            Turn Debug OFF everywhere
  m9m workflows debug --enable --all --mysql "$M9M_MYSQL_DSN"`,
	RunE: runWorkflowsDebug,
}

func init() {
	workflowsDebugCmd.Flags().BoolVar(&workflowsDebugEnable, "enable", false, "Turn Debug ON (default)")
	workflowsDebugCmd.Flags().BoolVar(&workflowsDebugDisable, "disable", false, "Turn Debug OFF")
	workflowsDebugCmd.Flags().BoolVar(&workflowsDebugAll, "all", false, "Apply the change to every existing workflow (required)")
	workflowsDebugCmd.Flags().BoolVar(&workflowsDebugDryRun, "dry-run", false, "Report what would change without writing anything")

	workflowsDebugCmd.Flags().StringVar(&workflowsDebugDBType, "db-type", "", "Force storage backend: sqlite|memory|postgres|mysql (overrides other flags and M9M_DB_TYPE)")
	workflowsDebugCmd.Flags().StringVar(&workflowsDebugDB, "db", "", "SQLite database path")
	workflowsDebugCmd.Flags().StringVar(&workflowsDebugPostgres, "postgres", "", "PostgreSQL connection URL")
	workflowsDebugCmd.Flags().StringVar(&workflowsDebugMySQL, "mysql", "", "MySQL connection DSN")
	workflowsDebugCmd.Flags().BoolVar(&workflowsDebugMemory, "memory", false, "Use in-memory storage (no CGO required, data lost on restart)")

	workflowsCmd.AddCommand(workflowsDebugCmd)
}

// runWorkflowsDebug flips the Debug flag of every stored workflow.
func runWorkflowsDebug(cmd *cobra.Command, args []string) error {
	if workflowsDebugEnable && workflowsDebugDisable {
		return fmt.Errorf("--enable and --disable are mutually exclusive")
	}
	if !workflowsDebugAll {
		return fmt.Errorf("--all is required: this command only operates on every existing workflow")
	}

	// --enable is the default; --disable turns the flag off again.
	enabled := !workflowsDebugDisable

	store, backend, err := openStorage(storageOptions{
		DBType:      workflowsDebugDBType,
		DBPath:      workflowsDebugDB,
		PostgresURL: workflowsDebugPostgres,
		MySQLDSN:    workflowsDebugMySQL,
		Memory:      workflowsDebugMemory,
		Workspace:   GetWorkspace(),
	})
	if err != nil {
		return fmt.Errorf("failed to open the storage backend: %w", err)
	}
	defer store.Close()

	// An empty workspace id means "every workspace".
	workspaceID := GetWorkspace()

	result, err := storage.ApplyDebugToAll(store, workspaceID, enabled, workflowsDebugDryRun)
	if err != nil {
		return err
	}

	state := "ON"
	if !enabled {
		state = "OFF"
	}

	fmt.Printf("Storage backend : %s\n", backend)
	fmt.Printf("Scope           : %s\n", debugScopeDescription(workspaceID))
	fmt.Printf("Workflows       : %d\n", result.Total)
	fmt.Printf("Debug ON        : %d\n", result.Enabled)
	fmt.Printf("Debug OFF       : %d\n", result.Disabled)
	if result.DryRun {
		fmt.Printf("Mode            : dry-run (nothing was written)\n")
		fmt.Printf("Would change    : %d workflow(s) to Debug %s\n", result.Changed, state)
	} else {
		fmt.Printf("Changed         : %d workflow(s) to Debug %s\n", result.Changed, state)
	}

	return nil
}

// debugScopeDescription renders the workspace scope for the summary.
func debugScopeDescription(workspaceID string) string {
	if workspaceID == "" {
		return "all workspaces"
	}
	return "workspace " + workspaceID
}
