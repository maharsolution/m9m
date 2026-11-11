/*
Package nodemetatest holds the metadata quality gate for every node
type m9m registers.

The catalog mirrors the registration list in
cmd/m9m/commands/exec.go — the same list the REST endpoint
GET /api/v1/node-types exposes — and asserts the parameter metadata
the web parameter panel depends on:

  - every node type has a display name, a description and at least one
    property;
  - every property has a name, a display name, a type, a description
    and a non-nil default;
  - properties that accept a fixed choice set carry their options;
  - a large share of the node types declares conditional visibility
    (displayOptions) for their mode/resource/operation specific
    parameters;
  - every conditional rule references a sibling parameter the node
    actually declares.

The package holds test code only; it deliberately lives outside
internal/nodes/base so it never collides with the base package owner.
*/
package nodemetatest

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/nodes/ai"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/cli"
	"github.com/neul-labs/m9m/internal/nodes/cloud/aws"
	"github.com/neul-labs/m9m/internal/nodes/cloud/azure"
	"github.com/neul-labs/m9m/internal/nodes/cloud/gcp"
	"github.com/neul-labs/m9m/internal/nodes/code"
	"github.com/neul-labs/m9m/internal/nodes/core"
	"github.com/neul-labs/m9m/internal/nodes/data"
	"github.com/neul-labs/m9m/internal/nodes/database"
	"github.com/neul-labs/m9m/internal/nodes/email"
	"github.com/neul-labs/m9m/internal/nodes/file"
	httpnode "github.com/neul-labs/m9m/internal/nodes/http"
	"github.com/neul-labs/m9m/internal/nodes/messaging"
	"github.com/neul-labs/m9m/internal/nodes/productivity"
	"github.com/neul-labs/m9m/internal/nodes/timer"
	"github.com/neul-labs/m9m/internal/nodes/transform"
	"github.com/neul-labs/m9m/internal/nodes/trigger"
	"github.com/neul-labs/m9m/internal/nodes/vcs"
)

// expectedNodeTypes is the number of node types the engine registers
// (see RegisterAllNodes in cmd/m9m/commands/exec.go).
const expectedNodeTypes = 52

// minDisplayOptionsNodeTypes is the share of node types that must
// declare at least one conditional visibility rule. Conditional
// parameters drive the n8n-style parameter panel: without them the
// panel renders fields the node ignores.
const minDisplayOptionsNodeTypes = 40

// nodeEntry pairs a registered node type id with its executor and the
// implementation package it lives in.
type nodeEntry struct {
	TypeID string
	Pkg    string
	Node   base.NodeExecutor
}

// stubDataStore is a no-op data.DataTableStore. The Data Table node
// only needs a store to exist; the metadata gate never executes it.
type stubDataStore struct{}

func (stubDataStore) SaveRaw(key string, value []byte) error { return nil }
func (stubDataStore) GetRaw(key string) ([]byte, error)      { return nil, nil }
func (stubDataStore) ListKeys(prefix string) ([]string, error) {
	return nil, nil
}
func (stubDataStore) DeleteRaw(key string) error { return nil }

