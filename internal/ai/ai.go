// Package ai provides the in-app AI assistant that powers workflow
// generation, node suggestions, error fixes, and conversational
// workflow building. It runs against any OpenAI-compatible, Anthropic,
// Anthropic-compatible (MiniMax), or Ollama endpoint, configured either
// through environment variables or live from the Settings → AI page
// (env-default with DB-override, same pattern as Settings → Telemetry).
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/neul-labs/m9m/internal/model"
)

// Provider represents an AI provider.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderMiniMax   Provider = "minimax" // Anthropic-compatible wire shape
	ProviderOllama    Provider = "ollama"
)

// Config configures the AI service.
type Config struct {
	Enabled     bool          // master switch — when false, AI calls are short-circuited
	Provider    Provider
	APIKey      string
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature float64
	Timeout     time.Duration
}

// DefaultConfig returns the default AI configuration.
func DefaultConfig() *Config {
	return &Config{
		Provider:    ProviderOpenAI,
		Model:       "gpt-4",
		MaxTokens:   4096,
		Temperature: 0.7,
		Timeout:     60 * time.Second,
	}
}

// AI provides the in-app AI assistant: workflow generation, node
// suggestions, explanation, error fixes, and chat. Construct one with
// NewAI; the Runtime type (see runtime.go) handles live config swaps.
type AI struct {
	config     *Config
	httpClient *http.Client
	nodeTypes  []NodeTypeInfo
}

// NodeTypeInfo describes an available node type.
type NodeTypeInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// NewAI creates a new AI assistant. config may be nil, in which case
// DefaultConfig is used.
func NewAI(config *Config) *AI {
	if config == nil {
		config = DefaultConfig()
	}

	return &AI{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		nodeTypes: getAvailableNodeTypes(),
	}
}

// Config returns the AI service's current configuration. Callers should
// not mutate the returned pointer; use Runtime.Apply for live changes.
func (a *AI) Config() *Config { return a.config }

// GenerateWorkflowRequest represents a request to generate a workflow.
type GenerateWorkflowRequest struct {
	Description string                 `json:"description"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// GenerateWorkflowResponse represents the response from workflow generation.
type GenerateWorkflowResponse struct {
	Workflow    *model.Workflow `json:"workflow"`
	Explanation string          `json:"explanation"`
	Suggestions []string        `json:"suggestions,omitempty"`
}

// SuggestNodesRequest represents a request for node suggestions.
type SuggestNodesRequest struct {
	CurrentWorkflow *model.Workflow `json:"currentWorkflow,omitempty"`
	SelectedNode    string          `json:"selectedNode,omitempty"`
	UserQuery       string          `json:"userQuery"`
}

// SuggestNodesResponse represents node suggestions.
type SuggestNodesResponse struct {
	Suggestions []NodeSuggestion `json:"suggestions"`
}

// NodeSuggestion represents a suggested node.
type NodeSuggestion struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Reason      string                 `json:"reason"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Confidence  float64                `json:"confidence"`
}

// ExplainWorkflowRequest represents a request to explain a workflow.
type ExplainWorkflowRequest struct {
	Workflow *model.Workflow `json:"workflow"`
}

// ExplainWorkflowResponse represents workflow explanation.
type ExplainWorkflowResponse struct {
	Summary     string            `json:"summary"`
	NodeDetails []NodeExplanation `json:"nodeDetails"`
	DataFlow    string            `json:"dataFlow"`
	Suggestions []string          `json:"suggestions,omitempty"`
}

// NodeExplanation explains a single node.
type NodeExplanation struct {
	NodeName   string `json:"nodeName"`
	Purpose    string `json:"purpose"`
	InputData  string `json:"inputData"`
	OutputData string `json:"outputData"`
}

// FixErrorRequest represents a request to fix an error.
type FixErrorRequest struct {
	Workflow      *model.Workflow `json:"workflow"`
	ErrorMessage  string          `json:"errorMessage"`
	FailedNode    string          `json:"failedNode"`
	ExecutionData interface{}     `json:"executionData,omitempty"`
}

// FixErrorResponse represents error fix suggestions.
type FixErrorResponse struct {
	Diagnosis  string     `json:"diagnosis"`
	Fixes      []ErrorFix `json:"fixes"`
	Prevention string     `json:"prevention"`
}

// ErrorFix represents a suggested fix.
type ErrorFix struct {
	Description string                 `json:"description"`
	NodeChanges map[string]interface{} `json:"nodeChanges,omitempty"`
	Confidence  float64                `json:"confidence"`
	AutoApply   bool                   `json:"autoApply"`
}

