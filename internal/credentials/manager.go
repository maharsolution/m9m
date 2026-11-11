package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// CredentialManager manages credentials for workflow nodes
type CredentialManager struct {
	store        *CredentialStore
	nodeMappings map[string]map[string]string // nodeID -> paramName -> credentialID
	mu           sync.RWMutex
}

// NewCredentialManager creates a new credential manager
func NewCredentialManager() (*CredentialManager, error) {
	store, err := NewCredentialStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create credential store: %w", err)
	}

	return &CredentialManager{
		store:        store,
		nodeMappings: make(map[string]map[string]string),
	}, nil
}

// RegisterNodeCredentials registers a credential for a specific node parameter
func (cm *CredentialManager) RegisterNodeCredentials(nodeID, paramName, credentialID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.nodeMappings[nodeID] == nil {
		cm.nodeMappings[nodeID] = make(map[string]string)
	}
	cm.nodeMappings[nodeID][paramName] = credentialID
}

// StoreCredentialFromWorkflow stores a credential from workflow data
func (cm *CredentialManager) StoreCredentialFromWorkflow(credData map[string]interface{}) error {
	// Extract credential information
	id, ok := credData["id"].(string)
	if !ok {
		return fmt.Errorf("credential id is required")
	}

	name, ok := credData["name"].(string)
	if !ok {
		return fmt.Errorf("credential name is required")
	}

	credType, ok := credData["type"].(string)
	if !ok {
		return fmt.Errorf("credential type is required")
	}

	// Extract data fields
	data := make(map[string]string)
	if dataField, exists := credData["data"]; exists {
		if dataMap, ok := dataField.(map[string]interface{}); ok {
			for k, v := range dataMap {
				if str, ok := v.(string); ok {
					data[k] = str
				} else if k == "oauthTokenData" {
					encoded, err := json.Marshal(v)
					if err != nil {
						return fmt.Errorf("failed to encode OAuth token data: %w", err)
					}
					data[k] = string(encoded)
				}
			}
		}
	}

	// Convert data to map[string]interface{}
	dataIntf := make(map[string]interface{})
	for k, v := range data {
		dataIntf[k] = v
	}

	credential := &Credential{
		ID:   id,
		Name: name,
		Type: credType,
		Data: dataIntf,
	}

	return cm.store.StoreCredential(credential)
}

// ResolveWorkflowCredentials resolves credentials for all nodes in a workflow
func (cm *CredentialManager) ResolveWorkflowCredentials(workflow *model.Workflow) error {
	for _, node := range workflow.Nodes {
		if len(node.Credentials) > 0 {
			for credType, credRef := range node.Credentials {
				if credRef.ID != "" {
					cm.RegisterNodeCredentials(node.ID, credType, credRef.ID)
				}
			}
		}
	}
	return nil
}

// InjectCredentialsIntoNodeParameters injects resolved credentials into node parameters
func (cm *CredentialManager) InjectCredentialsIntoNodeParameters(nodeID string, parameters map[string]interface{}) (map[string]interface{}, error) {
	creds, err := cm.GetNodeCredentials(nodeID)
	if err != nil {
		return nil, err
	}
	// TEMP DEBUG (BUG-2026-10-12-003): print what the engine sees for this
	// node so we can confirm whether the in-memory credential cache is in
	// sync with the bridge-pushed data.
	if os.Getenv("M9M_DEBUG_KAFKA_VALIDATE") != "" {
		cm.mu.RLock()
		mappings := cm.nodeMappings[nodeID]
		cm.mu.RUnlock()
		fmt.Fprintf(os.Stderr, "[InjectCredentialsIntoNodeParameters] nodeID=%s mappings=%d creds=%d keys=", nodeID, len(mappings), len(creds))
		for k := range creds {
			fmt.Fprintf(os.Stderr, "%s ", k)
		}
		fmt.Fprintf(os.Stderr, "\n")
	}

	// Create a copy of parameters and inject credentials
	result := make(map[string]interface{})
	for k, v := range parameters {
		result[k] = v
	}

	// Inject credential values
	for key, value := range creds {
		result[key] = value
	}

	return result, nil
}

// GetNodeCredentials retrieves credentials for a specific node
func (cm *CredentialManager) GetNodeCredentials(nodeID string) (map[string]string, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Get the credential mappings for this node
	mappings, exists := cm.nodeMappings[nodeID]
	if !exists {
		// No credentials registered for this node
		return make(map[string]string), nil
	}

	// Resolve each credential
	resolvedCredentials := make(map[string]string)

	for paramName, credentialID := range mappings {
		cred, err := cm.store.GetCredential(credentialID)
		if err != nil {
			// If credential not found, continue with empty value
			// This allows for graceful handling of missing credentials
			resolvedCredentials[paramName] = ""
			continue
		}

		// Resolve credential values, handling environment variables
		for key, value := range cred.Data {
			// Preserve structured OAuth token data across the string-valued
			// credential injection contract rather than producing Go map text.
			if key == "oauthTokenData" {
				if _, ok := value.(map[string]interface{}); ok {
					encoded, err := json.Marshal(value)
					if err != nil {
						return nil, fmt.Errorf("failed to encode OAuth token data: %w", err)
					}
					value = string(encoded)
				}
			}
			resolvedValue, err := cm.store.ResolveCredentialValue(value)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve credential value for %s.%s: %v", paramName, key, err)
			}

			// Store with prefixed key to avoid conflicts
			resolvedCredentials[fmt.Sprintf("%s_%s", paramName, key)] = resolvedValue
		}
	}

	return resolvedCredentials, nil
}

