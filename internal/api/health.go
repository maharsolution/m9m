package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/storage"
)

func (s *APIServer) HealthCheck(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"service": "m9m",
		"version": "1.0.0",
		"tagline": "Agent-Native Workflow Automation",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) ReadyCheck(w http.ResponseWriter, r *http.Request) {
	ready := s.engine != nil && s.storage != nil

	if ready {
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ready",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	s.sendJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
		"status": "not ready",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) GetVersion(w http.ResponseWriter, r *http.Request) {
	// Default n8n-compatible serverVersion is preserved so existing
	// consumers (the parity suite, the n8n sync bridge, the MCP server)
	// keep working unchanged. The three new fields below — version,
	// commit, buildDate — are the source identity stamped into the
	// binary by Dockerfile -ldflags. The frontend sidebar uses them to
	// render "v<version> @ <commit> • <buildDate>" so a user can verify
	// at a glance whether the container is serving the latest push.
	resp := map[string]interface{}{
		"n8nVersion":     "1.0.0-compatible",
		"serverVersion":  "0.2.0",
		"implementation": "m9m",
		"compatibility": map[string]interface{}{
			"workflows":   true,
			"nodes":       true,
			"expressions": true,
			"credentials": true,
		},
	}

	// Surface the running binary's identity. Populated by
	// SetBuildInfo at server startup; "unknown" when SetBuildInfo was
	// never called (e.g. running via `go run` for local dev or in unit
	// tests that don't bother wiring it).
	if s.version != "" {
		resp["version"] = s.version
	} else {
		resp["version"] = "unknown"
	}
	if s.commit != "" {
		resp["commit"] = s.commit
	} else {
		resp["commit"] = "unknown"
	}
	if s.buildDate != "" {
		resp["buildDate"] = s.buildDate
	} else {
		resp["buildDate"] = "unknown"
	}

	s.sendJSON(w, http.StatusOK, resp)
}

func (s *APIServer) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings := map[string]interface{}{
		"timezone":                 "UTC",
		"executionMode":            "regular",
		"saveDataSuccessExecution": "all",
		"saveDataErrorExecution":   "all",
		"saveExecutionProgress":    true,
		"saveManualExecutions":     true,
		"communityNodesEnabled":    false,
		"versionNotifications": map[string]bool{
			"enabled": false,
		},
		"instanceId": "m9m-instance",
		"telemetry": map[string]bool{
			"enabled": false,
		},
	}

	if data, err := s.storage.GetRaw("settings:system"); err == nil && len(data) > 0 {
		var persistedSettings map[string]interface{}
		if err := json.Unmarshal(data, &persistedSettings); err == nil {
			for key, value := range persistedSettings {
				settings[key] = value
			}
		}
	}

	s.sendJSON(w, http.StatusOK, settings)
}

func (s *APIServer) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var newSettings map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&newSettings); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	existingSettings := make(map[string]interface{})
	if data, err := s.storage.GetRaw("settings:system"); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &existingSettings)
	}

	for key, value := range newSettings {
		existingSettings[key] = value
	}

	data, err := json.Marshal(existingSettings)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to serialize settings", err)
		return
	}

	if err := s.storage.SaveRaw("settings:system", data); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save settings", err)
		return
	}

	s.sendJSON(w, http.StatusOK, existingSettings)
}

func (s *APIServer) GetLicense(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"licensed":    false,
		"licenseType": "community",
		"features":    []string{},
		"expiresAt":   nil,
		"message":     "License management is an enterprise feature",
	})
}

func (s *APIServer) GetLDAP(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":    false,
		"configured": false,
		"message":    "LDAP is an enterprise feature",
	})
}

func (s *APIServer) ListNodeTypes(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		s.sendJSON(w, http.StatusOK, []map[string]interface{}{})
		return
	}

	registered := s.engine.GetRegisteredNodeTypes()
	nodeTypes := make([]map[string]interface{}, 0, len(registered))
	for _, nt := range registered {
		entry := map[string]interface{}{
			"name":        nt.TypeID,
			"displayName": nt.DisplayName,
			"description": nt.Description,
			"category":    nt.Category,
			"version":     nt.Version,
			"defaults": map[string]interface{}{
				"name": nt.DisplayName,
			},
			"inputs":  nt.Inputs,
			"outputs": nt.Outputs,
		}
		if len(nt.Properties) > 0 {
			entry["properties"] = nt.Properties
		}
		nodeTypes = append(nodeTypes, entry)
	}

	s.sendJSON(w, http.StatusOK, nodeTypes)
}

