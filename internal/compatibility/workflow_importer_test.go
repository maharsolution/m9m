package compatibility

import (
	"encoding/json"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credentialWorkflow builds a one-node workflow document with the
// given credential reference.
func credentialWorkflow(id, name string, credentials string, disabled bool) []byte {
	disabledField := ""
	if disabled {
		disabledField = `"disabled": true,`
	}
	return []byte(`{
      "id": "` + id + `",
      "name": "` + name + `",
      "nodes": [
        {
          "id": "n1",
          "name": "HTTP Request",
          "type": "n8n-nodes-base.httpRequest",
          "typeVersion": 1,
          "position": [0, 0],
          ` + disabledField + `
          "parameters": {},
          "credentials": ` + credentials + `
        }
      ],
      "connections": {}
    }`)
}

// exportedCredential is the exported n8n credential shape.
type exportedCredential struct {
	ID   *string `json:"id"`
	Name string  `json:"name"`
}

// exportedNode is the exported n8n node shape.
type exportedNode struct {
	Name        string                        `json:"name"`
	Type        string                        `json:"type"`
	Credentials map[string]exportedCredential `json:"credentials"`
}

// exportedWorkflow is the exported n8n workflow shape.
type exportedWorkflow struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Active    bool                     `json:"active"`
	Nodes     []exportedNode           `json:"nodes"`
	VersionID string                   `json:"versionId"`
	Tags      []map[string]interface{} `json:"tags"`
	Meta      map[string]interface{}   `json:"meta"`
}

// TestImportWorkflow_LegacyCredentialBecomesNewShape pins the n8n
// shape conversion: the legacy string shape becomes the modern
// {id, name} shape.
func TestImportWorkflow_LegacyCredentialBecomesNewShape(t *testing.T) {
	importer := NewN8nWorkflowImporter()

	document := credentialWorkflow(
		"wf-legacy-credential",
		"Legacy credential",
		`{ "httpBasicAuth": "My Cred" }`,
		false,
	)

	result, err := importer.ImportWorkflowWithOptions(document, ImportOptions{})
	require.NoError(t, err)
	require.Len(t, result.Workflow.Nodes, 1)

	credential := result.Workflow.Nodes[0].Credentials["httpBasicAuth"]
	assert.Equal(t, "My Cred", credential.Name)
	assert.Empty(t, credential.ID)

	exported, err := importer.ExportWorkflow(result.Workflow)
	require.NoError(t, err)

	var document2 exportedWorkflow
	require.NoError(t, json.Unmarshal(exported, &document2))
	require.Len(t, document2.Nodes, 1)
	basic := document2.Nodes[0].Credentials["httpBasicAuth"]
	assert.Nil(t, basic.ID, "an unresolved credential is exported with a null id")
	assert.Equal(t, "My Cred", basic.Name)
}

// TestImportWorkflow_CredentialResolverWins pins the resolution order:
// the resolver decides which credential m9m links the node to.
func TestImportWorkflow_CredentialResolverWins(t *testing.T) {
	importer := NewN8nWorkflowImporter()

	resolver := func(credType, name, id string) (string, string, bool) {
		if credType == "httpBasicAuth" && name == "My Cred" {
			return "cred-42", "My Cred", true
		}
		return "", "", false
	}

	document := credentialWorkflow(
		"wf-resolved-credential",
		"Resolved credential",
		`{ "httpBasicAuth": { "id": "stale", "name": "My Cred" } }`,
		false,
	)

	result, err := importer.ImportWorkflowWithOptions(document, ImportOptions{
		CredentialResolver: resolver,
	})
	require.NoError(t, err)
	require.Len(t, result.Workflow.Nodes, 1)

	credential := result.Workflow.Nodes[0].Credentials["httpBasicAuth"]
	assert.Equal(t, "cred-42", credential.ID)
	assert.Equal(t, "My Cred", credential.Name)
	assert.Empty(t, result.ConversionIssues)
}