// LoadFromStorage pulls all credentials from the persistent storage backend
// (MySQL/Postgres/SQLite) and seeds them into the in-memory store used at
// execution time.
//
// Why this exists: the API layer persists credentials via
// storage.SaveCredential, while the engine reads them via the in-memory
// CredentialStore map. Without this seed, credentials written by the sync
// bridge (or any other API caller) after server startup are invisible to
// the workflow engine, so credential-injected node parameters stay empty
// and node validation rejects the workflow with messages like
// "either connectionUrl or host is required".
func (cm *CredentialManager) LoadFromStorage(store storage.WorkflowStorage) error {
	creds, err := store.ListCredentials()
	if err != nil {
		return fmt.Errorf("list credentials: %w", err)
	}
	for _, c := range creds {
		cm.store.credentials[c.ID] = &Credential{
			ID:   c.ID,
			Name: c.Name,
			Type: c.Type,
			Data: c.Data,
		}
	}
	return nil
}

// UpsertCredential writes a credential to the in-memory store that
// the engine reads at execution time. The API uses this after a
// successful storage.SaveCredential / storage.UpdateCredential so
// the in-memory map stays in sync with persistent storage without
// having to re-run LoadFromStorage (which would re-read the entire
// table and drop any in-flight node→credential mappings).
//
// We mirror the storage.Credential envelope as-is. The persistent
// storage layer persists `Data` as plain JSON (no encryption at
// that boundary; encryption happens inside the in-memory store on
// GetCredential when Encrypted=true), so the values we copy in
// here match exactly what LoadFromStorage would seed. Encrypted is
// left false so GetCredential returns the same plain map the engine
// already expected after a fresh LoadFromStorage.
func (cm *CredentialManager) UpsertCredential(c *storage.Credential) {
	if c == nil || c.ID == "" {
		return
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.store.credentials[c.ID] = &Credential{
		ID:   c.ID,
		Name: c.Name,
		Type: c.Type,
		Data: c.Data,
	}
}

// RemoveCredential evicts a credential from the in-memory store.
// Mirrors storage.DeleteCredential on the API side; without it, a
// credential deleted via the UI would still resolve at execution
// time (because the engine only knows about the in-memory map).
func (cm *CredentialManager) RemoveCredential(id string) {
	if id == "" {
		return
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.store.credentials, id)
}

// GetCredential fetches a credential from the in-memory store by
// id. Returns the same shape the engine sees via the store, so
// callers (including tests) can verify the write-through path
// without poking at internal maps.
func (cm *CredentialManager) GetCredential(id string) (*Credential, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.store.GetCredential(id)
}

// AccessTokenForType returns an OAuth access token for a stored
// credential of the given type.
//
// The token is read from the n8n `oauthTokenData.access_token`
// envelope first and falls back to the flat `accessToken` /
// `oauthToken` fields. Environment placeholders are resolved through
// the credential store, so a token stored as `${GOOGLE_TOKEN}` works
// as well.
//
// The lookup backs the `loadOptions` providers: the parameter panel
// asks the backend for the user's own files and the backend answers
// with the credential it already holds — the browser never sees the
// token.
func (cm *CredentialManager) AccessTokenForType(credentialType string) (string, error) {
	if cm == nil {
		return "", fmt.Errorf("credential manager is not initialized")
	}
	credentialType = strings.TrimSpace(credentialType)
	if credentialType == "" {
		return "", fmt.Errorf("credential type is required")
	}

	cm.mu.RLock()
	matches := make([]*Credential, 0, 4)
	for _, cred := range cm.store.credentials {
		if cred != nil && cred.Type == credentialType {
			matches = append(matches, cred)
		}
	}
	cm.mu.RUnlock()

	// Stable order so repeated calls answer with the same credential.
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })

	for _, cred := range matches {
		token := accessTokenFromData(cred.Data)
		if token == "" {
			continue
		}
		resolved, err := cm.store.ResolveCredentialValue(token)
		if err != nil || resolved == "" {
			continue
		}
		return resolved, nil
	}
	return "", fmt.Errorf("no %s credential with an access token is stored", credentialType)
}

// accessTokenFromData pulls the OAuth access token out of a credential
// payload. n8n stores it inside `oauthTokenData` (an object or its JSON
// text); m9m also accepts the flat `accessToken` field.
func accessTokenFromData(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	if raw, ok := data["oauthTokenData"]; ok {
		switch value := raw.(type) {
		case map[string]interface{}:
			if token, ok := value["access_token"].(string); ok && token != "" {
				return token
			}
		case string:
			var decoded map[string]interface{}
			if err := json.Unmarshal([]byte(value), &decoded); err == nil {
				if token, ok := decoded["access_token"].(string); ok && token != "" {
					return token
				}
			}
		}
	}
	for _, key := range []string{"accessToken", "oauthToken", "access_token"} {
		if token, ok := data[key].(string); ok && token != "" {
			return token
		}
	}
	return ""
}