// catalog returns every node type m9m registers, in registration
// order. The list must stay in sync with RegisterAllNodes in
// cmd/m9m/commands/exec.go — TestCatalogMatchesRegistrationList
// enforces that.
func catalog() []nodeEntry {
	eng := engine.NewWorkflowEngine()
	adapter := core.EngineAdapterFromEngine(eng, nil)

	return []nodeEntry{
		{"n8n-nodes-base.set", "transform", transform.NewSetNode()},
		{"n8n-nodes-base.filter", "transform", transform.NewFilterNode()},
		{"n8n-nodes-base.code", "transform", transform.NewCodeNode()},
		{"n8n-nodes-base.merge", "transform", transform.NewMergeNode()},
		{"n8n-nodes-base.splitInBatches", "transform", transform.NewSplitInBatchesNode()},
		{"n8n-nodes-base.itemLists", "transform", transform.NewItemListsNode()},
		{"n8n-nodes-base.switch", "transform", transform.NewSwitchNode()},
		{"n8n-nodes-base.function", "transform", transform.NewFunctionNode()},
		{"n8n-nodes-base.json", "transform", transform.NewJSONNode()},
		{"n8n-nodes-base.xml", "transform", transform.NewXmlNode()},
		{"n8n-nodes-base.httpRequest", "http", httpnode.NewHTTPRequestNode()},
		{"n8n-nodes-base.webhook", "trigger", trigger.NewWebhookNode()},
		{"n8n-nodes-base.cron", "timer", timer.NewCronNode()},
		{"n8n-nodes-base.executeCommand", "cli", cli.NewExecuteNode()},
		{"n8n-nodes-base.pythonCode", "code", code.NewPythonCodeNode()},
		{"n8n-nodes-base.readBinaryFile", "file", file.NewReadBinaryFileNode()},
		{"n8n-nodes-base.writeBinaryFile", "file", file.NewWriteBinaryFileNode()},
		{"n8n-nodes-base.postgres", "database", database.NewPostgresNode()},
		{"n8n-nodes-base.mySql", "database", database.NewMySQLNode()},
		{"n8n-nodes-base.sqlite", "database", database.NewSQLiteNode()},
		{"n8n-nodes-base.slack", "messaging", messaging.NewSlackNode()},
		{"n8n-nodes-base.discord", "messaging", messaging.NewDiscordNode()},
		{"@n8n/n8n-nodes-langchain.openAi", "ai", ai.NewOpenAINode()},
		{"@n8n/n8n-nodes-langchain.anthropic", "ai", ai.NewAnthropicNode()},
		{"n8n-nodes-base.start", "core", core.NewStartNode()},
		{"n8n-nodes-base.awsLambda", "cloud/aws", aws.NewLambdaOperationsNode()},
		{"n8n-nodes-base.s3", "cloud/aws", aws.NewS3OperationsNode()},
		{"n8n-nodes-base.azureBlobStorage", "cloud/azure", azure.NewBlobStorageNode()},
		{"n8n-nodes-base.gcpCloudStorage", "cloud/gcp", gcp.NewCloudStorageNode()},
		{"n8n-nodes-base.github", "vcs", vcs.NewGitHubNode()},
		{"n8n-nodes-base.gitlab", "vcs", vcs.NewGitLabNode()},
		{"n8n-nodes-base.googleSheets", "productivity", productivity.NewGoogleSheetsNode()},
		{"n8n-nodes-base.sendEmail", "email", email.NewSendEmailNode()},
		{"n8n-nodes-base.elasticsearch", "database", database.NewElasticsearchNode()},
		{"n8n-nodes-base.redis", "database", database.NewRedisNode()},
		{"n8n-nodes-base.mongoDb", "database", database.NewMongoDBNode()},
		{"n8n-nodes-base.noOp", "core", core.NewNoOpNode()},
		{"n8n-nodes-base.wait", "core", core.NewWaitNode()},
		{"n8n-nodes-base.dataTable", "data", data.NewDataTableNode(stubDataStore{})},
		{"n8n-nodes-base.executeWorkflow", "core", core.NewExecuteWorkflowNode(adapter, nil)},
		{"n8n-nodes-base.if", "transform", transform.NewIfNode()},
		{"n8n-nodes-base.loop", "transform", transform.NewLoopNode()},
		{"n8n-nodes-base.errorTrigger", "trigger", trigger.NewErrorTriggerNode()},
		{"n8n-nodes-base.respondToWebhook", "trigger", trigger.NewRespondToWebhookNode()},
		{"n8n-nodes-base.executeWorkflowTrigger", "trigger", trigger.NewExecuteWorkflowTriggerNode()},
		{"n8n-nodes-base.twilio", "messaging", messaging.NewTwilioNode()},
		{"n8n-nodes-base.microsoftTeams", "messaging", messaging.NewTeamsNode()},
		{"n8n-nodes-base.kafka", "messaging", messaging.NewKafkaNode()},
		{"n8n-nodes-base.kafkaTrigger", "trigger", trigger.NewKafkaTriggerNode()},
		{"n8n-nodes-base.sendGrid", "email", email.NewSendGridNode()},
		{"n8n-nodes-base.notion", "productivity", productivity.NewNotionNode()},
		{"n8n-nodes-base.stripe", "productivity", productivity.NewStripeNode()},
	}
}

