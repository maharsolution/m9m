package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/ai"
	"github.com/neul-labs/m9m/internal/otel"
	"github.com/neul-labs/m9m/internal/versions"
)

// versionManager is the workflow-version manager used by the
// "/workflows/{id}/versions" routes. It is injected by
// cmd/m9m/commands/serve.go through SetVersionManager before the
// routes are registered.
//
// It lives in a package-level slot because the APIServer struct — and
// therefore the setter methods that go with it — is defined in
// internal/api/server.go, which is maintained by another worker. The
// manager is set once at startup, so a single process only ever has
// one instance.
var versionManager *versions.VersionManager

// SetVersionManager wires the workflow-version manager that serves
// the "/api/v1/workflows/{id}/versions" routes. Passing nil keeps the
// routes registered but answers 503 (the manager was never wired).
func SetVersionManager(manager *versions.VersionManager) {
	versionManager = manager
}

// registerVersionRoutes mounts the workflow-version routes on the
// /api/v1 subrouter.
//
// The routes live in the same ordered route table as the rest of the
// API, which is what guarantees that "/workflows/{id}/versions" is
// matched before the "/workflows/{id}" wildcard. Inside the handler the
// static "/versions/compare" segment is registered before the
// "/versions/{versionId}" wildcard for the same reason.
//
// When no manager was wired (cmd/m9m/commands/serve.go always wires
// one) the routes still exist but answer 503 — the UI can tell "not
// configured" apart from "no such endpoint".
func registerVersionRoutes(api *mux.Router, manager *versions.VersionManager) {
	if manager == nil {
		api.HandleFunc("/workflows/{id}/versions/compare", versionManagerUnavailable).Methods("GET", "OPTIONS")
		api.HandleFunc("/workflows/{id}/versions", versionManagerUnavailable).Methods("GET", "POST", "OPTIONS")
		api.HandleFunc("/workflows/{id}/versions/{versionId}", versionManagerUnavailable).Methods("GET", "DELETE", "OPTIONS")
		api.HandleFunc("/workflows/{id}/versions/{versionId}/restore", versionManagerUnavailable).Methods("POST", "OPTIONS")
		return
	}

	handler := versions.NewVersionHandler(manager)
	handler.SetAuthorResolver(resolveVersionAuthor)
	handler.RegisterRoutesOn(api)
}

// resolveVersionAuthor attributes a created or restored version to the
// authenticated user. The version handler falls back to "system" when
// the resolver returns an empty string.
func resolveVersionAuthor(r *http.Request) string {
	if auth := AuthFromContext(r.Context()); auth != nil {
		return auth.GetUserID()
	}
	return ""
}