// ChatMessage represents a chat message.
type ChatMessage struct {
	Role    string `json:"role"` // user, assistant, system
	Content string `json:"content"`
}

// ChatRequest represents a chat request.
type ChatRequest struct {
	Messages        []ChatMessage   `json:"messages"`
	CurrentWorkflow *model.Workflow `json:"currentWorkflow,omitempty"`
}

// ChatResponse represents a chat response.
type ChatResponse struct {
	Message         string          `json:"message"`
	WorkflowChanges *model.Workflow `json:"workflowChanges,omitempty"`
	Actions         []ChatAction    `json:"actions,omitempty"`
}

// ChatAction represents a suggested action from chat.
type ChatAction struct {
	Type        string                 `json:"type"` // add_node, modify_node, delete_node, connect_nodes
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
}

// GenerateWorkflow generates a workflow from a description.
func (a *AI) GenerateWorkflow(ctx context.Context, req *GenerateWorkflowRequest) (*GenerateWorkflowResponse, error) {
	prompt := a.buildGenerateWorkflowPrompt(req)
	response, err := a.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}
	return a.parseGenerateWorkflowResponse(response)
}

// SuggestNodes suggests nodes based on context.
func (a *AI) SuggestNodes(ctx context.Context, req *SuggestNodesRequest) (*SuggestNodesResponse, error) {
	prompt := a.buildSuggestNodesPrompt(req)
	response, err := a.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}
	return a.parseSuggestNodesResponse(response)
}

// ExplainWorkflow explains a workflow in natural language.
func (a *AI) ExplainWorkflow(ctx context.Context, req *ExplainWorkflowRequest) (*ExplainWorkflowResponse, error) {
	prompt := a.buildExplainWorkflowPrompt(req)
	response, err := a.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}
	return a.parseExplainWorkflowResponse(response)
}

// FixError suggests fixes for workflow errors.
func (a *AI) FixError(ctx context.Context, req *FixErrorRequest) (*FixErrorResponse, error) {
	prompt := a.buildFixErrorPrompt(req)
	response, err := a.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}
	return a.parseFixErrorResponse(response)
}

// Chat handles conversational workflow building.
func (a *AI) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	prompt := a.buildChatPrompt(req)
	response, err := a.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}
	return a.parseChatResponse(response)
}

// Prompt builders

func (a *AI) buildGenerateWorkflowPrompt(req *GenerateWorkflowRequest) string {
	nodeTypesJSON, _ := json.Marshal(a.nodeTypes)

	return fmt.Sprintf(`You are an expert workflow automation assistant for m9m, an agent-native workflow platform.

Your task is to generate a workflow based on the user's description.

Available node types:
%s

User's request: %s

Generate a complete, working m9m workflow in JSON format. The workflow should:
1. Use appropriate nodes from the available types
2. Have proper connections between nodes
3. Include sensible default parameters
4. Be ready to execute

Respond with a JSON object containing:
{
  "workflow": { ... the workflow object ... },
  "explanation": "Brief explanation of what the workflow does",
  "suggestions": ["Optional improvements or alternatives"]
}

IMPORTANT: Generate valid JSON only. The workflow must follow this structure:
{
  "name": "Workflow Name",
  "active": false,
  "nodes": [
    {
      "id": "unique-id",
      "name": "Node Name",
      "type": "node-type",
      "position": [x, y],
      "parameters": {}
    }
  ],
  "connections": {
    "Source Node Name": {
      "main": [[{"node": "Target Node Name", "type": "main", "index": 0}]]
    }
  }
}`, string(nodeTypesJSON), req.Description)
}

func (a *AI) buildSuggestNodesPrompt(req *SuggestNodesRequest) string {
	nodeTypesJSON, _ := json.Marshal(a.nodeTypes)
	workflowJSON := ""
	if req.CurrentWorkflow != nil {
		wfBytes, _ := json.Marshal(req.CurrentWorkflow)
		workflowJSON = string(wfBytes)
	}

	return fmt.Sprintf(`You are an expert workflow automation assistant.

Available node types:
%s

Current workflow (if any):
%s

Selected node: %s

User query: %s

Suggest the most appropriate nodes to add. Respond with JSON:
{
  "suggestions": [
    {
      "type": "node-type",
      "name": "Suggested name",
      "description": "What this node does",
      "reason": "Why this is suggested",
      "parameters": {},
      "confidence": 0.95
    }
  ]
}`, string(nodeTypesJSON), workflowJSON, req.SelectedNode, req.UserQuery)
}

