package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// nodeTypeOptionTimeout bounds how long a dynamic-option provider may
// run. A provider that talks to a remote service (n8n's `loadOptions`
// semantics) must never hang the API request — same reasoning as
// `credentialConnectTimeout` in credential_conn.go.
const nodeTypeOptionTimeout = 10 * time.Second

// maxNodeTypeOptionBodyBytes caps the POST body we are willing to read.
const maxNodeTypeOptionBodyBytes = 1 << 20 // 1 MiB

// NodeTypeOption is one entry of a dynamic option list.
//
// The wire shape is exactly what the frontend client expects
// (web/src/api/nodeTypes.ts → `NodeTypeOption`): `{name, value}`.
// `description` is additive — clients that do not know it ignore it.
type NodeTypeOption struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// NodeTypeOptionRequest is the context handed to an option provider.
type NodeTypeOptionRequest struct {
	// NodeType is the registered node type id (e.g. "n8n-nodes-base.httpRequest").
	NodeType string
	// Property is the node property the option list belongs to.
	Property string
	// Action is the `options[].action` value declared by the property.
	Action string
	// Params carries the current parameter values of the node (may be nil).
	Params map[string]interface{}
	// Credentials resolves the access token of a stored credential.
	//
	// The parameter panel only sends node parameters — never secrets —
	// so a provider that lists the user's own files (spreadsheets,
	// repositories, buckets) reads its token from the server-side
	// credential store through this resolver.
	//
	// May be nil when no credential manager is wired.
	Credentials CredentialResolver
}

// CredentialResolver resolves an access token for a credential type.
//
// Implemented by *credentials.CredentialManager.
type CredentialResolver interface {
	// AccessTokenForType returns a usable OAuth access token for the
	// first stored credential of the given type.
	AccessTokenForType(credentialType string) (string, error)
}

// OptionProvider resolves the dynamic option list for one action.
//
// Implementations MUST honour ctx — the handler wraps every call in a
// bounded timeout, so a provider that ignores ctx can still not hang
// the request, it is merely abandoned in the background.
type OptionProvider func(ctx context.Context, req NodeTypeOptionRequest) ([]NodeTypeOption, error)

// optionProviderKey identifies a registered provider. An empty
// nodeType and/or property act as wildcards.
type optionProviderKey struct {
	nodeType string
	property string
	action   string
}

// OptionProviderRegistry holds the registered dynamic-option providers.
//
// Lookups are tried from most to least specific:
//
//	(nodeType, property, action)
//	(nodeType, "",        action)
//	("",        property, action)
//	("",        "",        action)
//
// so a node can register a provider for one property and still fall
// back to a shared provider for the same action name.
type OptionProviderRegistry struct {
	mu        sync.RWMutex
	providers map[optionProviderKey]OptionProvider
}

// NewOptionProviderRegistry returns an empty registry.
func NewOptionProviderRegistry() *OptionProviderRegistry {
	return &OptionProviderRegistry{providers: make(map[optionProviderKey]OptionProvider)}
}

// Register adds (or replaces) a provider for one
// (node type, property, action) triple. Empty nodeType/property are
// wildcards.
func (r *OptionProviderRegistry) Register(nodeType, property, action string, provider OptionProvider) {
	if r == nil || provider == nil {
		return
	}
	key := optionProviderKey{nodeType: nodeType, property: property, action: action}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers == nil {
		r.providers = make(map[optionProviderKey]OptionProvider)
	}
	r.providers[key] = provider
}

// RegisterAction adds a provider for an action name across every node
// type and property.
func (r *OptionProviderRegistry) RegisterAction(action string, provider OptionProvider) {
	r.Register("", "", action, provider)
}

// Unregister removes a provider. It is a no-op when nothing is
// registered for the key.
func (r *OptionProviderRegistry) Unregister(nodeType, property, action string) {
	if r == nil {
		return
	}
	key := optionProviderKey{nodeType: nodeType, property: property, action: action}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, key)
}