// versionManagerUnavailable answers requests that arrive before the
// version manager has been wired.
func versionManagerUnavailable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"error":true,"message":"workflow version manager not initialised","code":503}`))
}

// RegisterRoutes registers all API routes.
func (s *APIServer) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/health", s.HealthCheck).Methods("GET")
	router.HandleFunc("/healthz", s.HealthCheck).Methods("GET")
	router.HandleFunc("/ready", s.ReadyCheck).Methods("GET")

	api := router.PathPrefix("/api/v1").Subrouter()

	api.HandleFunc("/workflows", s.ListWorkflows).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/count", s.CountWorkflows).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows", s.CreateWorkflow).Methods("POST", "OPTIONS")
	// Import/export routes must be registered BEFORE "/workflows/{id}":
	// gorilla/mux matches routes in registration order, so
	// "/workflows/export" would otherwise be swallowed by the
	// "/workflows/{id}" route. The same holds for every other static
	// path under "/workflows/" ("/workflows/run").
	api.HandleFunc("/workflows/export", s.ExportWorkflows).Methods("GET", "POST", "OPTIONS")
	api.HandleFunc("/workflows/import", s.ImportWorkflows).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/run", s.ExecuteWorkflowByDefinition).Methods("POST", "OPTIONS")
	// Workflow versions are registered BEFORE the "/workflows/{id}"
	// wildcard group: gorilla/mux matches routes in registration
	// order, so the collection route must never be shadowed by a
	// shorter wildcard. The handler itself keeps the static
	// "/versions/compare" segment ahead of "/versions/{versionId}".
	registerVersionRoutes(api, versionManager)
	// Workflow ↔ tag assignment is registered in the same block for
	// the same reason: "/workflows/{id}/tags" must be matched before
	// any "/workflows/{id}/…" wildcard can swallow it.
	api.HandleFunc("/workflows/{id}/tags", s.GetWorkflowTags).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/{id}/tags", s.SetWorkflowTags).Methods("POST", "PUT", "OPTIONS")
	api.HandleFunc("/workflows/{id}/tags/{tagId}", s.AddWorkflowTag).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/tags/{tagId}", s.RemoveWorkflowTag).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/workflows/{id}/export", s.ExportWorkflow).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/{id}", s.GetWorkflow).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/{id}", s.UpdateWorkflow).Methods("PUT", "PATCH", "OPTIONS")
	api.HandleFunc("/workflows/{id}", s.DeleteWorkflow).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/workflows/{id}/activate", s.ActivateWorkflow).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/deactivate", s.DeactivateWorkflow).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/execute", s.ExecuteWorkflow).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/execute-async", s.ExecuteWorkflowAsync).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/duplicate", s.DuplicateWorkflow).Methods("POST", "OPTIONS")

	// Tags — CRUD over the `tags` table, which is the same store the
	// workflow import/export path resolves tag names against
	// (resolveTagID in workflow_import_export.go). A tag created here
	// is therefore immediately usable as a workflow tag.
	//
	// "/tags" (the collection) is registered before "/tags/{id}"
	// (the item) so the collection route is never read as a tag whose
	// id is "tags".
	api.HandleFunc("/tags", s.ListTags).Methods("GET", "OPTIONS")
	api.HandleFunc("/tags", s.CreateTag).Methods("POST", "OPTIONS")
	api.HandleFunc("/tags/{id}", s.GetTag).Methods("GET", "OPTIONS")
	api.HandleFunc("/tags/{id}", s.UpdateTag).Methods("PUT", "PATCH", "OPTIONS")
	api.HandleFunc("/tags/{id}", s.DeleteTag).Methods("DELETE", "OPTIONS")

	api.HandleFunc("/jobs", s.ListJobs).Methods("GET", "OPTIONS")
	api.HandleFunc("/jobs/{id}", s.GetJob).Methods("GET", "OPTIONS")

	api.HandleFunc("/templates", s.ListTemplates).Methods("GET", "OPTIONS")
	api.HandleFunc("/templates/{id}", s.GetTemplate).Methods("GET", "OPTIONS")
	api.HandleFunc("/templates/{id}/apply", s.ApplyTemplate).Methods("POST", "OPTIONS")

	api.HandleFunc("/expressions/evaluate", s.EvaluateExpression).Methods("POST", "OPTIONS")

	api.HandleFunc("/executions", s.CreateExecution).Methods("POST", "OPTIONS")
	api.HandleFunc("/executions", s.ListExecutions).Methods("GET", "OPTIONS")
	api.HandleFunc("/executions/count", s.CountExecutions).Methods("GET", "OPTIONS")
	api.HandleFunc("/executions/recent", s.RecentExecutions).Methods("GET", "OPTIONS")
	api.HandleFunc("/executions/{id}", s.GetExecution).Methods("GET", "OPTIONS")
	api.HandleFunc("/executions/{id}", s.DeleteExecution).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/executions/{id}/retry", s.RetryExecution).Methods("POST", "OPTIONS")
	api.HandleFunc("/executions/{id}/retry-node", s.RetryNode).Methods("POST", "OPTIONS")
	api.HandleFunc("/executions/{id}/cancel", s.CancelExecution).Methods("POST", "OPTIONS")

	api.HandleFunc("/schedules", s.ListSchedules).Methods("GET", "OPTIONS")
	api.HandleFunc("/schedules", s.CreateSchedule).Methods("POST", "OPTIONS")
	api.HandleFunc("/schedules/{id}", s.GetSchedule).Methods("GET", "OPTIONS")
	api.HandleFunc("/schedules/{id}", s.UpdateSchedule).Methods("PUT", "OPTIONS")
	api.HandleFunc("/schedules/{id}", s.DeleteSchedule).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/schedules/{id}/enable", s.EnableSchedule).Methods("POST", "OPTIONS")
	api.HandleFunc("/schedules/{id}/disable", s.DisableSchedule).Methods("POST", "OPTIONS")
	api.HandleFunc("/schedules/{id}/history", s.GetScheduleHistory).Methods("GET", "OPTIONS")

	api.HandleFunc("/credentials", s.ListCredentials).Methods("GET", "OPTIONS")
	api.HandleFunc("/credentials", s.CreateCredential).Methods("POST", "OPTIONS")
	// Static paths must be registered BEFORE the `{id}` pattern —
	// gorilla/mux matches routes in registration order, so
	// "/credentials/test" would otherwise be swallowed by
	// "/credentials/{id}" with id="test".
	api.HandleFunc("/credentials/schema/{type}", s.GetCredentialSchema).Methods("GET", "OPTIONS")
	api.HandleFunc("/credentials/types", s.ListCredentialTypes).Methods("GET", "OPTIONS")
	api.HandleFunc("/credentials/test", s.TestCredentialPayload).Methods("POST", "OPTIONS")
	// Credential export/import (n8n parity): a full workflow export can
	// carry the credential data, encrypted with the instance key, so the
	// bundle can be imported somewhere else and still run. Static paths
	// must precede "/credentials/{id}".
	s.RegisterCredentialExportRoutes(api)
	api.HandleFunc("/credentials/{id}", s.GetCredential).Methods("GET", "OPTIONS")
	api.HandleFunc("/credentials/{id}", s.UpdateCredential).Methods("PUT", "OPTIONS")
	api.HandleFunc("/credentials/{id}", s.PatchCredential).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/credentials/{id}", s.DeleteCredential).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/credentials/{id}/test", s.TestCredential).Methods("POST", "OPTIONS")
	api.HandleFunc("/credentials/{id}/transfer", s.TransferCredential).Methods("PUT", "OPTIONS")

	// Data Tables (n8n-compatible). Storage-backed by the same
	// `dataTable:{tableId}:{rowId}` keys the executor uses, so
	// what sync-service writes here is what the engine reads.
	api.HandleFunc("/data-tables", s.ListDataTables).Methods("GET", "OPTIONS")
	api.HandleFunc("/data-tables/{id}/rows", s.ListDataTableRows).Methods("GET", "OPTIONS")
	api.HandleFunc("/data-tables/{id}/rows", s.BulkInsertDataTableRows).Methods("POST", "OPTIONS")
	api.HandleFunc("/data-tables/{id}/rows/{rowId}", s.DeleteDataTableRow).Methods("DELETE", "OPTIONS")

	api.HandleFunc("/node-types", s.ListNodeTypes).Methods("GET", "OPTIONS")
	// Dynamic option lists must be registered BEFORE the "/node-types/{name}"
	// wildcard — gorilla/mux matches routes in registration order, so the
	// wildcard would otherwise swallow "/node-types/{name}/options".
	api.HandleFunc("/node-types/{name}/options", s.GetNodeTypeOptions).Methods("GET", "POST", "OPTIONS")
	api.HandleFunc("/node-types/{name}", s.GetNodeType).Methods("GET", "OPTIONS")

	api.HandleFunc("/settings", s.GetSettings).Methods("GET", "OPTIONS")
	api.HandleFunc("/settings", s.UpdateSettings).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/settings/license", s.GetLicense).Methods("GET", "OPTIONS")
	api.HandleFunc("/settings/ldap", s.GetLDAP).Methods("GET", "OPTIONS")

	api.HandleFunc("/version", s.GetVersion).Methods("GET", "OPTIONS")
	api.HandleFunc("/metrics", s.GetMetrics).Methods("GET", "OPTIONS")

	api.HandleFunc("/ai/generate", s.AIGenerate).Methods("POST", "OPTIONS")
	api.HandleFunc("/ai/suggest", s.AISuggest).Methods("POST", "OPTIONS")
	api.HandleFunc("/ai/explain", s.AIExplain).Methods("POST", "OPTIONS")
	api.HandleFunc("/ai/fix", s.AIFix).Methods("POST", "OPTIONS")
	api.HandleFunc("/ai/chat", s.AIChat).Methods("POST", "OPTIONS")
	api.HandleFunc("/ai/health", s.AIHealthCheck).Methods("GET", "OPTIONS")

	api.HandleFunc("/dlq", s.ListDLQ).Methods("GET", "OPTIONS")
	api.HandleFunc("/dlq/{id}", s.GetDLQItem).Methods("GET", "OPTIONS")
	api.HandleFunc("/dlq/{id}/retry", s.RetryDLQItem).Methods("POST", "OPTIONS")
	api.HandleFunc("/dlq/{id}/discard", s.DiscardDLQItem).Methods("POST", "OPTIONS")
	api.HandleFunc("/dlq/stats", s.GetDLQStats).Methods("GET", "OPTIONS")

	api.HandleFunc("/health/detailed", s.DetailedHealth).Methods("GET", "OPTIONS")
	api.HandleFunc("/performance", s.GetPerformanceStats).Methods("GET", "OPTIONS")
	api.HandleFunc("/push", s.HandleWebSocket).Methods("GET")

	// OpenTelemetry config — read and write the OTLP pipeline settings
	// from the UI. The handler is created lazily so routes can be
	// registered even when OTEL is disabled; the handler returns 503
	// in that case.
	if s.otelStore != nil {
		otelHandler := otel.NewAPIHandler(s.otelStore, s.otelManager)
		api.HandleFunc("/otel", otelHandler.Get).Methods("GET", "OPTIONS")
		api.HandleFunc("/otel", otelHandler.Put).Methods("PUT", "PATCH", "OPTIONS")
		api.HandleFunc("/otel", otelHandler.Delete).Methods("DELETE", "OPTIONS")
		api.HandleFunc("/otel/test", otelHandler.TestTrace).Methods("POST", "OPTIONS")
	} else {
		api.HandleFunc("/otel", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"otel storage not initialised"}`))
		}).Methods("GET", "PUT", "PATCH", "DELETE", "OPTIONS")
	}

	// AI agent config — env-default with DB-override, same pattern as
	// OTEL above. The handler is mounted only when an AI store is
	// wired in serve.go; otherwise we 503 so the UI can still render
	// the Settings → AI page and tell the operator to restart.
	if s.aiStore != nil {
		aiHandler := ai.NewAPIHandler(s.aiStore, s.aiRuntime)
		api.HandleFunc("/ai", aiHandler.Get).Methods("GET", "OPTIONS")
		api.HandleFunc("/ai", aiHandler.Put).Methods("PUT", "PATCH", "OPTIONS")
		api.HandleFunc("/ai", aiHandler.Delete).Methods("DELETE", "OPTIONS")
		api.HandleFunc("/ai/test", aiHandler.TestChat).Methods("POST", "OPTIONS")
	} else {
		api.HandleFunc("/ai", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"ai storage not initialised"}`))
		}).Methods("GET", "PUT", "PATCH", "DELETE", "OPTIONS")
	}
}
