package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neul-labs/m9m/internal/ai"
	"github.com/neul-labs/m9m/internal/model"
)

// AIGenerate generates a workflow from a natural-language description.
// When the runtime is configured (provider, API key, base URL) it
// forwards the request to the live *AI; otherwise it returns a basic
// skeleton workflow and tells the operator to configure a provider.
func (s *APIServer) AIGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string                 `json:"description"`
		Context     map[string]interface{} `json:"context,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	if req.Description == "" {
		s.sendError(w, http.StatusBadRequest, "Description is required", nil)
		return
	}

	if s.aiRuntime == nil {
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"workflow": skeletonWorkflow(),
			"explanation": "AI assistant not configured. " +
				"Configure an AI provider in Settings → AI to enable full workflow generation.",
			"suggestions": []string{
				"Open Settings → AI and pick a provider (openai, anthropic, minimax, ollama)",
				"Set M9M_AI_API_KEY in your shell if you prefer env-driven config",
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.aiRuntime.Config().Timeout)
	defer cancel()

	reply, err := s.aiRuntime.Current().GenerateWorkflow(ctx, &ai.GenerateWorkflowRequest{
		Description: req.Description,
		Context:     req.Context,
	})
	if err != nil {
		s.sendError(w, http.StatusBadGateway, "AI generation failed: "+err.Error(), err)
		return
	}
	s.sendJSON(w, http.StatusOK, reply)
}

// AISuggest suggests nodes to add to a workflow.
func (s *APIServer) AISuggest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentWorkflow *model.Workflow `json:"currentWorkflow,omitempty"`
		SelectedNode    string          `json:"selectedNode,omitempty"`
		UserQuery       string          `json:"userQuery"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if s.aiRuntime == nil {
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"suggestions": defaultSuggestions(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.aiRuntime.Config().Timeout)
	defer cancel()

	reply, err := s.aiRuntime.Current().SuggestNodes(ctx, &ai.SuggestNodesRequest{
		CurrentWorkflow: req.CurrentWorkflow,
		SelectedNode:    req.SelectedNode,
		UserQuery:       req.UserQuery,
	})
	if err != nil {
		s.sendError(w, http.StatusBadGateway, "AI suggest failed: "+err.Error(), err)
		return
	}
	s.sendJSON(w, http.StatusOK, reply)
}

// AIExplain explains a workflow in natural language.
func (s *APIServer) AIExplain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Workflow *model.Workflow `json:"workflow"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	if req.Workflow == nil {
		s.sendError(w, http.StatusBadRequest, "Workflow is required", nil)
		return
	}

	if s.aiRuntime == nil {
		nodeCount := len(req.Workflow.Nodes)
		connectionCount := len(req.Workflow.Connections)
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"summary":  fmt.Sprintf("This workflow '%s' contains %d nodes with %d connections.", req.Workflow.Name, nodeCount, connectionCount),
			"dataFlow": "Data flows from trigger nodes through processing nodes to output.",
			"suggestions": []string{
				"Configure an AI provider in Settings → AI for detailed explanations",
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.aiRuntime.Config().Timeout)
	defer cancel()

	reply, err := s.aiRuntime.Current().ExplainWorkflow(ctx, &ai.ExplainWorkflowRequest{
		Workflow: req.Workflow,
	})
	if err != nil {
		s.sendError(w, http.StatusBadGateway, "AI explain failed: "+err.Error(), err)
		return
	}
	s.sendJSON(w, http.StatusOK, reply)
}

// AIFix suggests fixes for workflow errors.
func (s *APIServer) AIFix(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Workflow     *model.Workflow `json:"workflow"`
		ErrorMessage string          `json:"errorMessage"`
		FailedNode   string          `json:"failedNode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if s.aiRuntime == nil {
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"diagnosis": fmt.Sprintf("Error in node '%s': %s", req.FailedNode, req.ErrorMessage),
			"fixes": []map[string]interface{}{
				{
					"description": "Check node parameters and credentials",
					"confidence":  0.8,
					"autoApply":   false,
				},
				{
					"description": "Verify input data format matches expected schema",
					"confidence":  0.7,
					"autoApply":   false,
				},
			},
			"prevention": "Add validation nodes before critical operations",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.aiRuntime.Config().Timeout)
	defer cancel()

	reply, err := s.aiRuntime.Current().FixError(ctx, &ai.FixErrorRequest{
		Workflow:     req.Workflow,
		ErrorMessage: req.ErrorMessage,
		FailedNode:   req.FailedNode,
	})
	if err != nil {
		s.sendError(w, http.StatusBadGateway, "AI fix failed: "+err.Error(), err)
		return
	}
	s.sendJSON(w, http.StatusOK, reply)
}

