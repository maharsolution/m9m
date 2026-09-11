package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/neul-labs/m9m/internal/ai"
	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/otel"
	"github.com/neul-labs/m9m/internal/queue"
	"github.com/neul-labs/m9m/internal/scheduler"
	"github.com/neul-labs/m9m/internal/storage"
	"github.com/neul-labs/m9m/internal/webhooks"
)

// APIServerConfig configures the API server
type APIServerConfig struct {
	// AllowedOrigins for WebSocket CORS (comma-separated in env)
	AllowedOrigins []string
	// DevMode enables permissive security settings
	DevMode bool
	// MaxPaginationLimit caps the number of items per page
	MaxPaginationLimit int
}

// DefaultAPIServerConfig returns default configuration from environment
func DefaultAPIServerConfig() *APIServerConfig {
	config := &APIServerConfig{
		DevMode:            os.Getenv("M9M_DEV_MODE") == "true",
		MaxPaginationLimit: 100,
	}

	// Parse allowed origins from environment
	originsEnv := os.Getenv("M9M_ALLOWED_ORIGINS")
	if originsEnv != "" {
		config.AllowedOrigins = strings.Split(originsEnv, ",")
		for i := range config.AllowedOrigins {
			config.AllowedOrigins[i] = strings.TrimSpace(config.AllowedOrigins[i])
		}
	}

	return config
}

// APIServer provides REST API with workflow compatibility
type APIServer struct {
	engine         engine.WorkflowEngine
	scheduler      *scheduler.WorkflowScheduler
	storage        storage.WorkflowStorage
	jobQueue       queue.JobQueue
	upgrader       websocket.Upgrader
	wsClients      map[string]*websocket.Conn
	config         *APIServerConfig
	webhookManager *webhooks.WebhookManager

	// credMgr is the in-memory credential store the workflow
	// engine reads from at execution time. The API handlers
	// write through both `storage` (DB) and this manager so
	// credentials created or updated after server startup are
	// visible to the engine immediately — without going through
	// a full LoadFromStorage round trip. Set via
	// SetCredentialManager from cmd/m9m/commands/serve.go.
	credMgr *credentials.CredentialManager

	otelManager *otel.Manager
	otelStore   *otel.ConfigStore

	// aiStore persists user-edited AI config overrides (env-default +
	// DB-override). aiRuntime is the live *AI service the handlers
	// reach into; both can be nil when the AI subsystem is disabled.
	aiStore   *ai.ConfigStore
	aiRuntime *ai.Runtime

	// version / commit / buildDate identify the running binary's source.
	// Set via SetBuildInfo from cmd/m9m/commands/serve.go so the
	// /api/v1/version endpoint can tell the UI which exact source is
	// serving requests — letting users distinguish "I rebuilt and pulled
	// the fresh image" from "the container is still on the old image".
	// All three default to "unknown" so callers that never call
	// SetBuildInfo (tests, etc.) still get a valid JSON response.
	version    string
	commit     string
	buildDate  string

	executionMu      sync.RWMutex
	executionCancels map[string]context.CancelFunc
}

// NewAPIServer creates a new API server instance
func NewAPIServer(eng engine.WorkflowEngine, scheduler *scheduler.WorkflowScheduler, storage storage.WorkflowStorage) *APIServer {
	return NewAPIServerWithConfig(eng, scheduler, storage, DefaultAPIServerConfig())
}

// NewAPIServerWithConfig creates a new API server with custom configuration
func NewAPIServerWithConfig(eng engine.WorkflowEngine, scheduler *scheduler.WorkflowScheduler, storage storage.WorkflowStorage, config *APIServerConfig) *APIServer {
	if config == nil {
		config = DefaultAPIServerConfig()
	}

	server := &APIServer{
		engine:    eng,
		scheduler: scheduler,
		storage:   storage,
		wsClients: make(map[string]*websocket.Conn),
		config:    config,

		executionCancels: make(map[string]context.CancelFunc),
	}

	// Configure WebSocket upgrader with proper CORS
	server.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// In dev mode, allow all origins
			if config.DevMode {
				return true
			}

			// Check if origin is in allowed list
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // Same-origin requests have no Origin header
			}

			for _, allowed := range config.AllowedOrigins {
				if origin == allowed {
					return true
				}
			}
			return false
		},
	}

	return server
}

// SetJobQueue sets the job queue for async execution
func (s *APIServer) SetJobQueue(jq queue.JobQueue) {
	s.jobQueue = jq
}

// SetWebhookManager wires the webhook manager so workflow activation/deactivation
// keeps the activeHooks cache in sync. Without this, /webhook/{path} requests
// return "Webhook not found" even after the workflow is activated.
func (s *APIServer) SetWebhookManager(wm *webhooks.WebhookManager) {
	s.webhookManager = wm
}

// SetOTelManager wires the OpenTelemetry Manager so /api/v1/otel can
// hot-reload the tracer provider on PUT. Calling this is optional —
// when the manager is nil, the API still serves GET and PUT but PUT
// only persists the override without reloading the tracer.
func (s *APIServer) SetOTelManager(m *otel.Manager, store *otel.ConfigStore) {
	s.otelManager = m
	s.otelStore = store
}

// SetCredentialManager wires the in-memory credential store the
// engine reads from at execution time. The credential create /
// update / delete handlers write through to this manager in
// addition to the DB, so a credential added via the UI is visible
// to running workflows without waiting for a server restart or
// the next LoadFromStorage refresh.
func (s *APIServer) SetCredentialManager(cm *credentials.CredentialManager) {
	s.credMgr = cm
}

// SetBuildInfo records the running binary's source identity so
// /api/v1/version can return it. The frontend sidebar footer reads
// these fields to render "v1.0.0 @ <commit> • <date>" — without them,
// users can't tell whether the container they're hitting was rebuilt
// from the latest push or is still running the previous image.
//
// All three arguments are expected to be non-empty (the Dockerfile
// stamps COMMIT + BUILD_DATE via -ldflags); an empty value falls back
// to "unknown" so the response is always JSON-valid.
func (s *APIServer) SetBuildInfo(version, commit, buildDate string) {
	if version == "" {
		version = "unknown"
	}
	if commit == "" {
		commit = "unknown"
	}
	if buildDate == "" {
		buildDate = "unknown"
	}
	s.version = version
	s.commit = commit
	s.buildDate = buildDate
}

// SetAI wires the AI config store and runtime so the /api/v1/ai and
// /api/v1/ai/{generate,suggest,explain,fix,chat} handlers can serve
// requests. Both arguments may be nil — when only store is supplied,
// PUT/DELETE will persist changes without live-reloading the AI
// service (useful for tests).
func (s *APIServer) SetAI(store *ai.ConfigStore, runtime *ai.Runtime) {
	s.aiStore = store
	s.aiRuntime = runtime
}
