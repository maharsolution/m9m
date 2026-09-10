package commands

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"github.com/neul-labs/m9m/internal/api"
	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/nodes/ai"
	"github.com/neul-labs/m9m/internal/otel"
	"github.com/neul-labs/m9m/internal/queue"
	"github.com/neul-labs/m9m/internal/scheduler"
	"github.com/neul-labs/m9m/internal/storage"
	"github.com/neul-labs/m9m/internal/web"
	"github.com/neul-labs/m9m/internal/webhooks"
	"github.com/neul-labs/m9m/internal/workspace"
)

var (
	servePort        int
	serveHost        string
	serveMetricsPort int
	serveDevMode     bool
	serveDB          string
	servePostgres    string
	serveMySQL       string
	serveDBType      string
	serveMemory      bool
	serveQueueType   string
	serveQueueDB     string
	serveWorkers     int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the m9m server",
	Long: `Start the m9m server with REST API and optional web UI.

This runs m9m as a full server, similar to n8n, with:
- REST API for workflow management
- Web UI (if built)
- Webhook handling
- Scheduled workflow execution

Examples:
  m9m serve                      Start on default port 8080
  m9m serve --port 3000          Start on custom port
  m9m serve --dev                Start in development mode
  m9m serve --db ./data/m9m.db   Use specific database`,
	Run: runServe,
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "Server port")
	serveCmd.Flags().StringVar(&serveHost, "host", "0.0.0.0", "Server host")
	serveCmd.Flags().IntVar(&serveMetricsPort, "metrics-port", 0, "Metrics port (0 = disabled)")
	serveCmd.Flags().BoolVar(&serveDevMode, "dev", false, "Enable development mode (permissive CORS)")
	serveCmd.Flags().StringVar(&serveDB, "db", "", "SQLite database path")
	serveCmd.Flags().StringVar(&servePostgres, "postgres", "", "PostgreSQL connection URL")
	serveCmd.Flags().StringVar(&serveMySQL, "mysql", "", "MySQL connection DSN (e.g. user:pass@tcp(127.0.0.1:3306)/m9m?parseTime=true&charset=utf8mb4)")
	serveCmd.Flags().StringVar(&serveDBType, "db-type", "", "Force storage backend: sqlite|memory|postgres|mysql (overrides other flags and M9M_DB_TYPE)")
	serveCmd.Flags().BoolVar(&serveMemory, "memory", false, "Use in-memory storage (no CGO required, data lost on restart)")
	serveCmd.Flags().StringVar(&serveQueueType, "queue", "sqlite", "Queue type: memory, sqlite")
	serveCmd.Flags().StringVar(&serveQueueDB, "queue-db", "", "Queue SQLite database path (for sqlite queue)")
	serveCmd.Flags().IntVar(&serveWorkers, "workers", 4, "Number of worker threads for job processing")
}

