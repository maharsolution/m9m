package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
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
		ID:   c.ID,
		Name: c.Name,
		Type: c.Type,
		// data is intentionally omitted from list responses — n8n strips
		// it there for security parity. The single-credential GET
		// (toCredentialResponseWithData) does return it so the editor
		// can pre-fill the edit form.
		IsManaged:               false,
		IsGlobal:                c.IsGlobal,
		IsResolvable:            false,
		ResolvableAllowFallback: false,
		ResolverID:              "",
		CreatedAt:               c.CreatedAt,
		UpdatedAt:               c.UpdatedAt,
	}
}

// toCredentialResponseWithData is the single-credential envelope. It
// attaches the stored `data` payload so the UI's edit modal can show
// the real values (host, database, connection string, …) instead of
// blank inputs — the user asked for exactly that: the data fields must
// be honoured so the credential can be tested against its real data.
//
// n8n's editor REST API behaves the same way: GET /credentials/:id
// returns the decrypted `data` so the edit form is pre-filled.
func toCredentialResponseWithData(c *storage.Credential) CredentialResponse {
	resp := toCredentialResponse(c)
	if c.Data != nil {
		resp.Data = c.Data
	}
	return resp
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

// GetCredential returns the envelope for a single credential, WITH its
// `data` payload attached so the UI can pre-fill the edit form.
func (s *APIServer) GetCredential(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	credential, err := s.storage.GetCredential(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Credential not found", err)
		return
	}
	s.sendJSON(w, http.StatusOK, toCredentialResponseWithData(credential))
}

// credentialCreateRequest is the inbound POST/PATCH body. Mirrors n8n's
// { name, type, data, isGlobal?, id? } shape.
//
// `id` is OPTIONAL. n8n's own REST API accepts a client-supplied id on
// create (a 7-char alphanumeric like `7yYBGnRIN6EZRLD5`); honouring
// it here lets the sync bridge round-trip without generating new ids
// on every cycle. When omitted, m9m falls back to its own server-side
// id scheme (cred_<ts>_<seq>) so existing callers aren't broken.
type credentialCreateRequest struct {
	ID       string                 `json:"id,omitempty"`
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
		ID:        req.ID,
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

	// Mirror to the in-memory credential store the engine reads
	// at execution time. Without this, a credential created via
	// the UI after server startup is invisible to running
	// workflows until the server is restarted (or LoadFromStorage
	// is re-run, which we never do).
	if s.credMgr != nil {
		s.credMgr.UpsertCredential(cred)
	}

	s.sendJSON(w, http.StatusCreated, toCredentialResponseWithData(cred))
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
		merged.Data = mergeCredentialData(existing.Data, req.Data)
	}
	if req.IsGlobal {
		merged.IsGlobal = req.IsGlobal
	}
	merged.UpdatedAt = time.Now().UTC()

	if err := s.storage.UpdateCredential(id, &merged); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update credential", err)
		return
	}

	// Mirror to the in-memory store so the next engine
	// ResolveWorkflowCredentials picks up the updated data.
	// Same rationale as CreateCredential above.
	if s.credMgr != nil {
		s.credMgr.UpsertCredential(&merged)
	}

	s.sendJSON(w, http.StatusOK, toCredentialResponseWithData(&merged))
}