// Lookup returns the provider that matches the given
// (node type, property, action) triple, most specific match first.
func (r *OptionProviderRegistry) Lookup(nodeType, property, action string) (OptionProvider, bool) {
	if r == nil {
		return nil, false
	}
	candidates := []optionProviderKey{
		{nodeType: nodeType, property: property, action: action},
		{nodeType: nodeType, property: "", action: action},
		{nodeType: "", property: property, action: action},
		{nodeType: "", property: "", action: action},
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, key := range candidates {
		if provider, ok := r.providers[key]; ok && provider != nil {
			return provider, true
		}
	}
	return nil, false
}

// defaultOptionProviders is the process-wide registry used by the
// HTTP handler. It is package-level because the API server struct
// cannot be extended from this file — node packages (or the wiring in
// cmd/m9m) register their providers against it.
var defaultOptionProviders = NewOptionProviderRegistry()

// RegisterOptionProvider registers a provider for one
// (node type, property, action) triple.
//
// A node declares the action on its property metadata
// (base.Option.Action); the backend maps that action onto a provider
// here. Example:
//
//	api.RegisterOptionProvider(
//	    "n8n-nodes-base.googleSheets",
//	    "sheet",
//	    "loadSheets",
//	    func(ctx context.Context, req api.NodeTypeOptionRequest) ([]api.NodeTypeOption, error) {
//	        // … talk to the Google API, honouring ctx …
//	        return []api.NodeTypeOption{{Name: "Sheet1", Value: "sheet1"}}, nil
//	    },
//	)
func RegisterOptionProvider(nodeType, property, action string, provider OptionProvider) {
	defaultOptionProviders.Register(nodeType, property, action, provider)
}

// RegisterActionOptionProvider registers a provider for an action name
// across every node type and property.
func RegisterActionOptionProvider(action string, provider OptionProvider) {
	defaultOptionProviders.RegisterAction(action, provider)
}

// UnregisterOptionProvider removes a previously registered provider.
func UnregisterOptionProvider(nodeType, property, action string) {
	defaultOptionProviders.Unregister(nodeType, property, action)
}

// nodeTypeOptionRequest is the inbound request payload.
//
// GET  → query parameters: ?property=<name>[&params=<json object>]
// POST → JSON body:        {"property": "<name>", "params": {…}}
type nodeTypeOptionRequest struct {
	Property string                 `json:"property"`
	Params   map[string]interface{} `json:"params"`
}

// nodeTypeOptionResponse is the response envelope.
//
// The frontend client (`unwrapOptions` in web/src/api/nodeTypes.ts)
// accepts a bare array or an object carrying a `data` array. We send
// the object form because it can also carry the human-readable
// `message` that explains why the list is empty.
//
//	{
//	  "data": [{"name": "Raw", "value": "raw"}],
//	  "message": "…"
//	}
type nodeTypeOptionResponse struct {
	Data    []NodeTypeOption `json:"data"`
	Message string           `json:"message,omitempty"`
}

// GetNodeTypeOptions serves the dynamic option list of a node property.
//
// GET  /api/v1/node-types/{name}/options?property=<property>
// POST /api/v1/node-types/{name}/options  {"property":"…","params":{…}}
//
// The property is looked up on the registered node type, its
// `options[].action` is resolved through the provider registry and the
// matching provider produces the list.
//
// Behaviour:
//   - unknown node type            → 404
//   - unknown/missing property     → 400
//   - malformed request body       → 400
//   - property without an action   → 200 + empty list + message
//   - action without a provider    → 200 + empty list + message
//   - provider failure             → 200 + empty list + message
//
// The empty-list cases are deliberate: the parameter panel degrades to
// a plain text input instead of breaking, so this endpoint never
// answers 5xx for a missing provider.
func (s *APIServer) GetNodeTypeOptions(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(mux.Vars(r)["name"])
	if name == "" {
		s.sendError(w, http.StatusBadRequest, "node type name is required", nil)
		return
	}

	payload, err := parseNodeTypeOptionRequest(r)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	nodeType, ok := s.findNodeType(name)
	if !ok {
		s.sendError(w, http.StatusNotFound, "node type not found: "+name, nil)
		return
	}

	property, ok := findNodeProperty(nodeType, payload.Property)
	if !ok {
		s.sendError(w, http.StatusBadRequest,
			fmt.Sprintf("property %q not found on node type %s", payload.Property, name), nil)
		return
	}

	action := propertyAction(property)
	if action == "" {
		s.sendJSON(w, http.StatusOK, nodeTypeOptionResponse{
			Data:    []NodeTypeOption{},
			Message: fmt.Sprintf("property %q of node type %s declares no dynamic option action", property.Name, name),
		})
		return
	}

	provider, ok := defaultOptionProviders.Lookup(name, property.Name, action)
	if !ok {
		s.sendJSON(w, http.StatusOK, nodeTypeOptionResponse{
			Data:    []NodeTypeOption{},
			Message: fmt.Sprintf("no dynamic options are available for action %q", action),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), nodeTypeOptionTimeout)
	defer cancel()

	// The provider never receives credentials through the request body —
	// it resolves them from the server-side store.
	var resolver CredentialResolver
	if s.credMgr != nil {
		resolver = s.credMgr
	}

	options, err := provider(ctx, NodeTypeOptionRequest{
		NodeType:    name,
		Property:    property.Name,
		Action:      action,
		Params:      payload.Params,
		Credentials: resolver,
	})
	if err != nil {
		log.Printf("[node-type-options] provider for action %q failed: %v", action, err)
		s.sendJSON(w, http.StatusOK, nodeTypeOptionResponse{
			Data:    []NodeTypeOption{},
			Message: fmt.Sprintf("failed to load dynamic options for action %q: %s", action, err.Error()),
		})
		return
	}

	if options == nil {
		options = []NodeTypeOption{}
	}
	s.sendJSON(w, http.StatusOK, nodeTypeOptionResponse{Data: options})
}

// findNodeType returns the registered node type with the given id.
func (s *APIServer) findNodeType(name string) (engine.NodeTypeInfo, bool) {
	if s.engine == nil {
		return engine.NodeTypeInfo{}, false
	}
	for _, nt := range s.engine.GetRegisteredNodeTypes() {
		if nt.TypeID == name {
			return nt, true
		}
	}
	return engine.NodeTypeInfo{}, false
}

// findNodeProperty returns the named property of a node type.
func findNodeProperty(nodeType engine.NodeTypeInfo, property string) (base.NodeProperty, bool) {
	for _, prop := range nodeType.Properties {
		if prop.Name == property {
			return prop, true
		}
	}
	return base.NodeProperty{}, false
}

// propertyAction returns the first non-empty `options[].action` of a
// property — the backend action that resolves its dynamic option list.
func propertyAction(property base.NodeProperty) string {
	for _, option := range property.Options {
		if strings.TrimSpace(option.Action) != "" {
			return strings.TrimSpace(option.Action)
		}
	}
	return ""
}

// parseNodeTypeOptionRequest extracts the property name and the
// current parameter values from either the query string (GET) or the
// JSON body (POST).
func parseNodeTypeOptionRequest(r *http.Request) (nodeTypeOptionRequest, error) {
	var payload nodeTypeOptionRequest

	if r.Method == http.MethodGet {
		query := r.URL.Query()
		payload.Property = query.Get("property")
		if raw := strings.TrimSpace(query.Get("params")); raw != "" {
			if err := json.Unmarshal([]byte(raw), &payload.Params); err != nil {
				return payload, fmt.Errorf("invalid params: %s", err.Error())
			}
		}
	} else {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxNodeTypeOptionBodyBytes))
		if err != nil {
			return payload, fmt.Errorf("failed to read request body: %s", err.Error())
		}
		if len(bytes.TrimSpace(body)) == 0 {
			return payload, errors.New("request body is required")
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return payload, fmt.Errorf("invalid JSON body: %s", err.Error())
		}
	}

	payload.Property = strings.TrimSpace(payload.Property)
	if payload.Property == "" {
		return payload, errors.New("property is required")
	}
	return payload, nil
}
