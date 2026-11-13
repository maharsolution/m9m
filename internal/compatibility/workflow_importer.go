package compatibility

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neul-labs/m9m/internal/model"
)

type N8nWorkflowImporter struct {
	nodeRegistry        map[string]*N8nNodeExecutor
	credentialsManager  *CredentialsManager
	expressionConverter *ExpressionConverter
}

type N8nWorkflowData struct {
	ID           interface{}                   `json:"id"`
	Name         string                        `json:"name"`
	Active       bool                          `json:"active"`
	Nodes        []N8nNodeData                 `json:"nodes"`
	Connections  map[string]N8nConnectionGroup `json:"connections"`
	Settings     map[string]interface{}        `json:"settings"`
	StaticData   map[string]interface{}        `json:"staticData"`
	Tags         []interface{}                 `json:"tags"`
	TriggerCount int                           `json:"triggerCount"`
	Meta         map[string]interface{}        `json:"meta"`
	PinData      map[string][]interface{}      `json:"pinData"`
	VersionID    string                        `json:"versionId"`
}

// N8nConnectionGroup holds the connections of one source node. n8n keys
// the connection map by node NAME (not id) and may carry connection
// types other than "main" (the AI connection types, for example).
// m9m's model only stores "main", so the other types are kept here and
// reported as warnings during import.
type N8nConnectionGroup struct {
	Main  [][]N8nConnection            `json:"main"`
	Other map[string][][]N8nConnection `json:"-"`
}

// UnmarshalJSON decodes a connection group and remembers every
// non-"main" connection type so the importer can report it.
func (g *N8nConnectionGroup) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if mainRaw, ok := raw["main"]; ok {
		if err := json.Unmarshal(mainRaw, &g.Main); err != nil {
			return err
		}
	}

	for key, value := range raw {
		if key == "main" {
			continue
		}
		var group [][]N8nConnection
		if err := json.Unmarshal(value, &group); err != nil {
			continue
		}
		if g.Other == nil {
			g.Other = make(map[string][][]N8nConnection)
		}
		g.Other[key] = group
	}

	return nil
}

type N8nNodeData struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	TypeVersion N8nTypeVersion         `json:"typeVersion"`
	Position    N8nPosition            `json:"position"`
	Parameters  map[string]interface{} `json:"parameters"`
	// Credentials holds both n8n shapes: the legacy string form
	// ({"httpBasicAuth": "My Cred"}) and the modern object form
	// ({"httpBasicAuth": {"id": "abc123", "name": "My Cred"}}).
	Credentials      map[string]interface{} `json:"credentials"`
	Disabled         bool                   `json:"disabled"`
	Notes            string                 `json:"notes"`
	Color            string                 `json:"color"`
	OnError          string                 `json:"onError"`
	RetryOnFail      bool                   `json:"retryOnFail"`
	MaxTries         int                    `json:"maxTries"`
	WaitBetweenTries int                    `json:"waitBetweenTries"`
	AlwaysOutputData bool                   `json:"alwaysOutputData"`
	ExecuteOnce      bool                   `json:"executeOnce"`
	ContinueOnFail   bool                   `json:"continueOnFail"`
	WebhookID        string                 `json:"webhookId"`
	Root             string                 `json:"root"`
}

// N8nPosition is the position of a node on the n8n canvas. n8n
// tolerates a malformed position (it falls back to [0, 0]), so the
// importer does the same instead of failing the whole workflow.
type N8nPosition []float64

// UnmarshalJSON decodes a position leniently: a non-numeric entry
// becomes 0 and a value that is not an array at all is ignored.
func (p *N8nPosition) UnmarshalJSON(data []byte) error {
	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		// Not an array (or not valid JSON at all): keep the zero
		// position instead of rejecting the workflow.
		*p = nil
		return nil
	}

	values := make(N8nPosition, 0, len(raw))
	for _, item := range raw {
		switch value := item.(type) {
		case float64:
			values = append(values, value)
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				values = append(values, 0)
				continue
			}
			values = append(values, parsed)
		default:
			values = append(values, 0)
		}
	}

	*p = values
	return nil
}

// N8nTypeVersion accepts the typeVersion shapes n8n emits: a number
// (2), a float (2.1) or a string ("2.1"). m9m stores an int, so the
// value is truncated to its integer part (n8n does the same when it
// resolves a node version).
type N8nTypeVersion float64

func (v *N8nTypeVersion) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*v = 0
		return nil
	}

	if strings.HasPrefix(trimmed, "\"") {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			// An unparsable version must not break the import; the
			// node keeps version 0 and a warning is reported.
			*v = 0
			return nil
		}
		*v = N8nTypeVersion(parsed)
		return nil
	}

	var parsed float64
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = N8nTypeVersion(parsed)
	return nil
}

type N8nConnection struct {
	Node        string `json:"node"`
	Type        string `json:"type"`
	Index       int    `json:"index"`
	OutputIndex int    `json:"outputIndex"`
}

type ExpressionConverter struct {
	// Handles conversion between n8n and n8n-go expression formats
}