func (a *AI) buildExplainWorkflowPrompt(req *ExplainWorkflowRequest) string {
	workflowJSON, _ := json.Marshal(req.Workflow)

	return fmt.Sprintf(`Explain this workflow in simple terms:

%s

Respond with JSON:
{
  "summary": "One paragraph summary",
  "nodeDetails": [
    {
      "nodeName": "Name",
      "purpose": "What it does",
      "inputData": "What data it receives",
      "outputData": "What data it produces"
    }
  ],
  "dataFlow": "How data flows through the workflow",
  "suggestions": ["Optional improvements"]
}`, string(workflowJSON))
}

func (a *AI) buildFixErrorPrompt(req *FixErrorRequest) string {
	workflowJSON, _ := json.Marshal(req.Workflow)

	return fmt.Sprintf(`A workflow execution failed. Help diagnose and fix the issue.

Workflow:
%s

Failed node: %s
Error message: %s

Respond with JSON:
{
  "diagnosis": "What went wrong",
  "fixes": [
    {
      "description": "How to fix it",
      "nodeChanges": {},
      "confidence": 0.9,
      "autoApply": true
    }
  ],
  "prevention": "How to prevent this in the future"
}`, string(workflowJSON), req.FailedNode, req.ErrorMessage)
}

func (a *AI) buildChatPrompt(req *ChatRequest) string {
	nodeTypesJSON, _ := json.Marshal(a.nodeTypes)
	workflowJSON := ""
	if req.CurrentWorkflow != nil {
		wfBytes, _ := json.Marshal(req.CurrentWorkflow)
		workflowJSON = string(wfBytes)
	}

	messagesText := ""
	for _, msg := range req.Messages {
		messagesText += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
	}

	return fmt.Sprintf(`You are a helpful workflow automation assistant for m9m.

Available node types:
%s

Current workflow:
%s

Conversation:
%s

Respond helpfully. If suggesting workflow changes, include them in JSON format:
{
  "message": "Your response",
  "workflowChanges": null or { workflow object },
  "actions": [
    {
      "type": "add_node|modify_node|delete_node|connect_nodes",
      "description": "What this action does",
      "data": {}
    }
  ]
}`, string(nodeTypesJSON), workflowJSON, messagesText)
}

// LLM integration

func (a *AI) callLLM(ctx context.Context, prompt string) (string, error) {
	switch a.config.Provider {
	case ProviderOpenAI:
		return a.callOpenAI(ctx, prompt)
	case ProviderAnthropic, ProviderMiniMax:
		// MiniMax exposes an Anthropic-compatible /v1/messages surface,
		// so the same call path works for both.
		return a.callAnthropic(ctx, prompt)
	case ProviderOllama:
		return a.callOllama(ctx, prompt)
	default:
		return "", fmt.Errorf("unsupported provider: %s", a.config.Provider)
	}
}

func (a *AI) callOpenAI(ctx context.Context, prompt string) (string, error) {
	baseURL := a.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	requestBody := map[string]interface{}{
		"model": a.config.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  a.config.MaxTokens,
		"temperature": a.config.Temperature,
	}

	body, _ := json.Marshal(requestBody)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error: %s", string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return result.Choices[0].Message.Content, nil
}

func (a *AI) callAnthropic(ctx context.Context, prompt string) (string, error) {
	baseURL := a.config.BaseURL
	if baseURL == "" {
		// Honour MiniMax first when its env var is set, so operators
		// can route the AI assistant through the same gateway as
		// their Anthropic-compatible workflow calls.
		if u := strings.TrimSpace(os.Getenv("M9M_MINIMAX_BASE_URL")); u != "" {
			baseURL = strings.TrimRight(u, "/")
		} else if u := strings.TrimSpace(os.Getenv("M9M_ANTHROPIC_BASE_URL")); u != "" {
			baseURL = strings.TrimRight(u, "/")
		} else {
			baseURL = "https://api.anthropic.com/v1"
		}
	}

	requestBody := map[string]interface{}{
		"model":      a.config.Model,
		"max_tokens": a.config.MaxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, _ := json.Marshal(requestBody)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Anthropic API error: %s", string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from Anthropic")
	}

	return result.Content[0].Text, nil
}