func (s *APIServer) GetNodeType(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	if s.engine == nil {
		s.sendJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": fmt.Sprintf("node type not found: %s", name),
		})
		return
	}

	for _, nt := range s.engine.GetRegisteredNodeTypes() {
		if nt.TypeID == name {
			s.sendJSON(w, http.StatusOK, map[string]interface{}{
				"name":        nt.TypeID,
				"displayName": nt.DisplayName,
				"description": nt.Description,
				"category":    nt.Category,
				"version":     nt.Version,
				"defaults": map[string]interface{}{
					"name": nt.DisplayName,
				},
				"inputs":     nt.Inputs,
				"outputs":    nt.Outputs,
				"properties": nt.Properties,
			})
			return
		}
	}

	s.sendJSON(w, http.StatusNotFound, map[string]interface{}{
		"error": fmt.Sprintf("node type not found: %s", name),
	})
}

func (s *APIServer) GetMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.scheduler.GetMetrics()
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"scheduler": metrics,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) DetailedHealth(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"service": "m9m",
		"version": "1.0.0",
		"components": map[string]interface{}{
			"engine": map[string]interface{}{
				"status":  "healthy",
				"message": "Workflow engine operational",
			},
			"storage": map[string]interface{}{
				"status":  "healthy",
				"message": "Storage backend connected",
			},
			"scheduler": map[string]interface{}{
				"status":  "healthy",
				"message": "Scheduler running",
			},
		},
		"uptime": "Running",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) GetPerformanceStats(w http.ResponseWriter, r *http.Request) {
	// Real metrics from storage. Everything here comes from saved workflows
	// and executions — no hard-coded marketing numbers.
	//
	// We compute averages over up to 200 most-recent executions and use
	// the full stored counts for "Total Executions" / "Active Workflows".
	// Any failure (e.g. before storage is initialized) collapses to
	// null values so the frontend can show "No data yet" instead of a
	// random fallback.

	metrics := map[string]interface{}{
		"avgExecutionTime":  nil,
		"successRate":       nil,
		"totalExecutions":   0,
		"activeWorkflows":   0,
		"failedExecutions":  0,
		"sampledExecutions": 0,
		"sampleWindow":      "200 most recent",
		"generatedAt":       time.Now().UTC().Format(time.RFC3339),
	}

	if s.storage == nil {
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"metrics":     metrics,
			"improvement": map[string]interface{}{},
		})
		return
	}

	// Active workflows = stored workflows with active == true.
	activeTrue := true
	activeWorkflows, err := s.storage.CountWorkflows(storage.WorkflowFilters{Active: &activeTrue})
	if err == nil {
		metrics["activeWorkflows"] = activeWorkflows
	}

	// Total executions (across all status values).
	totalExec, err := s.storage.CountExecutions(storage.ExecutionFilters{})
	if err == nil {
		metrics["totalExecutions"] = totalExec
	}

	// Failed executions (status == "error" or "failed" — both spellings
	// can appear depending on engine version).
	failed := 0
	for _, st := range []string{"error", "failed"} {
		n, err := s.storage.CountExecutions(storage.ExecutionFilters{Status: st})
		if err == nil {
			failed += n
		}
	}
	metrics["failedExecutions"] = failed

	// Average duration + success rate from the last 200 executions.
	recent, err := s.storage.RecentExecutions(storage.ExecutionFilters{}, 200)
	if err == nil && len(recent) > 0 {
		var totalMs float64
		var withDuration int
		var succeeded int
		for _, e := range recent {
			if e.FinishedAt != nil && !e.StartedAt.IsZero() {
				dur := e.FinishedAt.Sub(e.StartedAt).Milliseconds()
				if dur >= 0 {
					totalMs += float64(dur)
					withDuration++
				}
			}
			if e.Status == "success" || e.Status == "completed" {
				succeeded++
			}
		}
		if withDuration > 0 {
			avgMs := totalMs / float64(withDuration)
			metrics["avgExecutionTime"] = map[string]interface{}{
				"ms":          avgMs,
				"display":     formatDuration(avgMs),
				"sampleCount": withDuration,
			}
		}
		metrics["sampledExecutions"] = len(recent)
		metrics["successRate"] = map[string]interface{}{
			"percent":     float64(succeeded) / float64(len(recent)) * 100,
			"succeeded":   succeeded,
			"sampleCount": len(recent),
		}
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"metrics":     metrics,
		"improvement": map[string]interface{}{},
	})
}

// formatDuration converts a millisecond float into a human-readable
// short string ("42ms", "1.2s", "1m 4s") for the Performance page.
func formatDuration(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%.0fms", ms)
	}
	if ms < 60_000 {
		return fmt.Sprintf("%.2fs", ms/1000)
	}
	minutes := int(ms / 60_000)
	seconds := int((ms - float64(minutes)*60_000) / 1000)
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