// ImportOptions controls how an n8n payload is mapped onto m9m's
// workflow model. The zero value keeps the legacy behaviour of
// ImportWorkflow (the JSON "active" flag is honoured).
type ImportOptions struct {
	// HonourActive keeps the "active" flag of the JSON payload.
	// n8n's CLI import forces active=false unless the operator asks
	// for `activeState: fromJson`, so the HTTP import endpoint sets
	// this to false by default.
	HonourActive bool

	// CredentialResolver maps an n8n credential reference
	// (type + name + optional id) onto a credential that exists
	// in m9m. It returns the resolved id and the name m9m stores for
	// it. Returning ok=false means "not resolvable": the importer
	// keeps the credential name, leaves the id empty and reports a
	// warning instead of failing the import.
	CredentialResolver func(credType, name, id string) (resolvedID string, resolvedName string, ok bool)

	// IDExists reports whether a workflow id is already taken. When
	// it is, the importer mints a fresh id instead of overwriting an
	// unrelated workflow.
	IDExists func(id string) bool

	// KnownNodeTypes, when set, is used to flag node types m9m does
	// not implement. A nil map disables the check.
	KnownNodeTypes map[string]bool

	// NodeDisplayNames maps a node type onto its display name. It is
	// used to generate a readable name for nodes that arrive without
	// one - n8n's editor uses the node type display name for the
	// same purpose.
	NodeDisplayNames map[string]string

	// GenerateWorkflowID, GenerateNodeID and GenerateVersionID let
	// the caller control id generation. When nil, uuid v4 strings
	// are used.
	GenerateWorkflowID func() string
	GenerateNodeID     func() string
	GenerateVersionID  func() string

	// GenerateWebhookID lets the caller control the webhook id
	// generation. When nil, a nanoid-style id is used.
	GenerateWebhookID func() string
}

type ImportResult struct {
	Workflow         *model.Workflow   `json:"workflow"`
	ConversionIssues []ConversionIssue `json:"conversion_issues"`
	MissingNodes     []string          `json:"missing_nodes"`
	Statistics       *ImportStatistics `json:"statistics"`
	// SourceID is the workflow id carried by the n8n payload. It is
	// kept for traceability when m9m had to mint a different id.
	SourceID string `json:"source_id,omitempty"`
	// VersionID is the fresh version id assigned to the import.
	VersionID string `json:"version_id,omitempty"`
}