// AIChat handles conversational workflow building.
func (s *APIServer) AIChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Messages []ai.ChatMessage   `json:"messages"`
		Workflow *model.Workflow    `json:"currentWorkflow,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if s.aiRuntime == nil {
		lastMessage := ""
		if len(req.Messages) > 0 {
			lastMessage = req.Messages[len(req.Messages)-1].Content
		}
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"message": fmt.Sprintf("I understand you want to: '%s'. Configure an AI provider in Settings → AI to enable full chat.", lastMessage),
			"actions": []map[string]interface{}{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.aiRuntime.Config().Timeout)
	defer cancel()

	reply, err := s.aiRuntime.Current().Chat(ctx, &ai.ChatRequest{
		Messages:        req.Messages,
		CurrentWorkflow: req.Workflow,
	})
	if err != nil {
		s.sendError(w, http.StatusBadGateway, "AI chat failed: "+err.Error(), err)
		return
	}
	s.sendJSON(w, http.StatusOK, reply)
}

// AIHealthCheck is a quick liveness probe for the AI configuration.
// Returns 200 with a small JSON payload describing whether AI is
// configured, or 503 if the runtime is missing. We expose this so the
// UI can render a status pill without scraping the full config view.
func (s *APIServer) AIHealthCheck(w http.ResponseWriter, r *http.Request) {
	if s.aiRuntime == nil {
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"enabled": false,
			"reason":  "ai runtime not initialised",
		})
		return
	}
	cfg := s.aiRuntime.Config()
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":  cfg.Enabled,
		"provider": cfg.Provider,
		"model":    cfg.Model,
		"baseUrl":  cfg.BaseURL,
	})
}

// skeletonWorkflow returns a minimal manual-trigger workflow the UI
// can show while AI generation is offline. Kept here (and not in ai
// package) so the api layer can shape the JSON freely.
func skeletonWorkflow() map[string]interface{} {
	return map[string]interface{}{
		"name":   "Generated Workflow",
		"active": false,
		"nodes": []map[string]interface{}{
			{
				"id":       "trigger-1",
				"name":     "Manual Trigger",
				"type":     "n8n-nodes-base.manualTrigger",
				"position": []int{250, 300},
			},
		},
		"connections": map[string]interface{}{},
	}
}

// defaultSuggestions mirrors the static suggestions the old copilot
// handler returned. Kept here so the UI shows the same list during
// AI-disabled bootstrapping.
func defaultSuggestions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type":        "n8n-nodes-base.httpRequest",
			"name":        "HTTP Request",
			"description": "Make HTTP requests to external APIs",
			"reason":      "Commonly used for API integrations",
			"confidence":  0.9,
		},
		{
			"type":        "n8n-nodes-base.set",
			"name":        "Set",
			"description": "Set field values",
			"reason":      "Transform data between nodes",
			"confidence":  0.85,
		},
		{
			"type":        "n8n-nodes-base.if",
			"name":        "IF",
			"description": "Conditional branching",
			"reason":      "Add logic to your workflow",
			"confidence":  0.8,
		},
	}
}