// mergeCredentialData overlays the fields present in `patch` onto a
// copy of `base`. A PATCH that only carries the fields the user
// actually touched must not wipe the secrets they left alone — the
// previous wholesale replace erased the credential's data whenever the
// UI submitted an empty (or partial) map.
func mergeCredentialData(base, patch map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(patch))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
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

	// Drop from the in-memory store too, otherwise the engine
	// would still resolve this id at execution time and the user
	// would wonder why a "deleted" credential keeps working.
	if s.credMgr != nil {
		s.credMgr.RemoveCredential(id)
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
	Type           string           `json:"type"`
	DisplayName    string           `json:"displayName"`
	Properties     []SchemaProperty `json:"properties"`
	Required       []string         `json:"required"`
	DisplayOptions map[string]bool  `json:"displayOptions,omitempty"`
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
	"mongoDb": {
		Type:        "mongoDb",
		DisplayName: "MongoDB",
		Properties: []SchemaProperty{
			{Name: "connectionString", DisplayName: "Connection String", Type: "password", Required: false, Description: "Full mongodb:// URI (overrides host/port)."},
			{Name: "host", DisplayName: "Host", Type: "string", Required: false, Default: "localhost"},
			{Name: "port", DisplayName: "Port", Type: "number", Required: false, Default: 27017},
			{Name: "database", DisplayName: "Default Database", Type: "string", Required: false, Description: "Default auth source / db (also used as authSource for user/pass login)."},
			{Name: "user", DisplayName: "User", Type: "string", Required: false},
			{Name: "password", DisplayName: "Password", Type: "password", Required: false},
			{Name: "tls", DisplayName: "TLS", Type: "boolean", Default: false},
			{Name: "tlsAllowInvalidCertificates", DisplayName: "Allow Invalid TLS Certs", Type: "boolean", Default: false},
			{Name: "authSource", DisplayName: "Auth Source", Type: "string", Default: "admin", Description: "Database where the user is defined (e.g. 'admin' for Atlas)."},
		},
		Required: []string{},
	},
	"kafka": {
		Type:        "kafka",
		DisplayName: "Kafka",
		Properties: []SchemaProperty{
			// Native-mode fields. Either `brokers` (comma-separated) is
			// required, or `restProxyUrl` is required (REST-proxy mode).
			// Both can be supplied; the node picks based on `mode`.
			{Name: "brokers", DisplayName: "Bootstrap Servers", Type: "string", Required: false, Description: "Comma-separated broker list (e.g. broker1:9092,broker2:9092)."},
			{Name: "clientId", DisplayName: "Client ID", Type: "string", Required: false, Default: "m9m-kafka"},
			{Name: "saslMechanism", DisplayName: "SASL Mechanism", Type: "options", Options: []SchemaPropertyOption{
				{Name: "None", Value: ""},
				{Name: "PLAIN", Value: "PLAIN"},
				{Name: "SCRAM-SHA-256", Value: "SCRAM-SHA-256"},
				{Name: "SCRAM-SHA-512", Value: "SCRAM-SHA-512"},
			}, Required: false, Default: "", Description: "SASL mechanism for the native broker connection. Leave empty for no SASL; when a username is set the connection test falls back to PLAIN, then SCRAM-SHA-256, then SCRAM-SHA-512."},
			// Exactly ONE username and ONE password. The canonical
			// names are `username` / `password` — n8n parity, see
			// n8n/packages/nodes-base/credentials/Kafka.credentials.ts
			// (the n8n Kafka credential stores the login as
			// `username` / `password`). The legacy m9m spellings
			// `saslUsername` / `saslPassword` stay valid as aliases
			// (see kafkaCredentialUserPass in credential_conn.go), so
			// already-stored credentials and synced n8n credentials
			// both keep working without showing a second input.
			{Name: "username", DisplayName: "Username", Type: "string", Required: false, Description: "Username for SASL (native brokers) and for Basic auth against the REST Proxy."},
			{Name: "password", DisplayName: "Password", Type: "password", Required: false, Description: "Password for SASL (native brokers) and for Basic auth against the REST Proxy."},
			{Name: "tls", DisplayName: "TLS", Type: "boolean", Default: false, Description: "Enable TLS for the native broker connection."},
			// REST-proxy mode fields. Picked when the node's `mode` is
			// `rest` (the default).
			{Name: "restProxyUrl", DisplayName: "REST Proxy URL", Type: "string", Required: false, Description: "Confluent-compatible REST Proxy base URL (e.g. http://kafka-rest:8082)."},
			{Name: "authentication", DisplayName: "Authentication", Type: "options", Options: []SchemaPropertyOption{
				{Name: "None", Value: "none"},
				{Name: "Basic", Value: "basic"},
				{Name: "API Key", Value: "apiKey"},
			}, Required: false, Default: "none"},
			{Name: "apiKey", DisplayName: "API Key", Type: "password", Required: false, Description: "API key (alternative to Basic) for the REST Proxy."},
		},
		// Either `brokers` (native) or `restProxyUrl` (REST) is required.
		// We can't express the OR in the per-field schema, so callers
		// validate at Execute time.
		Required: []string{},
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

// credentialTypeSummary describes one supported credential type.
type credentialTypeSummary struct {
	Type        string `json:"type"`
	DisplayName string `json:"displayName"`
}

// ListCredentialTypes returns every credential type m9m ships a
// schema for. The UI's "Add Credential" chooser is built from this
// list so it can never drift from the backend — the hardcoded
// frontend list was missing mongoDb and kafka, which made those
// credentials impossible to create from the UI.
func (s *APIServer) ListCredentialTypes(w http.ResponseWriter, r *http.Request) {
	types := make([]credentialTypeSummary, 0, len(credentialSchemas))
	for _, schema := range credentialSchemas {
		types = append(types, credentialTypeSummary{
			Type:        schema.Type,
			DisplayName: schema.DisplayName,
		})
	}
	sort.Slice(types, func(i, j int) bool { return types[i].Type < types[j].Type })
	s.sendJSON(w, http.StatusOK, map[string]interface{}{"data": types})
}

// init registers the legacy n8n type spellings as aliases of their
// canonical schemas, and adds the schemas for the credential types that
// exist in the store but had no form definition (rabbitmq, redis).
// Without them the edit modal renders no fields at all — the user sees
// an empty form for exactly the credentials they want to fix.
func init() {
	for alias, source := range map[string]string{
		"mySql":     "mysql",
		"openAiApi": "openai",
	} {
		base, ok := credentialSchemas[source]
		if !ok {
			continue
		}
		clone := base
		clone.Type = alias
		credentialSchemas[alias] = clone
	}

	credentialSchemas["rabbitmq"] = CredentialTypeSchema{
		Type:        "rabbitmq",
		DisplayName: "RabbitMQ",
		Properties: []SchemaProperty{
			{Name: "host", DisplayName: "Host", Type: "string", Required: true, Default: "localhost"},
			{Name: "port", DisplayName: "Port", Type: "number", Required: true, Default: 5672},
			{Name: "user", DisplayName: "User", Type: "string", Required: false},
			{Name: "password", DisplayName: "Password", Type: "password", Required: false},
			{Name: "vhost", DisplayName: "Virtual Host", Type: "string", Required: false, Default: "/"},
			{Name: "ssl", DisplayName: "SSL", Type: "boolean", Required: false, Default: false},
		},
		Required: []string{"host"},
	}

	credentialSchemas["redis"] = CredentialTypeSchema{
		Type:        "redis",
		DisplayName: "Redis",
		Properties: []SchemaProperty{
			{Name: "host", DisplayName: "Host", Type: "string", Required: true, Default: "localhost"},
			{Name: "port", DisplayName: "Port", Type: "number", Required: true, Default: 6379},
			{Name: "password", DisplayName: "Password", Type: "password", Required: false},
			{Name: "db", DisplayName: "Database Index", Type: "number", Required: false, Default: 0},
			{Name: "tls", DisplayName: "TLS", Type: "boolean", Required: false, Default: false},
		},
		Required: []string{"host"},
	}

	// VCS and productivity credential schemas. The GitHub / GitLab nodes
	// read the flattened `githubApi_accessToken` / `gitlabApi_accessToken`
	// envelope, Google Sheets reads `googleSheetsOAuth2Api_accessToken`.
	credentialSchemas["githubApi"] = CredentialTypeSchema{
		Type:        "githubApi",
		DisplayName: "GitHub API",
		Properties: []SchemaProperty{
			{Name: "accessToken", DisplayName: "Access Token", Type: "password", Required: true, Description: "GitHub personal access token (ghp_...)"},
			{Name: "server", DisplayName: "GitHub Server", Type: "string", Required: false, Default: "https://api.github.com", Description: "API base URL (change for GitHub Enterprise)."},
		},
		Required: []string{"accessToken"},
	}

	credentialSchemas["gitlabApi"] = CredentialTypeSchema{
		Type:        "gitlabApi",
		DisplayName: "GitLab API",
		Properties: []SchemaProperty{
			{Name: "accessToken", DisplayName: "Access Token", Type: "password", Required: true, Description: "GitLab personal access token."},
			{Name: "server", DisplayName: "GitLab Server", Type: "string", Required: false, Default: "https://gitlab.com", Description: "Instance base URL (self-hosted GitLab). /api/v4 is appended automatically."},
		},
		Required: []string{"accessToken"},
	}

	credentialSchemas["googleSheetsOAuth2Api"] = CredentialTypeSchema{
		Type:        "googleSheetsOAuth2Api",
		DisplayName: "Google Sheets OAuth2 API",
		Properties: []SchemaProperty{
			{Name: "accessToken", DisplayName: "Access Token", Type: "password", Required: true, Description: "Google OAuth2 access token with the spreadsheets scope."},
			{Name: "clientId", DisplayName: "Client ID", Type: "string", Required: false},
			{Name: "clientSecret", DisplayName: "Client Secret", Type: "password", Required: false},
		},
		Required: []string{"accessToken"},
	}

	// Cloud storage credential schemas (batch). These flatten into the
	// node parameter surface the AWS / Azure / GCP nodes already read
	// (accessKeyId, secretAccessKey, connectionString, keyFileContent, …).
	credentialSchemas["aws"] = CredentialTypeSchema{
		Type:        "aws",
		DisplayName: "AWS",
		Properties: []SchemaProperty{
			{Name: "region", DisplayName: "Region", Type: "string", Required: true, Default: "us-east-1"},
			{Name: "accessKeyId", DisplayName: "Access Key ID", Type: "string", Required: true},
			{Name: "secretAccessKey", DisplayName: "Secret Access Key", Type: "password", Required: true},
			{Name: "sessionToken", DisplayName: "Session Token", Type: "password", Required: false},
			{Name: "endpoint", DisplayName: "Custom Endpoint", Type: "string", Required: false},
		},
		Required: []string{"region", "accessKeyId", "secretAccessKey"},
	}

	credentialSchemas["azureStorage"] = CredentialTypeSchema{
		Type:        "azureStorage",
		DisplayName: "Azure Storage",
		Properties: []SchemaProperty{
			{Name: "accountName", DisplayName: "Account Name", Type: "string", Required: false},
			{Name: "accountKey", DisplayName: "Account Key", Type: "password", Required: false},
			{Name: "sasToken", DisplayName: "SAS Token", Type: "password", Required: false},
			{Name: "connectionString", DisplayName: "Connection String", Type: "password", Required: false, Description: "Full connection string (overrides accountName/accountKey)."},
		},
		Required: []string{},
	}

	credentialSchemas["googleCloudStorage"] = CredentialTypeSchema{
		Type:        "googleCloudStorage",
		DisplayName: "Google Cloud Storage",
		Properties: []SchemaProperty{
			{Name: "projectId", DisplayName: "Project ID", Type: "string", Required: false},
			{Name: "keyFileContent", DisplayName: "Service Account JSON", Type: "json", Required: false, Description: "Service account key JSON (paste the whole file)."},
			{Name: "serviceAccountEmail", DisplayName: "Service Account Email", Type: "string", Required: false},
		},
		Required: []string{},
	}
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

	s.sendJSON(w, http.StatusOK, validateCredentialData(cred.Type, cred.Data))
}

// credentialTestPayload is the body of POST /credentials/test. The UI
// sends the credential type plus the *current form values* so the
// validation honours unsaved edits — "Test connection" must reflect
// what is in the form, not what is still stored.
type credentialTestPayload struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// TestCredentialPayload validates a credential payload that has NOT
// been saved yet. Same rules as the by-id test endpoint, but it works
// on the request body so the UI can test the edited values directly.
func (s *APIServer) TestCredentialPayload(w http.ResponseWriter, r *http.Request) {
	var req credentialTestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}
	if strings.TrimSpace(req.Type) == "" {
		s.sendError(w, http.StatusBadRequest, "type is required", nil)
		return
	}
	s.sendJSON(w, http.StatusOK, validateCredentialData(req.Type, req.Data))
}

// validateCredentialData holds the type-specific rules shared by the
// by-id test endpoint and the payload test endpoint.
func validateCredentialData(credType string, data map[string]interface{}) testCredentialResult {
	schema, ok := credentialSchemas[credType]
	if !ok {
		return testCredentialResult{
			Status:  "Error",
			Message: "Unknown credential type: " + credType,
		}
	}

	var missing []string
	for _, req := range schema.Required {
		// Alias keys (hostname → host, username → user) satisfy the
		// requirement too — synced n8n credentials use them.
		v, ok := credentialFieldValue(data, req)
		if !ok {
			missing = append(missing, req)
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			missing = append(missing, req)
		}
	}

	// Type-specific format checks.
	if credType == "oAuth2Api" {
		if u, ok := data["authUrl"].(string); ok && !isValidURL(u) {
			return testCredentialResult{Status: "Error", Message: "Authorization URL is not a valid URL"}
		}
		if u, ok := data["accessTokenUrl"].(string); ok && !isValidURL(u) {
			return testCredentialResult{Status: "Error", Message: "Access Token URL is not a valid URL"}
		}
	}
	if credType == "postgres" || credType == "mysql" {
		if p, ok := data["port"]; ok {
			switch v := p.(type) {
			case float64:
				if v < 1 || v > 65535 {
					return testCredentialResult{Status: "Error", Message: "Port out of range"}
				}
			case string:
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 || n > 65535 {
					return testCredentialResult{Status: "Error", Message: "Port out of range"}
				}
			}
		}
	}

	if len(missing) > 0 {
		return testCredentialResult{
			Status:  "Error",
			Message: "Missing or empty required fields: " + strings.Join(missing, ", "),
		}
	}

	// MongoDB can be configured with a connection string OR with
	// host/port/user/password — neither is in the required list, so
	// check explicitly before attempting a connection.
	if credType == "mongoDb" || credType == "mongodb" {
		if dataString(data, "connectionString") == "" && dataString(data, "host") == "" {
			return testCredentialResult{
				Status:  "Error",
				Message: "connectionString or host is required",
			}
		}
	}

	// Real connection check for the types where it is meaningful. A
	// wrong password must be reported as a failure — not "valid".
	if res, attempted := testCredentialConnection(credType, data); attempted {
		return res
	}

	return testCredentialResult{Status: "OK", Message: "Credential is valid"}
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