// TestImportWorkflow_UnresolvedCredentialWarnsAndKeepsName pins the
// non-fatal rule: a credential that cannot be resolved never fails the
// batch, the name is kept and a warning is reported.
func TestImportWorkflow_UnresolvedCredentialWarnsAndKeepsName(t *testing.T) {
	importer := NewN8nWorkflowImporter()

	document := credentialWorkflow(
		"wf-unresolved-credential",
		"Unresolved credential",
		`{ "httpBasicAuth": { "id": "gone", "name": "Missing Cred" } }`,
		false,
	)

	result, err := importer.ImportWorkflowWithOptions(document, ImportOptions{
		CredentialResolver: func(credType, name, id string) (string, string, bool) {
			return "", "", false
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Workflow.Nodes, 1)

	credential := result.Workflow.Nodes[0].Credentials["httpBasicAuth"]
	assert.Empty(t, credential.ID)
	assert.Equal(t, "Missing Cred", credential.Name)

	var warned bool
	for _, issue := range result.ConversionIssues {
		if issue.Type == "warning" && issue.Field == "credentials.httpBasicAuth" {
			warned = true
		}
	}
	assert.True(t, warned, "an unresolved credential has to be reported as a warning")
}

// TestImportWorkflow_DisabledNodeCredentialIsNormalized pins the m9m
// rule: every node is normalised the same way, so a disabled node does
// not keep a dangling credential reference.
func TestImportWorkflow_DisabledNodeCredentialIsNormalized(t *testing.T) {
	importer := NewN8nWorkflowImporter()

	document := credentialWorkflow(
		"wf-disabled-credential",
		"Disabled credential",
		`{ "httpBasicAuth": { "id": "gone", "name": "Missing Cred" } }`,
		true,
	)

	result, err := importer.ImportWorkflowWithOptions(document, ImportOptions{
		CredentialResolver: func(credType, name, id string) (string, string, bool) {
			return "", "", false
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Workflow.Nodes, 1)

	credential := result.Workflow.Nodes[0].Credentials["httpBasicAuth"]
	assert.Empty(t, credential.ID)
	assert.Equal(t, "Missing Cred", credential.Name)

	var warned bool
	for _, issue := range result.ConversionIssues {
		if issue.Type == "warning" && issue.Field == "credentials.httpBasicAuth" {
			warned = true
		}
	}
	assert.True(t, warned)
}

// TestImportWorkflow_AIGatewayManagedCredentialIsSkipped pins the
// n8n rule that AI Gateway managed credentials are never resolved.
func TestImportWorkflow_AIGatewayManagedCredentialIsSkipped(t *testing.T) {
	importer := NewN8nWorkflowImporter()

	called := false
	document := credentialWorkflow(
		"wf-ai-gateway",
		"AI Gateway credential",
		`{ "httpBasicAuth": { "id": "managed", "name": "AI Cred", "__aiGatewayManaged": true } }`,
		false,
	)

	result, err := importer.ImportWorkflowWithOptions(document, ImportOptions{
		CredentialResolver: func(credType, name, id string) (string, string, bool) {
			called = true
			return "cred-42", "AI Cred", true
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Workflow.Nodes, 1)

	assert.False(t, called, "an AI Gateway managed credential is never resolved")
	credential := result.Workflow.Nodes[0].Credentials["httpBasicAuth"]
	assert.Equal(t, "managed", credential.ID)
	assert.Equal(t, "AI Cred", credential.Name)
}

// TestExportWorkflow_AlwaysWritesVersionID pins the export contract:
// n8n always writes a version id.
func TestExportWorkflow_AlwaysWritesVersionID(t *testing.T) {
	importer := NewN8nWorkflowImporter()

	exported, err := importer.ExportWorkflow(&model.Workflow{
		ID:          "wf-no-version",
		Name:        "No version",
		Nodes:       []model.Node{},
		Connections: map[string]model.Connections{},
	})
	require.NoError(t, err)

	var document exportedWorkflow
	require.NoError(t, json.Unmarshal(exported, &document))
	assert.NotEmpty(t, document.VersionID)
}

// TestExportWorkflows_AlwaysWritesAnArray pins the n8n CLI parity:
// a bulk export writes an array, also for a single workflow.
func TestExportWorkflows_AlwaysWritesAnArray(t *testing.T) {
	importer := NewN8nWorkflowImporter()

	exported, err := importer.ExportWorkflows([]*model.Workflow{{
		ID:          "wf-1",
		Name:        "One",
		Nodes:       []model.Node{},
		Connections: map[string]model.Connections{},
	}})
	require.NoError(t, err)

	var documents []exportedWorkflow
	require.NoError(t, json.Unmarshal(exported, &documents))
	require.Len(t, documents, 1)
	assert.Equal(t, "wf-1", documents[0].ID)
}
