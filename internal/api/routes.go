package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/ai"
	"github.com/neul-labs/m9m/internal/otel"
)

// RegisterRoutes registers all API routes.
func (s *APIServer) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/health", s.HealthCheck).Methods("GET")
	router.HandleFunc("/healthz", s.HealthCheck).Methods("GET")
	router.HandleFunc("/ready", s.ReadyCheck).Methods("GET")

	api := router.PathPrefix("/api/v1").Subrouter()

	api.HandleFunc("/workflows", s.ListWorkflows).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/count", s.CountWorkflows).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows", s.CreateWorkflow).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}", s.GetWorkflow).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/{id}", s.UpdateWorkflow).Methods("PUT", "PATCH", "OPTIONS")
	api.HandleFunc("/workflows/{id}", s.DeleteWorkflow).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/workflows/{id}/activate", s.ActivateWorkflow).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/deactivate", s.DeactivateWorkflow).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/execute", s.ExecuteWorkflow).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/execute-async", s.ExecuteWorkflowAsync).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/duplicate", s.DuplicateWorkflow).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/run", s.ExecuteWorkflowByDefinition).Methods("POST", "OPTIONS")

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
	api.HandleFunc("/credentials/schema/{type}", s.GetCredentialSchema).Methods("GET", "OPTIONS")
	api.HandleFunc("/credentials/{id}", s.GetCredential).Methods("GET", "OPTIONS")
	api.HandleFunc("/credentials/{id}", s.UpdateCredential).Methods("PUT", "OPTIONS")
	api.HandleFunc("/credentials/{id}", s.PatchCredential).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/credentials/{id}", s.DeleteCredential).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/credentials/{id}/test", s.TestCredential).Methods("POST", "OPTIONS")
	api.HandleFunc("/credentials/{id}/transfer", s.TransferCredential).Methods("PUT", "OPTIONS")

	api.HandleFunc("/node-types", s.ListNodeTypes).Methods("GET", "OPTIONS")
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
