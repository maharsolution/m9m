package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/storage"
)

// CredentialResponse is the safe (data-stripped) credential envelope
// returned to API callers. Mirrors n8n's GET /api/v1/credentials/{id}
// shape so the UI can swap its target without renaming keys.
type CredentialResponse struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	Type                    string    `json:"type"`
	Data                    any       `json:"data,omitempty"`
	IsManaged               bool      `json:"isManaged"`
	IsGlobal                bool      `json:"isGlobal"`
	IsResolvable            bool      `json:"isResolvable"`
	ResolvableAllowFallback bool      `json:"resolvableAllowFallback"`
	ResolverID              string    `json:"resolverId,omitempty"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

func toCredentialResponse(c *storage.Credential) CredentialResponse {
	return CredentialResponse{
		ID:                      c.ID,
		Name:                    c.Name,
		Type:                    c.Type,
		// data is intentionally omitted from API responses — n8n strips it
		// on read for security parity. UI re-fetches via POST/GET cycle.
		IsManaged:               false,
		IsGlobal:                c.IsGlobal,
		IsResolvable:            false,
		ResolvableAllowFallback: false,
		ResolverID:              "",
		CreatedAt:               c.CreatedAt,
		UpdatedAt:               c.UpdatedAt,
	}
}

// ListCredentials returns the safe envelope list.
func (s *APIServer) ListCredentials(w http.ResponseWriter, r *http.Request) {
	credentials, err := s.storage.ListCredentials()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list credentials", err)
		return
	}

	limit := parseIntParam(r.URL.Query().Get("limit"), 100, 250)
	if limit > 0 && len(credentials) > limit {
		credentials = credentials[:limit]
	}

	safe := make([]CredentialResponse, 0, len(credentials))
	for _, c := range credentials {
		safe = append(safe, toCredentialResponse(c))
	}

	// n8n wraps list responses in { data, nextCursor }.
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":       safe,
		"nextCursor": nil,
	})
}

// GetCredential returns the safe envelope for a single credential.
func (s *APIServer) GetCredential(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	credential, err := s.storage.GetCredential(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Credential not found", err)
		return
	}
	s.sendJSON(w, http.StatusOK, toCredentialResponse(credential))
}

// credentialCreateRequest is the inbound POST/PATCH body. Mirrors n8n's
// { name, type, data, isGlobal? } shape.
type credentialCreateRequest struct {
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Data     map[string]interface{} `json:"data"`
	IsGlobal bool                   `json:"isGlobal,omitempty"`
}

// CreateCredential registers a new credential envelope and stores the
// secret `data` payload in the existing credentials.data column. Returns
// the safe envelope (data stripped on read).
func (s *APIServer) CreateCredential(w http.ResponseWriter, r *http.Request) {
	var req credentialCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		s.sendError(w, http.StatusBadRequest, "name is required", nil)
		return
	}
	if strings.TrimSpace(req.Type) == "" {
		s.sendError(w, http.StatusBadRequest, "type is required", nil)
		return
	}

	cred := &storage.Credential{
		Name:      req.Name,
		Type:      req.Type,
		Data:      req.Data,
		IsGlobal:  req.IsGlobal,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.storage.SaveCredential(cred); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save credential", err)
		return
	}
	s.sendJSON(w, http.StatusCreated, toCredentialResponse(cred))
}

// UpdateCredential performs a partial update via PATCH (n8n) and a full
// overwrite via PUT. Both methods share this handler because the
// underlying storage treats them identically — partial updates are
// implemented by reading the existing row and merging any non-zero
// fields from the body.
func (s *APIServer) UpdateCredential(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	existing, err := s.storage.GetCredential(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Credential not found", err)
		return
	}

	var req credentialCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	// n8n rule: if `type` changes, `data` must be supplied.
	typeChanged := req.Type != "" && req.Type != existing.Type
	if typeChanged && len(req.Data) == 0 {
		s.sendError(w, http.StatusBadRequest, "data is required when changing type", nil)
		return
	}

	merged := *existing
	if req.Name != "" {
		merged.Name = req.Name
	}
	if req.Type != "" {
		merged.Type = req.Type
	}
	if req.Data != nil {
		merged.Data = req.Data
	}
	if req.IsGlobal {
		merged.IsGlobal = req.IsGlobal
	}
	merged.UpdatedAt = time.Now().UTC()

	if err := s.storage.UpdateCredential(id, &merged); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update credential", err)
		return
	}
	s.sendJSON(w, http.StatusOK, toCredentialResponse(&merged))
}

// PatchCredential is the explicit PATCH entry point registered in routes.go.
// It shares UpdateCredential's body so the UI can use either verb.
func (s *APIServer) PatchCredential(w http.ResponseWriter, r *http.Request) {
	s.UpdateCredential(w, r)
}

// DeleteCredential removes a credential by id.
func (s *APIServer) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := s.storage.DeleteCredential(id); err != nil {
		s.sendError(w, http.StatusNotFound, "Credential not found", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Schema endpoint
// ---------------------------------------------------------------------------

// SchemaProperty is the JSON Schema "property" object for a credential
// field. Mirrors n8n's per-type form schema so the UI can render the
// matching input without hardcoding per-type field lists.
type SchemaProperty struct {
	Name        string                 `json:"name"`
	DisplayName string                 `json:"displayName"`
	Type        string                 `json:"type"` // string | password | number | boolean | options | json
	Required    bool                   `json:"required"`
	Default     interface{}            `json:"default,omitempty"`
	Placeholder string                 `json:"placeholder,omitempty"`
	Description string                 `json:"description,omitempty"`
	Options     []SchemaPropertyOption `json:"options,omitempty"`
}

type SchemaPropertyOption struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

// CredentialTypeSchema is the full per-type schema returned by
// GET /api/v1/credentials/schema/{type}.
type CredentialTypeSchema struct {
	Type                 string           `json:"type"`
	DisplayName          string           `json:"displayName"`
	Properties           []SchemaProperty `json:"properties"`
	Required             []string         `json:"required"`
	DisplayOptions       map[string]bool  `json:"displayOptions,omitempty"`
}

// credentialSchemas holds the static per-type schemas for the credential
// types we ship with the UI. The 10 types listed in the UI's hardcoded
// credentialTypes array (httpBasicAuth, httpHeaderAuth, oAuth2Api, apiKey,
// postgres, mysql, slack, discord, openai, anthropic) are the only
// authoritative source — anything else returns 404.
var credentialSchemas = map[string]CredentialTypeSchema{
	"httpBasicAuth": {
		Type:        "httpBasicAuth",
		DisplayName: "Basic Auth",
		Properties: []SchemaProperty{
			{Name: "user", DisplayName: "User", Type: "string", Required: true, Placeholder: "username"},
			{Name: "password", DisplayName: "Password", Type: "password", Required: true},
			{Name: "allowedDomains", DisplayName: "Allowed HTTP Request Domains", Type: "string", Description: "Comma-separated domains to allow without CORS"},
		},
		Required: []string{"user", "password"},
	},
	"httpHeaderAuth": {
		Type:        "httpHeaderAuth",
		DisplayName: "Header Auth",
		Properties: []SchemaProperty{
			{Name: "name", DisplayName: "Header Name", Type: "string", Required: true, Placeholder: "Authorization", Default: "Authorization"},
			{Name: "value", DisplayName: "Header Value", Type: "string", Required: true},
		},
		Required: []string{"name", "value"},
	},
	"oAuth2Api": {
		Type:        "oAuth2Api",
		DisplayName: "OAuth2 API",
		Properties: []SchemaProperty{
			{Name: "grantType", DisplayName: "Grant Type", Type: "options", Required: true, Default: "authorizationCode",
				Options: []SchemaPropertyOption{
					{Name: "Authorization Code", Value: "authorizationCode"},
					{Name: "Client Credentials", Value: "clientCredentials"},
					{Name: "PKCE", Value: "pkce"},
				}},
			{Name: "authUrl", DisplayName: "Authorization URL", Type: "string", Required: true, Placeholder: "https://example.com/oauth/authorize"},
			{Name: "accessTokenUrl", DisplayName: "Access Token URL", Type: "string", Required: true, Placeholder: "https://example.com/oauth/token"},
			{Name: "clientId", DisplayName: "Client ID", Type: "string", Required: true},
			{Name: "clientSecret", DisplayName: "Client Secret", Type: "password", Required: true},
			{Name: "scope", DisplayName: "Scope", Type: "string", Placeholder: "read write"},
			{Name: "authQueryParameters", DisplayName: "Auth URI Query Parameters", Type: "string"},
			{Name: "authentication", DisplayName: "Authentication", Type: "options", Default: "header",
				Options: []SchemaPropertyOption{
					{Name: "Header", Value: "header"},
					{Name: "Body", Value: "body"},
				}},
		},
		Required: []string{"grantType", "authUrl", "accessTokenUrl", "clientId", "clientSecret"},
		DisplayOptions: map[string]bool{
			"showAuthUrl": true,
		},
	},
	"apiKey": {
		Type:        "apiKey",
		DisplayName: "API Key",
		Properties: []SchemaProperty{
			{Name: "apiKey", DisplayName: "API Key", Type: "password", Required: true},
		},
		Required: []string{"apiKey"},
	},
	"postgres": {
		Type:        "postgres",
		DisplayName: "Postgres",
		Properties: []SchemaProperty{
			{Name: "host", DisplayName: "Host", Type: "string", Required: true, Default: "localhost"},
			{Name: "port", DisplayName: "Port", Type: "number", Required: true, Default: 5432},
			{Name: "database", DisplayName: "Database", Type: "string", Required: true},
			{Name: "user", DisplayName: "User", Type: "string", Required: true},
			{Name: "password", DisplayName: "Password", Type: "password", Required: true},
			{Name: "ssl", DisplayName: "SSL", Type: "options", Default: "disable",
				Options: []SchemaPropertyOption{
					{Name: "Allow", Value: "allow"},
					{Name: "Disable", Value: "disable"},
					{Name: "Require", Value: "require"},
					{Name: "Verify-Full", Value: "verify-full"},
				}},
			{Name: "sshTunnel", DisplayName: "SSH Tunnel", Type: "boolean", Default: false},
			{Name: "sshHost", DisplayName: "SSH Host", Type: "string"},
			{Name: "sshPort", DisplayName: "SSH Port", Type: "number", Default: 22},
			{Name: "sshUser", DisplayName: "SSH User", Type: "string"},
			{Name: "sshPassword", DisplayName: "SSH Password", Type: "password"},
			{Name: "maxConnections", DisplayName: "Max Connections", Type: "number", Default: 10},
			{Name: "connectionTimeout", DisplayName: "Connection Timeout", Type: "number", Default: 30},
		},
		Required: []string{"host", "database", "user", "password"},
	},
	"mysql": {
		Type:        "mysql",
		DisplayName: "MySQL",
		Properties: []SchemaProperty{
			{Name: "host", DisplayName: "Host", Type: "string", Required: true, Default: "localhost"},
			{Name: "port", DisplayName: "Port", Type: "number", Required: true, Default: 3306},
			{Name: "database", DisplayName: "Database", Type: "string", Required: true},
			{Name: "user", DisplayName: "User", Type: "string", Required: true},
			{Name: "password", DisplayName: "Password", Type: "password", Required: true},
			{Name: "ssl", DisplayName: "SSL", Type: "boolean", Default: false},
			{Name: "maxConnections", DisplayName: "Max Connections", Type: "number", Default: 10},
		},
		Required: []string{"host", "database", "user", "password"},
	},
	"slack": {
		Type:        "slack",
		DisplayName: "Slack",
		Properties: []SchemaProperty{
			{Name: "accessToken", DisplayName: "Access Token", Type: "password", Required: true, Description: "Slack OAuth token (xoxb-... or xoxp-...)"},
		},
		Required: []string{"accessToken"},
	},
	"discord": {
		Type:        "discord",
		DisplayName: "Discord",
		Properties: []SchemaProperty{
			{Name: "webhookUrl", DisplayName: "Webhook URL", Type: "string", Required: true, Placeholder: "https://discord.com/api/webhooks/..."},
			{Name: "botToken", DisplayName: "Bot Token", Type: "password"},
		},
		Required: []string{"webhookUrl"},
	},
	"openai": {
		Type:        "openai",
		DisplayName: "OpenAI",
		Properties: []SchemaProperty{
			{Name: "apiKey", DisplayName: "API Key", Type: "password", Required: true, Description: "OpenAI API key (sk-...)"},
			{Name: "organization", DisplayName: "Organization", Type: "string"},
		},
		Required: []string{"apiKey"},
	},
	"anthropic": {
		Type:        "anthropic",
		DisplayName: "Anthropic",
		Properties: []SchemaProperty{
			{Name: "apiKey", DisplayName: "API Key", Type: "password", Required: true, Description: "Anthropic API key (sk-ant-...)"},
		},
		Required: []string{"apiKey"},
	},
	"jwtAuth": {
		Type:        "jwtAuth",
		DisplayName: "JWT Auth",
		Properties: []SchemaProperty{
			{Name: "secret", DisplayName: "Secret", Type: "password", Required: true},
			{Name: "algorithm", DisplayName: "Algorithm", Type: "options", Default: "HS256",
				Options: []SchemaPropertyOption{
					{Name: "HS256", Value: "HS256"},
					{Name: "HS384", Value: "HS384"},
					{Name: "HS512", Value: "HS512"},
				}},
			{Name: "headerPrefix", DisplayName: "Header Prefix", Type: "string", Default: "Bearer"},
			{Name: "payload", DisplayName: "Payload (JSON)", Type: "json"},
		},
		Required: []string{"secret"},
	},
}

// GetCredentialSchema returns the JSON Schema for one credential type.
func (s *APIServer) GetCredentialSchema(w http.ResponseWriter, r *http.Request) {
	credType, err := url.PathUnescape(mux.Vars(r)["type"])
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid type", err)
		return
	}
	schema, ok := credentialSchemas[credType]
	if !ok {
		s.sendError(w, http.StatusNotFound, "Unknown credential type: "+credType, nil)
		return
	}
	s.sendJSON(w, http.StatusOK, schema)
}

// ---------------------------------------------------------------------------
// Test endpoint
// ---------------------------------------------------------------------------

// testCredentialResult is the n8n-shaped test response.
type testCredentialResult struct {
	Status  string `json:"status"`  // "OK" or "Error"
	Message string `json:"message"` // human-readable
}

// TestCredential runs type-specific validation against the stored data.
// We deliberately do NOT make outbound network calls (e.g. open a DB
// connection) — n8n's "Test" is also a static validator for most types
// and an offline reachability check for HTTP-based ones. We keep parity
// with the static-validator behaviour and return OK / Error per field.
func (s *APIServer) TestCredential(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	cred, err := s.storage.GetCredential(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Credential not found", err)
		return
	}

	schema, ok := credentialSchemas[cred.Type]
	if !ok {
		s.sendJSON(w, http.StatusOK, testCredentialResult{
			Status:  "Error",
			Message: "Unknown credential type: " + cred.Type,
		})
		return
	}

	var missing []string
	for _, req := range schema.Required {
		v, ok := cred.Data[req]
		if !ok {
			missing = append(missing, req)
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			missing = append(missing, req)
		}
	}

	// Type-specific format checks.
	if cred.Type == "oAuth2Api" {
		if u, ok := cred.Data["authUrl"].(string); ok && !isValidURL(u) {
			s.sendJSON(w, http.StatusOK, testCredentialResult{
				Status:  "Error",
				Message: "Authorization URL is not a valid URL",
			})
			return
		}
		if u, ok := cred.Data["accessTokenUrl"].(string); ok && !isValidURL(u) {
			s.sendJSON(w, http.StatusOK, testCredentialResult{
				Status:  "Error",
				Message: "Access Token URL is not a valid URL",
			})
			return
		}
	}
	if cred.Type == "postgres" || cred.Type == "mysql" {
		if p, ok := cred.Data["port"]; ok {
			switch v := p.(type) {
			case float64:
				if v < 1 || v > 65535 {
					s.sendJSON(w, http.StatusOK, testCredentialResult{Status: "Error", Message: "Port out of range"})
					return
				}
			case string:
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 || n > 65535 {
					s.sendJSON(w, http.StatusOK, testCredentialResult{Status: "Error", Message: "Port out of range"})
					return
				}
			}
		}
	}

	if len(missing) > 0 {
		s.sendJSON(w, http.StatusOK, testCredentialResult{
			Status:  "Error",
			Message: "Missing or empty required fields: " + strings.Join(missing, ", "),
		})
		return
	}
	s.sendJSON(w, http.StatusOK, testCredentialResult{Status: "OK", Message: "Credential is valid"})
}

func isValidURL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// ---------------------------------------------------------------------------
// Transfer endpoint
// ---------------------------------------------------------------------------

// transferCredentialRequest is the inbound PUT body.
type transferCredentialRequest struct {
	DestinationProjectID string `json:"destinationProjectId"`
}

// TransferCredential moves a credential to a project. m9m has no project
// model yet, so this is a soft-op that accepts the request and returns
// the envelope — same shape as n8n's response. When the projects model
// lands, this is where the move happens.
func (s *APIServer) TransferCredential(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var req transferCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}
	if strings.TrimSpace(req.DestinationProjectID) == "" {
		s.sendError(w, http.StatusBadRequest, "destinationProjectId is required", nil)
		return
	}

	cred, err := s.storage.GetCredential(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Credential not found", err)
		return
	}

	s.sendJSON(w, http.StatusOK, toCredentialResponse(cred))
}