// TestCatalogMatchesRegistrationList keeps the catalog in sync with
// the real registration list so the gate can never silently skip a
// node type.
func TestCatalogMatchesRegistrationList(t *testing.T) {
	registered := registeredNodeTypes(t)

	got := map[string]bool{}
	for _, entry := range catalog() {
		got[entry.TypeID] = true
	}

	for typeID := range registered {
		if !got[typeID] {
			t.Errorf("node type %q is registered by the engine but missing from the metadata catalog", typeID)
		}
	}
	for _, entry := range catalog() {
		if !registered[entry.TypeID] {
			t.Errorf("node type %q is in the metadata catalog but not registered by the engine", entry.TypeID)
		}
	}
	if len(catalog()) != expectedNodeTypes {
		t.Errorf("catalog holds %d node types, want %d", len(catalog()), expectedNodeTypes)
	}
}

// registeredNodeTypes parses the RegisterNodeExecutor calls from the
// engine registration source so the gate tracks the real list.
func registeredNodeTypes(t *testing.T) map[string]bool {
	t.Helper()

	const src = "../../../cmd/m9m/commands/exec.go"
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}

	re := regexp.MustCompile(`RegisterNodeExecutor\("([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatalf("no RegisterNodeExecutor calls found in %s", src)
	}

	types := map[string]bool{}
	for _, m := range matches {
		types[m[1]] = true
	}
	return types
}

// TestNodeTypesHaveCompleteMetadata is the core gate: every node
// type carries the metadata the parameter panel renders.
func TestNodeTypesHaveCompleteMetadata(t *testing.T) {
	for _, entry := range catalog() {
		entry := entry
		t.Run(entry.TypeID, func(t *testing.T) {
			desc := entry.Node.Description()

			if strings.TrimSpace(desc.Name) == "" {
				t.Errorf("%s: node display name is empty", entry.TypeID)
			}
			if strings.TrimSpace(desc.Description) == "" {
				t.Errorf("%s: node description is empty", entry.TypeID)
			}
			if len(desc.Properties) == 0 {
				t.Fatalf("%s: node exposes no properties", entry.TypeID)
			}

			seen := map[string]bool{}
			for _, prop := range desc.Properties {
				if strings.TrimSpace(prop.Name) == "" {
					t.Errorf("%s: a property has no name", entry.TypeID)
					continue
				}
				if seen[prop.Name] {
					t.Errorf("%s: duplicate property %q", entry.TypeID, prop.Name)
				}
				seen[prop.Name] = true

				if strings.TrimSpace(prop.DisplayName) == "" {
					t.Errorf("%s.%s: property has no display name", entry.TypeID, prop.Name)
				}
				if strings.TrimSpace(prop.Type) == "" {
					t.Errorf("%s.%s: property has no type", entry.TypeID, prop.Name)
				}
				if strings.TrimSpace(prop.Description) == "" {
					t.Errorf("%s.%s: property has no description", entry.TypeID, prop.Name)
				}
				if prop.Default == nil {
					t.Errorf("%s.%s: property has no default", entry.TypeID, prop.Name)
				}
				if prop.Type == "options" {
					if len(prop.Options) == 0 {
						t.Errorf("%s.%s: options property has no options", entry.TypeID, prop.Name)
					}
					for _, opt := range prop.Options {
						if strings.TrimSpace(opt.Name) == "" {
							t.Errorf("%s.%s: an option has no display name", entry.TypeID, prop.Name)
						}
					}
				}
			}
		})
	}
}

// TestDisplayOptionsCoverage guards the conditional-visibility gain:
// the parameter panel hides fields that do not apply to the selected
// mode/resource/operation.
func TestDisplayOptionsCoverage(t *testing.T) {
	withDisplayOptions := 0
	var missing []string

	for _, entry := range catalog() {
		desc := entry.Node.Description()
		conditional := false
		for _, prop := range desc.Properties {
			if len(prop.DisplayOptions) > 0 {
				conditional = true
				break
			}
		}
		if conditional {
			withDisplayOptions++
			continue
		}
		missing = append(missing, entry.TypeID)
	}

	sort.Strings(missing)
	t.Logf("node types with displayOptions: %d/%d", withDisplayOptions, len(catalog()))
	if len(missing) > 0 {
		t.Logf("node types without displayOptions: %s", strings.Join(missing, ", "))
	}
	if withDisplayOptions < minDisplayOptionsNodeTypes {
		t.Errorf("only %d/%d node types declare displayOptions, want at least %d",
			withDisplayOptions, len(catalog()), minDisplayOptionsNodeTypes)
	}
}

// TestDisplayOptionsAreWellFormed checks that every conditional rule
// uses the n8n shape the frontend understands (show/hide keyed by the
// sibling parameter name) and references a parameter the node
// declares. Dotted paths (workflowInputs.mappingMode) are supported by
// the frontend, so only the root segment is matched.
func TestDisplayOptionsAreWellFormed(t *testing.T) {
	for _, entry := range catalog() {
		desc := entry.Node.Description()
		names := map[string]bool{}
		for _, prop := range desc.Properties {
			names[prop.Name] = true
		}

		for _, prop := range desc.Properties {
			for _, key := range []string{"show", "hide"} {
				raw, ok := prop.DisplayOptions[key]
				if !ok {
					continue
				}
				params := displayOptionParams(raw)
				if len(params) == 0 {
					t.Errorf("%s.%s: displayOptions.%s lists no controlling parameter",
						entry.TypeID, prop.Name, key)
					continue
				}
				for param := range params {
					if names[param] {
						continue
					}
					root := param
					if idx := strings.Index(root, "."); idx > 0 {
						root = root[:idx]
					}
					if !names[root] {
						t.Errorf("%s.%s: displayOptions.%s references unknown parameter %q",
							entry.TypeID, prop.Name, key, param)
					}
				}
			}
		}
	}
}

// displayOptionParams extracts the controlling parameter names from a
// displayOptions condition value regardless of the concrete slice type
// used by the builders.
func displayOptionParams(raw interface{}) map[string]bool {
	out := map[string]bool{}
	switch cond := raw.(type) {
	case map[string]interface{}:
		for k := range cond {
			out[k] = true
		}
	case map[string][]string:
		for k := range cond {
			out[k] = true
		}
	case map[string][]bool:
		for k := range cond {
			out[k] = true
		}
	case map[string]string:
		for k := range cond {
			out[k] = true
		}
	}
	return out
}

// TestMetadataInventory prints the per-node metadata inventory used
// for the enrichment report. It never fails; it is a diagnostic.
func TestMetadataInventory(t *testing.T) {
	withProps, withDescription, withDisplayOptions := 0, 0, 0
	withNodeSpecific := 0
	explicitInputs, explicitOutputs := 0, 0
	var implicitIO []string

	for _, entry := range catalog() {
		desc := entry.Node.Description()
		conditional := 0
		controllers := map[string]bool{}
		for _, prop := range desc.Properties {
			if len(prop.DisplayOptions) == 0 {
				continue
			}
			conditional++
			for _, key := range []string{"show", "hide"} {
				raw, ok := prop.DisplayOptions[key]
				if !ok {
					continue
				}
				for param := range displayOptionParams(raw) {
					controllers[param] = true
				}
			}
		}
		if len(desc.Properties) > 0 {
			withProps++
		}
		if strings.TrimSpace(desc.Description) != "" && strings.TrimSpace(desc.Name) != "" {
			withDescription++
		}
		if conditional > 0 {
			withDisplayOptions++
		}
		nodeSpecific := false
		for name := range controllers {
			if name != "retryOnFail" {
				nodeSpecific = true
			}
		}
		if nodeSpecific {
			withNodeSpecific++
		}
		if len(desc.Inputs) > 0 {
			explicitInputs++
		}
		if len(desc.Outputs) > 0 {
			explicitOutputs++
		}
		if len(desc.Inputs) == 0 || len(desc.Outputs) == 0 {
			implicitIO = append(implicitIO, entry.TypeID)
		}

		ctrl := make([]string, 0, len(controllers))
		for name := range controllers {
			ctrl = append(ctrl, name)
		}
		sort.Strings(ctrl)

		t.Logf("%-38s pkg=%-12s props=%-3d condProps=%-3d ctrl=%-46s in=%v out=%v group=%v creds=%d tool=%v icon=%q/%q ver=%d",
			entry.TypeID, entry.Pkg, len(desc.Properties), conditional, strings.Join(ctrl, ","),
			desc.Inputs, desc.Outputs, desc.Group, len(desc.Credentials), desc.UsableAsTool,
			desc.Icon, desc.IconColor, desc.Version)
	}

	sort.Strings(implicitIO)
	t.Logf("TOTALS nodes=%d withProperties=%d withDescriptionAndDisplayName=%d withDisplayOptions=%d nodeSpecificDisplayOptions=%d explicitInputs=%d explicitOutputs=%d",
		len(catalog()), withProps, withDescription, withDisplayOptions, withNodeSpecific, explicitInputs, explicitOutputs)
	t.Logf("nodes relying on the engine default for inputs/outputs: %s", strings.Join(implicitIO, ", "))
}