func (a *AI) callOllama(ctx context.Context, prompt string) (string, error) {
	baseURL := a.config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	requestBody := map[string]interface{}{
		"model":  a.config.Model,
		"prompt": prompt,
		"stream": false,
	}

	body, _ := json.Marshal(requestBody)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Response string `json:"response"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	return result.Response, nil
}

// Response parsers

func (a *AI) parseGenerateWorkflowResponse(response string) (*GenerateWorkflowResponse, error) {
	jsonStr := extractJSON(response)
	var result GenerateWorkflowResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return &GenerateWorkflowResponse{
			Explanation: response,
			Suggestions: []string{"Could not parse workflow. Please try a more specific description."},
		}, nil
	}
	return &result, nil
}

func (a *AI) parseSuggestNodesResponse(response string) (*SuggestNodesResponse, error) {
	jsonStr := extractJSON(response)
	var result SuggestNodesResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return &SuggestNodesResponse{Suggestions: []NodeSuggestion{}}, nil
	}
	return &result, nil
}

func (a *AI) parseExplainWorkflowResponse(response string) (*ExplainWorkflowResponse, error) {
	jsonStr := extractJSON(response)
	var result ExplainWorkflowResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return &ExplainWorkflowResponse{Summary: response}, nil
	}
	return &result, nil
}

func (a *AI) parseFixErrorResponse(response string) (*FixErrorResponse, error) {
	jsonStr := extractJSON(response)
	var result FixErrorResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return &FixErrorResponse{Diagnosis: response}, nil
	}
	return &result, nil
}

func (a *AI) parseChatResponse(response string) (*ChatResponse, error) {
	jsonStr := extractJSON(response)
	var result ChatResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return &ChatResponse{Message: response}, nil
	}
	return &result, nil
}

// Helper functions

func extractJSON(text string) string {
	start := strings.Index(text, "{")
	if start == -1 {
		return text
	}
	depth := 0
	for i := start; i < len(text); i++ {
		if text[i] == '{' {
			depth++
		} else if text[i] == '}' {
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return text[start:]
}

func getAvailableNodeTypes() []NodeTypeInfo {
	return []NodeTypeInfo{
		// Triggers
		{Type: "n8n-nodes-base.webhook", Name: "Webhook", Description: "Receive HTTP requests", Category: "trigger"},
		{Type: "n8n-nodes-base.scheduleTrigger", Name: "Schedule Trigger", Description: "Run on a schedule", Category: "trigger"},
		{Type: "n8n-nodes-base.manualTrigger", Name: "Manual Trigger", Description: "Start manually", Category: "trigger"},

		// HTTP
		{Type: "n8n-nodes-base.httpRequest", Name: "HTTP Request", Description: "Make HTTP requests", Category: "http"},

		// Data Transform
		{Type: "n8n-nodes-base.set", Name: "Set", Description: "Set field values", Category: "transform"},
		{Type: "n8n-nodes-base.function", Name: "Function", Description: "Run JavaScript code", Category: "transform"},
		{Type: "n8n-nodes-base.code", Name: "Code", Description: "Execute code", Category: "transform"},
		{Type: "n8n-nodes-base.filter", Name: "Filter", Description: "Filter items", Category: "transform"},
		{Type: "n8n-nodes-base.merge", Name: "Merge", Description: "Merge data streams", Category: "transform"},
		{Type: "n8n-nodes-base.split", Name: "Split In Batches", Description: "Split into batches", Category: "transform"},
		{Type: "n8n-nodes-base.switch", Name: "Switch", Description: "Route based on conditions", Category: "transform"},
		{Type: "n8n-nodes-base.itemLists", Name: "Item Lists", Description: "Manipulate item lists", Category: "transform"},

		// Databases
		{Type: "n8n-nodes-base.postgres", Name: "PostgreSQL", Description: "Query PostgreSQL", Category: "database"},
		{Type: "n8n-nodes-base.mysql", Name: "MySQL", Description: "Query MySQL", Category: "database"},
		{Type: "n8n-nodes-base.sqlite", Name: "SQLite", Description: "Query SQLite", Category: "database"},

		// AI
		{Type: "n8n-nodes-base.openAi", Name: "OpenAI", Description: "OpenAI API", Category: "ai"},
		{Type: "n8n-nodes-base.anthropic", Name: "Anthropic", Description: "Claude API", Category: "ai"},

		// Email
		{Type: "n8n-nodes-base.emailSend", Name: "Send Email", Description: "Send emails via SMTP", Category: "email"},

		// Messaging
		{Type: "n8n-nodes-base.slack", Name: "Slack", Description: "Slack integration", Category: "messaging"},
		{Type: "n8n-nodes-base.discord", Name: "Discord", Description: "Discord integration", Category: "messaging"},

		// File
		{Type: "n8n-nodes-base.readBinaryFile", Name: "Read Binary File", Description: "Read files", Category: "file"},
		{Type: "n8n-nodes-base.writeBinaryFile", Name: "Write Binary File", Description: "Write files", Category: "file"},

		// Flow Control
		{Type: "n8n-nodes-base.if", Name: "IF", Description: "Conditional branching", Category: "flow"},
		{Type: "n8n-nodes-base.noOp", Name: "No Operation", Description: "Pass through", Category: "flow"},
	}
}