func runServe(cmd *cobra.Command, args []string) {
	logger := log.New(os.Stderr, "[m9m] ", log.LstdFlags)

	logger.Printf("Starting m9m server v%s", version)

	// Initialize storage. Resolution order:
	//   1. Explicit --db-type flag (forces a backend; required DSN via other flags)
	//   2. M9M_DB_TYPE env var (same semantics as --db-type)
	//   3. Heuristic: --mysql / M9M_MYSQL_DSN → --postgres / M9M_POSTGRES_URL → --memory → SQLite (default)
	dbType := strings.ToLower(strings.TrimSpace(firstNonEmpty(serveDBType, os.Getenv("M9M_DB_TYPE"))))
	mysqlDSN := firstNonEmpty(serveMySQL, os.Getenv("M9M_MYSQL_DSN"))
	postgresURL := firstNonEmpty(servePostgres, os.Getenv("M9M_POSTGRES_URL"), os.Getenv("M9M_POSTGRES_DSN"))

	var store storage.WorkflowStorage
	var err error

	switch {
	case dbType == "mysql" || (dbType == "" && mysqlDSN != ""):
		if mysqlDSN == "" {
			logger.Fatalf("MySQL storage requested but no DSN provided. Set --mysql or M9M_MYSQL_DSN.")
		}
		logger.Printf("Using MySQL storage")
		store, err = storage.NewMySQLStorage(mysqlDSN)
	case dbType == "postgres" || dbType == "postgresql" || (dbType == "" && postgresURL != ""):
		if postgresURL == "" {
			logger.Fatalf("PostgreSQL storage requested but no URL provided. Set --postgres, M9M_POSTGRES_URL, or M9M_POSTGRES_DSN.")
		}
		logger.Printf("Using PostgreSQL storage")
		store, err = storage.NewPostgresStorage(postgresURL)
	case dbType == "memory" || serveMemory:
		logger.Printf("Using in-memory storage (data will be lost on restart)")
		store = storage.NewMemoryStorage()
	case dbType == "" || dbType == "sqlite":
		// Use SQLite
		dbPath := serveDB
		if dbPath == "" {
			// Check workspace first
			if workspaceFlag != "" {
				mgr, _ := workspace.NewManager()
				if mgr != nil {
					dbPath, _ = mgr.GetStoragePath(workspaceFlag)
				}
			}
			// Default to data directory
			if dbPath == "" {
				homeDir, _ := os.UserHomeDir()
				dataDir := filepath.Join(homeDir, ".m9m", "data")
				_ = os.MkdirAll(dataDir, 0755)
				dbPath = filepath.Join(dataDir, "m9m.db")
			}
		}
		logger.Printf("Using SQLite storage: %s", dbPath)
		store, err = storage.NewSQLiteStorage(dbPath)
	default:
		logger.Fatalf("Unknown --db-type %q (supported: sqlite, memory, postgres, mysql)", dbType)
	}

	if err != nil {
		logger.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Initialize engine
	eng := engine.NewWorkflowEngine()
	RegisterAllNodes(eng, store)

	// Bootstrap OpenTelemetry. The manager is shared across the engine,
	// the AI agent nodes, and the API server. Config layers env defaults
	// with any DB-stored override. When tracing is disabled (which is
	// the default) every helper on *Manager returns a no-op span, so
	// downstream code stays branch-free.
	otelStore := otel.NewConfigStore(store)
	otelOverride, err := otelStore.LoadOrZero()
	if err != nil {
		logger.Printf("Warning: failed to load OTEL override: %v (falling back to env defaults)", err)
	}
	otelCfg := otel.EffectiveConfig(otelOverride)
	otelManager, err := otel.NewManager(context.Background(), otelCfg)
	if err != nil {
		logger.Printf("Warning: failed to initialise OTEL pipeline: %v (continuing with no tracing)", err)
		otelManager = nil
	}
	// otelManager may legitimately be nil here (build error); it is
	// also allowed to be non-nil with Enabled=false, which is the
	// normal "tracing off at boot" case the UI can flip on later.
	if otelManager != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelManager.Shutdown(shutdownCtx); err != nil {
				logger.Printf("OTEL shutdown error: %v", err)
			}
		}()
		logger.Printf("OpenTelemetry initialised: enabled=%t protocol=%s endpoint=%s", otelCfg.Enabled, otelCfg.Protocol, otelCfg.Endpoint)
	}

	// Hand the OTEL manager to the engine and the AI agent nodes so
	// workflow.execute / node.execute / <agent>.generate spans all
	// chain onto the same tree.
	if otelManager != nil {
		if mw, ok := eng.(interface {
			SetOTelManager(*otel.Manager)
		}); ok {
			mw.SetOTelManager(otelManager)
		}
		// AI agent nodes: ask the engine registry for the executor
		// and inject the manager. We tolerate missing executors
		// (e.g. when a build excludes OpenAI) so the test harnesses
		// can still run.
		if node, err := eng.GetNodeExecutor("n8n-nodes-base.openAi"); err == nil {
			if aiNode, ok := node.(*ai.OpenAINode); ok {
				aiNode.SetOTelManager(otelManager)
			}
		}
		if node, err := eng.GetNodeExecutor("n8n-nodes-base.anthropic"); err == nil {
			if aiNode, ok := node.(*ai.AnthropicNode); ok {
				aiNode.SetOTelManager(otelManager)
			}
		}
	}

	// Initialize credential manager
	credMgr, err := credentials.NewCredentialManager()
	if err != nil {
		logger.Printf("Warning: Failed to initialize credential manager: %v", err)
	} else {
		// Seed the in-memory credential cache from persistent storage so
		// credentials written by the API (and the n8n sync bridge) after
		// server startup are visible to the workflow engine. Without this
		// seed, GetCredential returns "not found" for synced credentials
		// and credential-injected node params stay empty at validation
		// time.
		if loadErr := credMgr.LoadFromStorage(store); loadErr != nil {
			logger.Printf("Warning: Failed to seed credential cache from storage: %v", loadErr)
		} else {
			logger.Println("Credential cache seeded from storage")
		}
		eng.SetCredentialManager(credMgr)
	}

	// Initialize job queue
	var jobQueue queue.JobQueue
	switch serveQueueType {
	case "memory":
		logger.Println("Using in-memory job queue (jobs will be lost on restart)")
		jobQueue = queue.NewMemoryJobQueue(1000)
	case "sqlite":
		queueDBPath := serveQueueDB
		if queueDBPath == "" {
			// Default to data directory
			homeDir, _ := os.UserHomeDir()
			dataDir := filepath.Join(homeDir, ".m9m", "data")
			_ = os.MkdirAll(dataDir, 0755)
			queueDBPath = filepath.Join(dataDir, "queue.db")
		}
		logger.Printf("Using SQLite job queue: %s", queueDBPath)
		var queueErr error
		jobQueue, queueErr = queue.NewSQLiteJobQueue(queueDBPath, 1000)
		if queueErr != nil {
			logger.Fatalf("Failed to initialize job queue: %v", queueErr)
		}
	default:
		logger.Fatalf("Unknown queue type: %s (supported: memory, sqlite)", serveQueueType)
	}
	defer jobQueue.Close()

	// Initialize worker pool
	workerPool := queue.NewWorkerPool(jobQueue, eng, serveWorkers)
	workerPool.Start()
	defer workerPool.Stop()
	logger.Printf("Started %d workers for job processing", serveWorkers)

	// Initialize scheduler
	sched := scheduler.NewWorkflowScheduler(eng)
	_ = sched.Start()
	defer func() {
		_ = sched.Stop()
	}()

	// Create API server
	apiConfig := &api.APIServerConfig{
		DevMode:            serveDevMode,
		MaxPaginationLimit: 100,
	}

	if serveDevMode {
		apiConfig.AllowedOrigins = []string{"*"}
		logger.Println("Development mode enabled (permissive CORS)")
	}

	apiServer := api.NewAPIServerWithConfig(eng, sched, store, apiConfig)
	apiServer.SetJobQueue(jobQueue)
	// Wire the OTEL manager into the API server so PUT /api/v1/otel
	// can hot-reload the tracer provider without a restart. The
	// manager is non-nil in both "enabled" and "disabled at boot"
	// cases — only a build error above leaves it nil.
	if otelManager != nil {
		apiServer.SetOTelManager(otelManager, otelStore)
	}

	// Setup router
	router := mux.NewRouter()

	// Register API routes
	apiServer.RegisterRoutes(router)

	// Initialize webhook manager + handler (persistent so webhooks survive restarts)
	webhookStore := webhooks.NewPersistentWebhookStorage(store)
	webhookManager := webhooks.NewWebhookManager(webhookStore, store, eng)
	if err := webhookManager.LoadActiveWebhooks(); err != nil {
		logger.Printf("Warning: failed to load active webhooks: %v", err)
	}
	// Wire webhook manager into API server so activate/deactivate keeps
	// the activeHooks cache in sync. Without this, POST /webhook/{path}
	// returns "Webhook not found" until server restart.
	apiServer.SetWebhookManager(webhookManager)
	webhookHandler := webhooks.NewHandler(webhookManager)
	webhookHandler.RegisterRoutes(router)

	// Serve embedded web UI
	// In dev mode, you can pass a path to serve from filesystem for hot-reload
	var webDevPath string
	if serveDevMode {
		// Check if there's a local web/dist folder for development
		if _, err := os.Stat("web/dist"); err == nil {
			webDevPath = "web/dist"
		}
	}
	webHandler := web.NewHandler(webDevPath)
	webHandler.RegisterRoutes(router)

	logger.Printf("Web UI enabled (embedded: %v)", webDevPath == "")

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", serveHost, servePort)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start metrics server if enabled
	if serveMetricsPort > 0 {
		go startMetricsServer(serveMetricsPort, logger)
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Println("Shutting down server...")

		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("Error during shutdown: %v", err)
		}

		cancel()
	}()

	// Start server
	nodeTypes := eng.GetRegisteredNodeTypes()
	logger.Printf("Server listening on http://%s", addr)
	logger.Printf("Registered nodes: %d", len(nodeTypes))
	logger.Printf("Queue: %s | Workers: %d", serveQueueType, serveWorkers)
	logger.Printf("Web UI: http://%s", addr)
	logger.Printf("API: http://%s/api/v1", addr)
	logger.Printf("Health: http://%s/health", addr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Server error: %v", err)
	}

	logger.Println("Server stopped")
}

// firstNonEmpty returns the first non-empty value among the inputs. Used to
// resolve CLI flag > env var precedence when wiring storage backends.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func startMetricsServer(port int, logger *log.Logger) {
	router := mux.NewRouter()
	router.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Prometheus-compatible metrics
		fmt.Fprintln(w, "# HELP m9m_up Whether the m9m server is up")
		fmt.Fprintln(w, "# TYPE m9m_up gauge")
		fmt.Fprintln(w, "m9m_up 1")
	})

	addr := fmt.Sprintf(":%d", port)
	logger.Printf("Metrics server listening on %s", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		logger.Printf("Metrics server error: %v", err)
	}
}