type ConversionIssue struct {
	Type       string `json:"type"` // "warning", "error", "info"
	NodeID     string `json:"node_id"`
	Field      string `json:"field"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	AutoFixed  bool   `json:"auto_fixed"`
}

type ImportStatistics struct {
	TotalNodes           int `json:"total_nodes"`
	ConvertedNodes       int `json:"converted_nodes"`
	SkippedNodes         int `json:"skipped_nodes"`
	TotalConnections     int `json:"total_connections"`
	ConvertedConnections int `json:"converted_connections"`
	ExpressionsConverted int `json:"expressions_converted"`
	CredentialsFound     int `json:"credentials_found"`
}

func NewN8nWorkflowImporter() *N8nWorkflowImporter {
	return &N8nWorkflowImporter{
		nodeRegistry:        make(map[string]*N8nNodeExecutor),
		credentialsManager:  &CredentialsManager{credentials: make(map[string]map[string]interface{})},
		expressionConverter: &ExpressionConverter{},
	}
}

func (importer *N8nWorkflowImporter) RegisterNode(nodeType string, executor *N8nNodeExecutor) {
	importer.nodeRegistry[nodeType] = executor
}

// ImportWorkflow converts an n8n workflow JSON document into m9m's
// workflow model.
//
// The legacy behaviour is preserved: the JSON "active" flag is
// honoured and expressions are kept verbatim (n8n expressions already
// run on m9m because its expression runtime exposes the same
// variables: $json, $parameter, $node, $workflow, $execution, ...).
//
// Use ImportWorkflowWithOptions for the n8n CLI import semantics
// (inactive by default, credential resolution, id collision handling).
func (importer *N8nWorkflowImporter) ImportWorkflow(n8nJson []byte) (*ImportResult, error) {
	return importer.ImportWorkflowWithOptions(n8nJson, ImportOptions{HonourActive: true})
}

// ImportWorkflowWithOptions converts an n8n workflow JSON document into
// m9m's workflow model following n8n's import contract:
//
//   - the source workflow id is preserved as a string when it is free,
//     and a fresh id is minted when it collides with an existing m9m
//     workflow (an unrelated workflow is never overwritten);
//   - a fresh version id is assigned on every import;
//   - nodes without an id get one;
//   - node credentials are normalised to the modern n8n shape
//     {type: {id, name}}; an id that cannot be resolved leaves the name
//     in place and is reported as a warning;
//   - tags are accepted as plain strings and as {id, name} objects.
func (importer *N8nWorkflowImporter) ImportWorkflowWithOptions(n8nJson []byte, opts ImportOptions) (*ImportResult, error) {
	var n8nWorkflow N8nWorkflowData
	if err := json.Unmarshal(n8nJson, &n8nWorkflow); err != nil {
		return nil, fmt.Errorf("failed to parse n8n workflow JSON: %w", err)
	}

	result := &ImportResult{
		ConversionIssues: []ConversionIssue{},
		MissingNodes:     []string{},
		Statistics: &ImportStatistics{
			TotalNodes:       len(n8nWorkflow.Nodes),
			TotalConnections: importer.countConnections(n8nWorkflow.Connections),
		},
	}

	// Workflow id: keep the n8n id as a string when it is free.
	sourceID := importer.convertWorkflowID(n8nWorkflow.ID)
	workflowID := sourceID
	if workflowID == "" {
		workflowID = importer.generateWorkflowID(opts)
	} else if opts.IDExists != nil && opts.IDExists(workflowID) {
		newID := importer.generateWorkflowID(opts)
		result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
			Type:       "warning",
			Field:      "id",
			Message:    fmt.Sprintf("workflow id %q already exists in m9m; the import was stored as %q", workflowID, newID),
			Suggestion: "Delete or rename the existing workflow to import under the original id",
		})
		workflowID = newID
	}

	// Every import gets a fresh version id so history ordering stays
	// correct (n8n assigns uuid() on every import).
	versionID := importer.generateVersionID(opts)

	active := n8nWorkflow.Active
	if !opts.HonourActive {
		active = false
	}

	name := strings.TrimSpace(n8nWorkflow.Name)
	if name == "" {
		name = "Imported workflow"
		result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
			Type:       "warning",
			Field:      "name",
			Message:    "the n8n workflow has no name; it was imported as \"Imported workflow\"",
			Suggestion: "Rename the workflow after the import",
		})
	}

	workflow := &model.Workflow{
		ID:          workflowID,
		Name:        name,
		Description: importer.extractDescription(n8nWorkflow),
		Active:      active,
		// n8n has no notion of the m9m Debug flag, so an imported
		// workflow always lands with Debug OFF.
		Debug:       false,
		Nodes:       []model.Node{},
		Connections: make(map[string]model.Connections),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		VersionID:   versionID,
		Tags:        importer.convertTags(n8nWorkflow.Tags, result),
		Settings:    importer.convertSettings(n8nWorkflow.Settings, result),
	}

	if n8nWorkflow.StaticData != nil {
		workflow.StaticData = n8nWorkflow.StaticData
	}
	if len(n8nWorkflow.PinData) > 0 {
		workflow.PinData = importer.convertPinData(n8nWorkflow.PinData)
	}

	// Convert nodes.
	//
	// n8n's editor drops nodes that have no type at all (they would
	// crash the canvas) and reports the count. The same happens
	// here: an unusable node is dropped and reported as a warning.
	validNodes := make([]N8nNodeData, 0, len(n8nWorkflow.Nodes))
	droppedWithoutType := 0
	for _, n8nNode := range n8nWorkflow.Nodes {
		if strings.TrimSpace(n8nNode.Type) == "" {
			droppedWithoutType++
			continue
		}
		validNodes = append(validNodes, n8nNode)
	}
	if droppedWithoutType > 0 {
		result.Statistics.SkippedNodes += droppedWithoutType
		result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
			Type:       "warning",
			Field:      "nodes",
			Message:    fmt.Sprintf("%d node(s) without a \"type\" were dropped", droppedWithoutType),
			Suggestion: "Give every node a type before importing",
		})
	}

	// n8n generates a unique name for every node that arrives without
	// one, derived from the node type display name.
	nodeNames := make(map[string]bool, len(validNodes))
	for _, n8nNode := range validNodes {
		if name := strings.TrimSpace(n8nNode.Name); name != "" {
			nodeNames[name] = true
		}
	}

	for _, n8nNode := range validNodes {
		if strings.TrimSpace(n8nNode.Name) == "" {
			n8nNode.Name = importer.generateNodeName(n8nNode.Type, nodeNames, opts)
			nodeNames[n8nNode.Name] = true
			result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
				Type:       "warning",
				NodeID:     n8nNode.ID,
				Field:      "name",
				Message:    fmt.Sprintf("node of type %q had no name; it was named %q", n8nNode.Type, n8nNode.Name),
				Suggestion: "Give the node a stable name in n8n before the next export",
			})
		}

		convertedNode, issues := importer.convertNode(n8nNode, opts, result)
		if convertedNode != nil {
			workflow.Nodes = append(workflow.Nodes, *convertedNode)
			result.Statistics.ConvertedNodes++
		} else {
			result.Statistics.SkippedNodes++
		}
		result.ConversionIssues = append(result.ConversionIssues, issues...)
	}

	// Convert connections. n8n keys the connection map by node NAME.
	convertedNodeNames := make(map[string]bool, len(workflow.Nodes))
	for _, node := range workflow.Nodes {
		convertedNodeNames[node.Name] = true
	}

	for sourceNodeName, connectionGroup := range n8nWorkflow.Connections {
		// n8n's editor sanitises the connections against the real
		// node names: a group of an unknown source node is dropped
		// completely.
		if !convertedNodeNames[sourceNodeName] {
			result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
				Type:       "warning",
				NodeID:     sourceNodeName,
				Field:      "connections",
				Message:    fmt.Sprintf("the connections of the unknown node %q were dropped", sourceNodeName),
				Suggestion: "Re-create the connection in the m9m editor",
			})
			continue
		}

		convertedGroup := model.Connections{
			Main: make([][]model.Connection, len(connectionGroup.Main)),
		}

		for outputIndex, outputConnections := range connectionGroup.Main {
			convertedGroup.Main[outputIndex] = make([]model.Connection, 0)
			for _, connection := range outputConnections {
				convertedConnection, issues := importer.convertConnection(
					sourceNodeName, outputIndex, connection, convertedNodeNames,
				)
				if convertedConnection != nil {
					convertedGroup.Main[outputIndex] = append(convertedGroup.Main[outputIndex], *convertedConnection)
					result.Statistics.ConvertedConnections++
				}
				result.ConversionIssues = append(result.ConversionIssues, issues...)
			}
		}

		workflow.Connections[sourceNodeName] = convertedGroup

		for connType, group := range connectionGroup.Other {
			count := 0
			for _, branch := range group {
				count += len(branch)
			}
			result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
				Type:       "warning",
				NodeID:     sourceNodeName,
				Field:      "connections." + connType,
				Message:    fmt.Sprintf("%d %q connection(s) from node %q were dropped: m9m only stores \"main\" connections", count, connType, sourceNodeName),
				Suggestion: "Re-create the connection in the m9m editor",
			})
		}
	}

	result.Workflow = workflow
	result.SourceID = sourceID
	result.VersionID = versionID
	return result, nil
}

func (importer *N8nWorkflowImporter) convertNode(n8nNode N8nNodeData, opts ImportOptions, result *ImportResult) (*model.Node, []ConversionIssue) {
	var issues []ConversionIssue

	// Check if we have a registered executor for this node type. The
	// registry is only populated by the compatibility runtime, so the
	// check is skipped when it is empty (the HTTP import path relies
	// on the engine's node catalogue instead).
	if len(importer.nodeRegistry) > 0 {
		_, exists := importer.nodeRegistry[n8nNode.Type]
		if !exists {
			issues = append(issues, ConversionIssue{
				Type:       "warning",
				NodeID:     n8nNode.ID,
				Message:    fmt.Sprintf("Node type '%s' not found in registry", n8nNode.Type),
				Suggestion: "Register the node type or use a compatible alternative",
			})

			// Check for built-in equivalents
			if equivalent := importer.findEquivalentNodeType(n8nNode.Type); equivalent != "" {
				issues = append(issues, ConversionIssue{
					Type:      "info",
					NodeID:    n8nNode.ID,
					Message:   fmt.Sprintf("Consider using '%s' as an alternative", equivalent),
					AutoFixed: false,
				})
			}
		}
	}

	if opts.KnownNodeTypes != nil && !opts.KnownNodeTypes[n8nNode.Type] {
		issues = append(issues, ConversionIssue{
			Type:       "warning",
			NodeID:     n8nNode.ID,
			Message:    fmt.Sprintf("Node type '%s' is not implemented by m9m; the node was imported as-is", n8nNode.Type),
			Suggestion: "Install a matching node type or replace the node in the editor",
		})
	}

	// Parameters are kept verbatim. n8n expressions run unchanged on
	// m9m because its expression runtime exposes the same variables
	// ($json, $parameter, $node, $workflow, $execution, $env, ...).
	convertedParams := n8nNode.Parameters
	if convertedParams == nil {
		convertedParams = make(map[string]interface{})
	}

	// n8n's editor regenerates the webhook id of webhook nodes so an
	// imported workflow never reuses the webhook id of the source
	// installation. The webhook path is kept in sync when it
	// referenced the old id.
	webhookID := n8nNode.WebhookID
	if webhookID != "" && isWebhookTriggerType(n8nNode.Type) {
		previousWebhookID := webhookID
		webhookID = importer.generateWebhookID(opts)

		pathMatched := false
		optionsPathMatched := false
		if path, ok := convertedParams["path"].(string); ok && path == previousWebhookID {
			pathMatched = true
		}
		if options, ok := convertedParams["options"].(map[string]interface{}); ok {
			if path, ok := options["path"].(string); ok && path == previousWebhookID {
				optionsPathMatched = true
			}
		}

		if pathMatched {
			convertedParams["path"] = webhookID
		} else if optionsPathMatched {
			convertedParams["options"].(map[string]interface{})["path"] = webhookID
		}
	}

	// Convert disabled flag to *bool
	var disabled *bool
	if n8nNode.Disabled {
		disabled = &n8nNode.Disabled
	}

	// Every node without an id gets one (n8n does the same on import).
	nodeID := strings.TrimSpace(n8nNode.ID)
	if nodeID == "" {
		nodeID = importer.generateNodeID(opts)
		issues = append(issues, ConversionIssue{
			Type:       "warning",
			Field:      "id",
			Message:    fmt.Sprintf("node %q had no id; it was assigned %q", n8nNode.Name, nodeID),
			Suggestion: "Give the node a stable id in n8n before the next export",
		})
	}

	credentials, credIssues := importer.convertCredentials(n8nNode.Credentials, opts)
	issues = append(issues, credIssues...)

	node := &model.Node{
		ID:               nodeID,
		Name:             n8nNode.Name,
		Type:             n8nNode.Type,
		TypeVersion:      int(n8nNode.TypeVersion),
		Position:         importer.convertPosition(n8nNode.Position),
		Parameters:       convertedParams,
		Credentials:      credentials,
		WebhookID:        webhookID,
		Notes:            n8nNode.Notes,
		Disabled:         disabled,
		OnError:          n8nNode.OnError,
		RetryOnFail:      n8nNode.RetryOnFail,
		MaxTries:         n8nNode.MaxTries,
		WaitBetweenTries: n8nNode.WaitBetweenTries,
		ContinueOnFail:   n8nNode.ContinueOnFail,
		AlwaysOutputData: n8nNode.AlwaysOutputData,
		ExecuteOnce:      n8nNode.ExecuteOnce,
		Root:             n8nNode.Root,
		Color:            n8nNode.Color,
	}

	return node, issues
}

// convertCredentials normalises both n8n credential shapes onto
// m9m's model. The modern shape is {type: {id, name}}; a credential
// whose id cannot be resolved keeps its name and reports a warning.
//
// The lookup follows n8n's `replaceInvalidCredentials`: a reference
// that carries an id is resolved by id AND type, a reference without
// an id is resolved by name AND type and only a unique match wins.
//
// Unlike n8n, which skips disabled nodes completely, m9m normalises
// every node the same way: an id that cannot be resolved is cleared
// so a stale reference never survives an import.
func (importer *N8nWorkflowImporter) convertCredentials(creds map[string]interface{}, opts ImportOptions) (map[string]model.Credential, []ConversionIssue) {
	var issues []ConversionIssue
	result := make(map[string]model.Credential)

	for credType, raw := range creds {
		if raw == nil {
			continue
		}

		var credID, credName string
		aiGatewayManaged := false
		switch value := raw.(type) {
		case string:
			// Legacy n8n shape: {"httpBasicAuth": "My Cred"}
			credName = value
		case map[string]interface{}:
			// Modern n8n shape: {"httpBasicAuth": {"id": "abc", "name": "My Cred"}}
			if id, ok := value["id"].(string); ok {
				credID = id
			}
			if name, ok := value["name"].(string); ok {
				credName = name
			}
			// AI Gateway managed credentials have no database record;
			// n8n skips them entirely (`workflow-helpers.ts`).
			if managed, ok := value["__aiGatewayManaged"].(bool); ok && managed {
				aiGatewayManaged = true
			}
		default:
			issues = append(issues, ConversionIssue{
				Type:       "warning",
				Field:      "credentials." + credType,
				Message:    fmt.Sprintf("credential %q has an unsupported shape and was dropped", credType),
				Suggestion: "Use the n8n shape {\"id\": \"...\", \"name\": \"...\"}",
			})
			continue
		}

		if credName == "" && credID == "" {
			continue
		}

		resolvedID := credID
		resolvedName := credName
		switch {
		case aiGatewayManaged:
			// n8n never resolves these, the reference is kept as it is.
			resolvedID = credID
		case opts.CredentialResolver != nil:
			id, name, ok := opts.CredentialResolver(credType, credName, credID)
			if ok {
				resolvedID = id
				if name != "" {
					resolvedName = name
				}
			} else {
				resolvedID = ""
				issues = append(issues, ConversionIssue{
					Type:       "warning",
					Field:      "credentials." + credType,
					Message:    fmt.Sprintf("credential %q (%s) could not be resolved in m9m; the name was kept and the id left empty", credName, credType),
					Suggestion: "Create the credential in m9m and re-link the node",
				})
			}
		}

		result[credType] = model.Credential{
			ID:   resolvedID,
			Name: resolvedName,
			Type: credType,
		}
	}

	return result, issues
}

func (importer *N8nWorkflowImporter) convertConnection(sourceNodeName string, outputIndex int, connection N8nConnection, nodeNames map[string]bool) (*model.Connection, []ConversionIssue) {
	var issues []ConversionIssue

	// n8n references nodes by NAME in the connection map.
	if !nodeNames[sourceNodeName] {
		issues = append(issues, ConversionIssue{
			Type:    "warning",
			Message: fmt.Sprintf("Source node '%s' not found in converted nodes", sourceNodeName),
		})
		return nil, issues
	}

	if !nodeNames[connection.Node] {
		issues = append(issues, ConversionIssue{
			Type:    "warning",
			Message: fmt.Sprintf("Target node '%s' not found in converted nodes", connection.Node),
		})
		return nil, issues
	}

	conn := &model.Connection{
		Node:  connection.Node,
		Type:  connection.Type,
		Index: connection.Index,
	}

	return conn, issues
}

func (importer *N8nWorkflowImporter) isN8nExpression(value string) bool {
	// Check for common n8n expression patterns
	patterns := []string{
		"={{",        // Standard expression
		"$json",      // JSON data access
		"$node",      // Node data access
		"$parameter", // Parameter access
		"$workflow",  // Workflow data
		"$execution", // Execution data
		"$binary",    // Binary data
		"$env",       // Environment variables
	}

	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}

	return false
}

func (importer *N8nWorkflowImporter) findEquivalentNodeType(nodeType string) string {
	// Map common n8n node types to n8n-go equivalents
	equivalents := map[string]string{
		"n8n-nodes-base.httpRequest":     "http-request",
		"n8n-nodes-base.set":             "set-data",
		"n8n-nodes-base.if":              "condition",
		"n8n-nodes-base.switch":          "switch",
		"n8n-nodes-base.merge":           "merge-data",
		"n8n-nodes-base.function":        "javascript-function",
		"n8n-nodes-base.webhook":         "webhook-trigger",
		"n8n-nodes-base.schedule":        "schedule-trigger",
		"n8n-nodes-base.manualTrigger":   "manual-trigger",
		"n8n-nodes-base.start":           "start",
		"n8n-nodes-base.executeWorkflow": "workflow-trigger",
		"n8n-nodes-base.wait":            "wait",
		"n8n-nodes-base.stopAndError":    "stop-error",
		"n8n-nodes-base.noOp":            "no-operation",
	}

	if equivalent, exists := equivalents[nodeType]; exists {
		return equivalent
	}

	// Try prefix matching for similar node types
	for n8nType, goType := range equivalents {
		if strings.Contains(nodeType, strings.TrimPrefix(n8nType, "n8n-nodes-base.")) {
			return goType
		}
	}

	return ""
}

// convertWorkflowID keeps the n8n workflow id as a string. Numbers are
// rendered without a fractional part; a missing id returns an empty
// string so the caller can generate one.
func (importer *N8nWorkflowImporter) convertWorkflowID(id interface{}) string {
	switch v := id.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func (importer *N8nWorkflowImporter) extractDescription(workflow N8nWorkflowData) string {
	if settings, ok := workflow.Settings["description"]; ok {
		if desc, ok := settings.(string); ok {
			return desc
		}
	}
	return ""
}

// convertTags accepts both n8n tag shapes: plain strings and
// {id, name} objects. m9m stores tag names only, so a tag id cannot
// be preserved; that is reported as a warning.
func (importer *N8nWorkflowImporter) convertTags(tags []interface{}, result *ImportResult) []string {
	var converted []string
	seen := make(map[string]bool)

	for _, tag := range tags {
		switch value := tag.(type) {
		case string:
			name := strings.TrimSpace(value)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			converted = append(converted, name)
		case map[string]interface{}:
			name, _ := value["name"].(string)
			name = strings.TrimSpace(name)
			id, _ := value["id"].(string)
			if name == "" {
				name = strings.TrimSpace(id)
			}
			if name == "" {
				if result != nil {
					result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
						Type:       "warning",
						Field:      "tags",
						Message:    "a tag entry has neither a name nor an id and was dropped",
						Suggestion: "Give the tag a name before exporting from n8n",
					})
				}
				continue
			}
			if id != "" && result != nil {
				result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
					Type:       "warning",
					Field:      "tags",
					Message:    fmt.Sprintf("tag id %q was not preserved: m9m stores workflow tags by name", id),
					Suggestion: "Create a matching tag in m9m to keep the tag id",
				})
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			converted = append(converted, name)
		default:
			if result != nil {
				result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
					Type:       "warning",
					Field:      "tags",
					Message:    "a tag entry has an unsupported shape and was dropped",
					Suggestion: "Use a tag name or an {id, name} object",
				})
			}
		}
	}

	return converted
}

// convertPosition mirrors n8n's ensureNodePosition: a position needs
// two finite numbers, everything else falls back to [0, 0].
func (importer *N8nWorkflowImporter) convertPosition(position N8nPosition) []int {
	if len(position) >= 2 {
		x, y := position[0], position[1]
		if isFiniteNumber(x) && isFiniteNumber(y) {
			return []int{int(x), int(y)}
		}
	}
	return []int{0, 0}
}

func isFiniteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// generateNodeName creates a readable, unique name for a node that
// arrives without one. n8n's editor uses the node type display name
// for the same purpose; the node type is the fallback.
func (importer *N8nWorkflowImporter) generateNodeName(nodeType string, used map[string]bool, opts ImportOptions) string {
	base := strings.TrimSpace(nodeType)
	if opts.NodeDisplayNames != nil {
		if display, ok := opts.NodeDisplayNames[nodeType]; ok && strings.TrimSpace(display) != "" {
			base = strings.TrimSpace(display)
		}
	}
	if base == "" {
		base = "Node"
	}
	return uniqueNodeName(base, used)
}

// uniqueNodeName mirrors n8n's uniqueNodeName: a free name is used
// as-is, otherwise a numeric suffix is appended ("Webhook" ->
// "Webhook1" -> "Webhook2").
func uniqueNodeName(original string, used map[string]bool) string {
	if original == "" {
		return original
	}
	if !used[original] {
		return original
	}

	// Strip an existing numeric suffix so "Webhook1" becomes
	// "Webhook2" and not "Webhook11".
	base := original
	for len(base) > 0 && base[len(base)-1] >= '0' && base[len(base)-1] <= '9' {
		base = base[:len(base)-1]
	}
	if base == "" {
		base = original
	}

	candidate := base
	for index := 1; used[candidate]; index++ {
		candidate = fmt.Sprintf("%s%d", base, index)
	}
	return candidate
}

// isWebhookTriggerType reports whether a node type carries a webhook
// id that has to be regenerated on import.
func isWebhookTriggerType(nodeType string) bool {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "n8n-nodes-base.webhook", "n8n-nodes-base.formtrigger", "webhook-trigger":
		return true
	}
	return false
}

func (importer *N8nWorkflowImporter) convertSettings(settings map[string]interface{}, result *ImportResult) *model.WorkflowSettings {
	if settings == nil {
		return nil
	}

	ws := &model.WorkflowSettings{}

	if order, ok := settings["executionOrder"].(string); ok {
		ws.ExecutionOrder = order
	}
	if tz, ok := settings["timezone"].(string); ok {
		ws.Timezone = tz
	}
	if saveErr, ok := settings["saveDataError"]; ok {
		ws.SaveDataError = saveErr
	}
	if saveSuccess, ok := settings["saveDataSuccess"]; ok {
		ws.SaveDataSuccess = saveSuccess
	}
	if saveManual, ok := settings["saveManualExecutions"]; ok {
		ws.SaveManualExecutions = saveManual
	}

	// Settings m9m does not model are reported once so the operator
	// knows they were dropped.
	supported := map[string]bool{
		"executionOrder":       true,
		"timezone":             true,
		"saveDataError":        true,
		"saveDataSuccess":      true,
		"saveManualExecutions": true,
		"description":          true,
	}
	var ignored []string
	for key := range settings {
		if !supported[key] {
			ignored = append(ignored, key)
		}
	}
	if len(ignored) > 0 && result != nil {
		sortStrings(ignored)
		result.ConversionIssues = append(result.ConversionIssues, ConversionIssue{
			Type:       "warning",
			Field:      "settings",
			Message:    fmt.Sprintf("workflow setting(s) %s are not modelled by m9m and were dropped", strings.Join(ignored, ", ")),
			Suggestion: "Store them in the node parameters instead",
		})
	}

	return ws
}

func (importer *N8nWorkflowImporter) convertPinData(pinData map[string][]interface{}) map[string][]model.DataItem {
	converted := make(map[string][]model.DataItem, len(pinData))
	for nodeName, items := range pinData {
		convertedItems := make([]model.DataItem, 0, len(items))
		for _, item := range items {
			if obj, ok := item.(map[string]interface{}); ok {
				convertedItems = append(convertedItems, model.DataItem{JSON: obj})
			}
		}
		converted[nodeName] = convertedItems
	}
	return converted
}

func (importer *N8nWorkflowImporter) countConnections(connections map[string]N8nConnectionGroup) int {
	count := 0
	for _, connectionGroup := range connections {
		for _, outputConnections := range connectionGroup.Main {
			count += len(outputConnections)
		}
	}
	return count
}

func (importer *N8nWorkflowImporter) generateWorkflowID(opts ImportOptions) string {
	if opts.GenerateWorkflowID != nil {
		return opts.GenerateWorkflowID()
	}
	return "workflow_" + uuid.NewString()
}

func (importer *N8nWorkflowImporter) generateNodeID(opts ImportOptions) string {
	if opts.GenerateNodeID != nil {
		return opts.GenerateNodeID()
	}
	return uuid.NewString()
}

func (importer *N8nWorkflowImporter) generateVersionID(opts ImportOptions) string {
	if opts.GenerateVersionID != nil {
		return opts.GenerateVersionID()
	}
	return uuid.NewString()
}

// generateWebhookID creates a fresh webhook id in n8n's nanoid style
// (16 URL-safe characters).
func (importer *N8nWorkflowImporter) generateWebhookID(opts ImportOptions) string {
	if opts.GenerateWebhookID != nil {
		return opts.GenerateWebhookID()
	}

	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return uuid.NewString()
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}

// ExportOptions controls how a workflow is written in n8n format.
type ExportOptions struct {
	// TagIDResolver maps a tag name onto the tag id m9m stores for
	// it. n8n exports tags as {id, name} objects; m9m keeps the name
	// on the workflow, so the id is looked up when possible.
	TagIDResolver func(name string) (id string, ok bool)
}

// ExportWorkflow writes a single workflow in n8n format. The shape
// matches what the n8n editor writes on "Download": one workflow
// object with name, nodes, connections, settings, staticData, pinData,
// meta, tags, active and versionId.
func (importer *N8nWorkflowImporter) ExportWorkflow(workflow *model.Workflow) ([]byte, error) {
	return importer.ExportWorkflowWithOptions(workflow, ExportOptions{})
}

// ExportWorkflows writes a list of workflows in n8n format. The shape
// matches n8n's CLI export (`n8n export:workflow`), which always
// writes an array - also for a single workflow.
func (importer *N8nWorkflowImporter) ExportWorkflows(workflows []*model.Workflow) ([]byte, error) {
	return importer.ExportWorkflowsWithOptions(workflows, ExportOptions{})
}

// ExportWorkflowsWithOptions writes a list of workflows in n8n format
// and resolves tag ids through the given resolver.
func (importer *N8nWorkflowImporter) ExportWorkflowsWithOptions(workflows []*model.Workflow, opts ExportOptions) ([]byte, error) {
	exported := make([]n8nExportWorkflow, 0, len(workflows))
	for _, workflow := range workflows {
		exported = append(exported, importer.buildExportWorkflow(workflow, opts))
	}
	return json.MarshalIndent(exported, "", "  ")
}

// ExportWorkflowWithOptions writes a single workflow in n8n format and
// resolves tag ids through the given resolver.
func (importer *N8nWorkflowImporter) ExportWorkflowWithOptions(workflow *model.Workflow, opts ExportOptions) ([]byte, error) {
	return json.MarshalIndent(importer.buildExportWorkflow(workflow, opts), "", "  ")
}

// n8nExportWorkflow is the n8n wire format of one workflow.
type n8nExportWorkflow struct {
	ID          string                          `json:"id"`
	Name        string                          `json:"name"`
	Active      bool                            `json:"active"`
	Nodes       []n8nExportNode                 `json:"nodes"`
	Connections map[string]n8nExportConnections `json:"connections"`
	Settings    map[string]interface{}          `json:"settings"`
	StaticData  map[string]interface{}          `json:"staticData"`
	PinData     map[string][]model.DataItem     `json:"pinData"`
	Meta        map[string]interface{}          `json:"meta"`
	Tags        []n8nExportTag                  `json:"tags"`
	VersionID   string                          `json:"versionId"`
}

// n8nExportNode is the n8n wire format of one node.
type n8nExportNode struct {
	ID               string                         `json:"id"`
	Name             string                         `json:"name"`
	Type             string                         `json:"type"`
	TypeVersion      int                            `json:"typeVersion"`
	Position         []int                          `json:"position"`
	Parameters       map[string]interface{}         `json:"parameters"`
	Credentials      map[string]n8nExportCredential `json:"credentials"`
	Disabled         bool                           `json:"disabled"`
	Notes            string                         `json:"notes,omitempty"`
	Color            string                         `json:"color,omitempty"`
	OnError          string                         `json:"onError,omitempty"`
	RetryOnFail      bool                           `json:"retryOnFail,omitempty"`
	MaxTries         int                            `json:"maxTries,omitempty"`
	WaitBetweenTries int                            `json:"waitBetweenTries,omitempty"`
	ContinueOnFail   bool                           `json:"continueOnFail,omitempty"`
	AlwaysOutputData bool                           `json:"alwaysOutputData,omitempty"`
	ExecuteOnce      bool                           `json:"executeOnce,omitempty"`
	WebhookID        string                         `json:"webhookId,omitempty"`
	Root             string                         `json:"root,omitempty"`
}

// n8nExportCredential is the modern n8n credential shape. The id is
// null when the credential could not be resolved in m9m - the same
// shape n8n writes for a credential it could not repair.
type n8nExportCredential struct {
	ID   *string `json:"id"`
	Name string  `json:"name"`
}

// n8nExportConnections is the n8n wire format of one node's
// connections.
type n8nExportConnections struct {
	Main [][]n8nExportConnection `json:"main"`
}

// n8nExportConnection is the n8n wire format of one connection.
type n8nExportConnection struct {
	Node  string `json:"node"`
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// n8nExportTag is the n8n wire format of one tag.
type n8nExportTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (importer *N8nWorkflowImporter) buildExportWorkflow(workflow *model.Workflow, opts ExportOptions) n8nExportWorkflow {
	exported := n8nExportWorkflow{
		ID:          workflow.ID,
		Name:        workflow.Name,
		Active:      workflow.Active,
		Nodes:       make([]n8nExportNode, 0, len(workflow.Nodes)),
		Connections: make(map[string]n8nExportConnections, len(workflow.Connections)),
		Settings:    importer.exportSettings(workflow.Settings),
		StaticData:  workflow.StaticData,
		PinData:     workflow.PinData,
		Meta:        map[string]interface{}{},
		Tags:        make([]n8nExportTag, 0, len(workflow.Tags)),
		VersionID:   workflow.VersionID,
	}

	if exported.StaticData == nil {
		exported.StaticData = map[string]interface{}{}
	}
	if exported.PinData == nil {
		exported.PinData = map[string][]model.DataItem{}
	}
	if exported.VersionID == "" {
		// n8n always writes a version id. A workflow that was created
		// inside m9m has none, so one is minted for the export.
		exported.VersionID = importer.generateVersionID(ImportOptions{})
	}

	for _, tag := range workflow.Tags {
		tagID := ""
		if opts.TagIDResolver != nil {
			if id, ok := opts.TagIDResolver(tag); ok {
				tagID = id
			}
		}
		exported.Tags = append(exported.Tags, n8nExportTag{ID: tagID, Name: tag})
	}

	for _, node := range workflow.Nodes {
		exportedNode := n8nExportNode{
			ID:               node.ID,
			Name:             node.Name,
			Type:             node.Type,
			TypeVersion:      node.TypeVersion,
			Position:         node.Position,
			Parameters:       node.Parameters,
			Disabled:         node.Disabled != nil && *node.Disabled,
			Notes:            node.Notes,
			Color:            node.Color,
			OnError:          node.OnError,
			RetryOnFail:      node.RetryOnFail,
			MaxTries:         node.MaxTries,
			WaitBetweenTries: node.WaitBetweenTries,
			ContinueOnFail:   node.ContinueOnFail,
			AlwaysOutputData: node.AlwaysOutputData,
			ExecuteOnce:      node.ExecuteOnce,
			WebhookID:        node.WebhookID,
			Root:             node.Root,
		}
		if exportedNode.Position == nil {
			exportedNode.Position = []int{0, 0}
		}
		if exportedNode.Parameters == nil {
			exportedNode.Parameters = map[string]interface{}{}
		}
		// n8n writes the credentials of every node, an empty object
		// when the node has none.
		exportedNode.Credentials = make(map[string]n8nExportCredential, len(node.Credentials))
		for credType, cred := range node.Credentials {
			// An unresolved credential keeps its name and carries a
			// null id, exactly like n8n's `{id: null, name}`.
			var credID *string
			if cred.ID != "" {
				id := cred.ID
				credID = &id
			}
			exportedNode.Credentials[credType] = n8nExportCredential{ID: credID, Name: cred.Name}
		}
		exported.Nodes = append(exported.Nodes, exportedNode)
	}

	for sourceNodeName, connectionGroup := range workflow.Connections {
		exportedGroup := n8nExportConnections{
			Main: make([][]n8nExportConnection, len(connectionGroup.Main)),
		}
		for outputIndex, outputConnections := range connectionGroup.Main {
			exportedGroup.Main[outputIndex] = make([]n8nExportConnection, 0, len(outputConnections))
			for _, connection := range outputConnections {
				exportedGroup.Main[outputIndex] = append(exportedGroup.Main[outputIndex], n8nExportConnection{
					Node:  connection.Node,
					Type:  connection.Type,
					Index: connection.Index,
				})
			}
		}
		exported.Connections[sourceNodeName] = exportedGroup
	}

	return exported
}

func (importer *N8nWorkflowImporter) exportSettings(settings *model.WorkflowSettings) map[string]interface{} {
	exported := map[string]interface{}{}
	if settings == nil {
		return exported
	}

	if settings.ExecutionOrder != "" {
		exported["executionOrder"] = settings.ExecutionOrder
	}
	if settings.Timezone != "" {
		exported["timezone"] = settings.Timezone
	}
	if settings.SaveDataError != nil {
		exported["saveDataError"] = settings.SaveDataError
	}
	if settings.SaveDataSuccess != nil {
		exported["saveDataSuccess"] = settings.SaveDataSuccess
	}
	if settings.SaveManualExecutions != nil {
		exported["saveManualExecutions"] = settings.SaveManualExecutions
	}

	return exported
}

// Expression converter methods
func (ec *ExpressionConverter) ConvertExpression(n8nExpression string) (string, error) {
	// Remove n8n expression wrapper if present
	expression := strings.TrimSpace(n8nExpression)
	if strings.HasPrefix(expression, "={{") && strings.HasSuffix(expression, "}}") {
		expression = strings.TrimSuffix(strings.TrimPrefix(expression, "={{"), "}}")
	}

	// Convert common n8n expression patterns to n8n-go format
	conversions := map[string]string{
		"$json":                "$item.json",
		"$node[":               "$nodes[",
		"$parameter[":          "$params[",
		"$workflow.id":         "$workflow.id",
		"$workflow.name":       "$workflow.name",
		"$execution.id":        "$execution.id",
		"$execution.mode":      "$execution.mode",
		"$binary":              "$binary",
		"$env":                 "$env",
		"$now":                 "$now",
		"moment()":             "moment()",
		"luxon.DateTime.now()": "moment()",
	}

	converted := expression
	for n8nPattern, goPattern := range conversions {
		converted = strings.ReplaceAll(converted, n8nPattern, goPattern)
	}

	// Handle complex patterns with regex if needed
	converted = ec.convertComplexPatterns(converted)

	return fmt.Sprintf("{{%s}}", converted), nil
}

func (ec *ExpressionConverter) convertComplexPatterns(expression string) string {
	// Convert $node["NodeName"].json to $nodes["NodeName"].json
	// Convert $parameter["paramName"] to $params["paramName"]
	// Add more complex pattern conversions as needed

	// For now, return as-is
	return expression
}

// ValidateN8nWorkflow checks an n8n workflow payload against n8n's
// import contract: the payload must carry BOTH a "nodes" and a
// "connections" property. n8n rejects the whole file otherwise
// ("File does not seem to contain valid workflows.").
func (importer *N8nWorkflowImporter) ValidateN8nWorkflow(n8nJson []byte) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(n8nJson, &payload); err != nil {
		return fmt.Errorf("invalid n8n workflow JSON: %w", err)
	}

	if _, ok := payload["nodes"]; !ok {
		return fmt.Errorf("File does not seem to contain valid workflows: missing \"nodes\" property")
	}
	if _, ok := payload["connections"]; !ok {
		return fmt.Errorf("File does not seem to contain valid workflows: missing \"connections\" property")
	}

	return nil
}

// sortStrings sorts a slice of strings in place. It keeps warning
// messages deterministic.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
