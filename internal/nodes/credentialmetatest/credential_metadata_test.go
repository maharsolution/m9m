// Package credentialmetatest holds the gate test for m9m's per-node
// credential metadata.
//
// The editor renders one credential picker per slot declared in a node
// description (`base.NodeDescription.Credentials`). A slot is only
// useful when
//
//   - the credential TYPE exists in m9m's credential schema registry
//     (the keys of `credentialSchemas` in internal/api/credentials.go),
//     because the picker creates the credential through
//     `GET /api/v1/credentials/types/{type}`; and
//   - the node really consumes that credential.
//
// The test therefore pins three things:
//
//  1. every registered node type declares a non-empty `Group`;
//  2. every declared credential type is supported by the schema registry
//     and carries the registry's display name;
//  3. every node type that consumes credentials declares exactly the
//     credential types it consumes.
//
// The registry is asserted against a LITERAL list (see
// `supportedCredentialTypes`) rather than importing internal/api: the
// `credentialSchemas` map is package-private and internal/api is owned by
// another worker whose in-flight edits may not compile. The literal was
// verified against internal/api/credentials.go on 2026-11-11.
package credentialmetatest

import (
	"testing"

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

// supportedCredentialTypes is the credential schema registry of m9m,
// mirrored from the keys and display names of `credentialSchemas` in
// internal/api/credentials.go (13 base types + the `mySql` / `openAiApi`
// aliases + `rabbitmq` + `redis`).
//
// A node may only declare a type that appears here — the editor's
// credential picker resolves the type against this registry, and a type
// that is missing from it renders an empty picker.
var supportedCredentialTypes = map[string]string{
	"httpBasicAuth":          "Basic Auth",
	"httpHeaderAuth":         "Header Auth",
	"oAuth2Api":              "OAuth2 API",
	"apiKey":                 "API Key",
	"postgres":               "Postgres",
	"mysql":                  "MySQL",
	"mySql":                  "MySQL",
	"mongoDb":                "MongoDB",
	"kafka":                  "Kafka",
	"slack":                  "Slack",
	"discord":                "Discord",
	"openai":                 "OpenAI",
	"openAiApi":              "OpenAI",
	"anthropic":              "Anthropic",
	"jwtAuth":                "JWT Auth",
	"rabbitmq":               "RabbitMQ",
	"redis":                  "Redis",
	"githubApi":              "GitHub API",
	"gitlabApi":              "GitLab API",
	"googleSheetsOAuth2Api":  "Google Sheets OAuth2 API",
	"aws":                    "AWS",
	"azureStorage":           "Azure Storage",
	"googleCloudStorage":     "Google Cloud Storage",
}

// nodeType couples an n8n node type id with the description m9m
// registers for it.
type nodeType struct {
	id          string
	description base.NodeDescription
}

// registeredNodeTypes returns the descriptions of every node type m9m
// registers in cmd/m9m/commands/exec.go (`RegisterAllNodes`).
//
// The registration map lives in package main-ish command code that pulls
// in the whole engine, so the table is mirrored here instead of
// imported. Every entry maps 1:1 onto a `RegisterNodeExecutor` call.
func registeredNodeTypes() []nodeType {
	return []nodeType{
		// Transform nodes
		{"n8n-nodes-base.set", transform.NewSetNode().Description()},
		{"n8n-nodes-base.filter", transform.NewFilterNode().Description()},
		{"n8n-nodes-base.code", transform.NewCodeNode().Description()},
		{"n8n-nodes-base.merge", transform.NewMergeNode().Description()},
		{"n8n-nodes-base.splitInBatches", transform.NewSplitInBatchesNode().Description()},
		{"n8n-nodes-base.itemLists", transform.NewItemListsNode().Description()},
		{"n8n-nodes-base.switch", transform.NewSwitchNode().Description()},
		{"n8n-nodes-base.function", transform.NewFunctionNode().Description()},
		{"n8n-nodes-base.json", transform.NewJSONNode().Description()},
		{"n8n-nodes-base.xml", transform.NewXmlNode().Description()},
		{"n8n-nodes-base.if", transform.NewIfNode().Description()},
		{"n8n-nodes-base.loop", transform.NewLoopNode().Description()},

		// HTTP node
		{"n8n-nodes-base.httpRequest", httpnode.NewHTTPRequestNode().Description()},

		// Trigger nodes
		{"n8n-nodes-base.webhook", trigger.NewWebhookNode().Description()},
		{"n8n-nodes-base.errorTrigger", trigger.NewErrorTriggerNode().Description()},
		{"n8n-nodes-base.respondToWebhook", trigger.NewRespondToWebhookNode().Description()},
		{"n8n-nodes-base.executeWorkflowTrigger", trigger.NewExecuteWorkflowTriggerNode().Description()},
		{"n8n-nodes-base.kafkaTrigger", trigger.NewKafkaTriggerNode().Description()},

		// Timer / schedule node
		{"n8n-nodes-base.cron", timer.NewCronNode().Description()},

		// Code execution nodes
		{"n8n-nodes-base.executeCommand", cli.NewExecuteNode().Description()},
		{"n8n-nodes-base.pythonCode", code.NewPythonCodeNode().Description()},

		// File nodes
		{"n8n-nodes-base.readBinaryFile", file.NewReadBinaryFileNode().Description()},
		{"n8n-nodes-base.writeBinaryFile", file.NewWriteBinaryFileNode().Description()},

		// Database nodes
		{"n8n-nodes-base.postgres", database.NewPostgresNode().Description()},
		{"n8n-nodes-base.mySql", database.NewMySQLNode().Description()},
		{"n8n-nodes-base.sqlite", database.NewSQLiteNode().Description()},
		{"n8n-nodes-base.elasticsearch", database.NewElasticsearchNode().Description()},
		{"n8n-nodes-base.redis", database.NewRedisNode().Description()},
		{"n8n-nodes-base.mongoDb", database.NewMongoDBNode().Description()},

		// Messaging nodes
		{"n8n-nodes-base.slack", messaging.NewSlackNode().Description()},
		{"n8n-nodes-base.discord", messaging.NewDiscordNode().Description()},
		{"n8n-nodes-base.twilio", messaging.NewTwilioNode().Description()},
		{"n8n-nodes-base.microsoftTeams", messaging.NewTeamsNode().Description()},
		{"n8n-nodes-base.kafka", messaging.NewKafkaNode().Description()},

		// AI nodes
		{"@n8n/n8n-nodes-langchain.openAi", ai.NewOpenAINode().Description()},
		{"@n8n/n8n-nodes-langchain.anthropic", ai.NewAnthropicNode().Description()},

		// Core nodes
		{"n8n-nodes-base.start", core.NewStartNode().Description()},
		{"n8n-nodes-base.noOp", core.NewNoOpNode().Description()},
		{"n8n-nodes-base.wait", core.NewWaitNode().Description()},
		{"n8n-nodes-base.executeWorkflow", core.NewExecuteWorkflowNode(nil, nil).Description()},

		// Data node (registered when the storage backend is a full store)
		{"n8n-nodes-base.dataTable", data.NewDataTableNode(nil).Description()},

		// Cloud nodes
		{"n8n-nodes-base.awsLambda", aws.NewLambdaOperationsNode().Description()},
		{"n8n-nodes-base.s3", aws.NewS3OperationsNode().Description()},
		{"n8n-nodes-base.azureBlobStorage", azure.NewBlobStorageNode().Description()},
		{"n8n-nodes-base.gcpCloudStorage", gcp.NewCloudStorageNode().Description()},

		// VCS nodes
		{"n8n-nodes-base.github", vcs.NewGitHubNode().Description()},
		{"n8n-nodes-base.gitlab", vcs.NewGitLabNode().Description()},

		// Productivity nodes
		{"n8n-nodes-base.googleSheets", productivity.NewGoogleSheetsNode().Description()},
		{"n8n-nodes-base.notion", productivity.NewNotionNode().Description()},
		{"n8n-nodes-base.stripe", productivity.NewStripeNode().Description()},

		// Email nodes
		{"n8n-nodes-base.sendEmail", email.NewSendEmailNode().Description()},
		{"n8n-nodes-base.sendGrid", email.NewSendGridNode().Description()},
	}
}

// credentialExpectations pins the credential slots of every node type
// that consumes a credential.
//
// The credential type string is the key the credential is stored under in
// the workflow's `node.credentials` map — the same key the credential
// manager uses to flatten the envelope into `<credType>_<field>` node
// parameters (internal/credentials/manager.go) and the same key the
// editor's picker writes back.
//
// `Required` follows m9m's own runtime rule: true when the node cannot
// work without the credential (its connection settings live entirely in
// the credential envelope), false when the credential is one of several
// ways to authenticate (n8n marks the same slots `required: true`).
var credentialExpectations = map[string][]base.NodeCredentialSpec{
	// HTTP Request accepts three alternative auth schemes; the node
	// reads `httpBasicAuth_user` / `httpBasicAuth_password`,
	// `httpHeaderAuth_name` / `httpHeaderAuth_value` and `apiKey_apiKey`.
	"n8n-nodes-base.httpRequest": {
		{Type: "httpBasicAuth", DisplayName: "Basic Auth", Required: false},
		{Type: "httpHeaderAuth", DisplayName: "Header Auth", Required: false},
		{Type: "apiKey", DisplayName: "API Key", Required: false},
	},
	// The webhook auth handler resolves `httpBasicAuth`,
	// `httpHeaderAuth` and `jwtAuth` straight from node.Credentials
	// (internal/webhooks/manager.go). They are optional: the node's
	// `authentication` parameter defaults to "none".
	"n8n-nodes-base.webhook": {
		{Type: "httpBasicAuth", DisplayName: "Basic Auth", Required: false},
		{Type: "httpHeaderAuth", DisplayName: "Header Auth", Required: false},
		{Type: "jwtAuth", DisplayName: "JWT Auth", Required: false},
	},
	// Database nodes read the flattened credential envelope:
	// `postgres_host`, `mySql_host`, `mongoDb_host`, …
	"n8n-nodes-base.postgres": {
		{Type: "postgres", DisplayName: "Postgres", Required: true},
	},
	"n8n-nodes-base.mySql": {
		{Type: "mySql", DisplayName: "MySQL", Required: true},
	},
	"n8n-nodes-base.mongoDb": {
		{Type: "mongoDb", DisplayName: "MongoDB", Required: true},
	},
	"n8n-nodes-base.redis": {
		{Type: "redis", DisplayName: "Redis", Required: true},
	},
	// Kafka nodes fold the `kafka_*` envelope keys back into their
	// explicit params (messaging.ApplyKafkaCredentialAuth).
	"n8n-nodes-base.kafka": {
		{Type: "kafka", DisplayName: "Kafka", Required: true},
	},
	"n8n-nodes-base.kafkaTrigger": {
		{Type: "kafka", DisplayName: "Kafka", Required: true},
	},
	// Messaging nodes authenticate with the credential that m9m's
	// registry defines for them. Slack also accepts a plain `webhookUrl`
	// parameter, so its credential is optional.
	"n8n-nodes-base.slack": {
		{Type: "slack", DisplayName: "Slack", Required: false},
	},
	"n8n-nodes-base.discord": {
		{Type: "discord", DisplayName: "Discord", Required: true},
	},
	// AI nodes take their API key from the credential envelope.
	"@n8n/n8n-nodes-langchain.openAi": {
		{Type: "openai", DisplayName: "OpenAI", Required: true},
	},
	"@n8n/n8n-nodes-langchain.anthropic": {
		{Type: "anthropic", DisplayName: "Anthropic", Required: true},
	},
	// VCS nodes
	"n8n-nodes-base.github": {
		{Type: "githubApi", DisplayName: "GitHub API", Required: true},
	},
	"n8n-nodes-base.gitlab": {
		{Type: "gitlabApi", DisplayName: "GitLab API", Required: true},
	},
	// Productivity
	"n8n-nodes-base.googleSheets": {
		{Type: "googleSheetsOAuth2Api", DisplayName: "Google Sheets OAuth2 API", Required: true},
	},
	// Cloud nodes
	"n8n-nodes-base.awsLambda": {
		{Type: "aws", DisplayName: "AWS", Required: true},
	},
	"n8n-nodes-base.s3": {
		{Type: "aws", DisplayName: "AWS", Required: true},
	},
	"n8n-nodes-base.azureBlobStorage": {
		{Type: "azureStorage", DisplayName: "Azure Storage", Required: false},
	},
	"n8n-nodes-base.gcpCloudStorage": {
		{Type: "googleCloudStorage", DisplayName: "Google Cloud Storage", Required: false},
	},
}

// TestEveryRegisteredNodeTypeHasGroup pins that every registered node
// type declares a non-empty Group so the node creator can bucket it.
func TestEveryRegisteredNodeTypeHasGroup(t *testing.T) {
	for _, nt := range registeredNodeTypes() {
		if len(nt.description.Group) == 0 {
			t.Errorf("node type %q declares no Group", nt.id)
			continue
		}
		for _, group := range nt.description.Group {
			if group == "" {
				t.Errorf("node type %q declares an empty Group entry", nt.id)
			}
		}
	}
}

// TestDeclaredCredentialTypesAreSupported pins that no node declares a
// credential type m9m cannot create, and that the display name matches
// the one the credential API uses.
func TestDeclaredCredentialTypesAreSupported(t *testing.T) {
	for _, nt := range registeredNodeTypes() {
		seen := map[string]bool{}
		for _, slot := range nt.description.Credentials {
			if slot.Type == "" {
				t.Errorf("node type %q declares a credential slot without a type", nt.id)
				continue
			}
			if seen[slot.Type] {
				t.Errorf("node type %q declares credential type %q twice", nt.id, slot.Type)
			}
			seen[slot.Type] = true

			displayName, ok := supportedCredentialTypes[slot.Type]
			if !ok {
				t.Errorf("node type %q declares unsupported credential type %q", nt.id, slot.Type)
				continue
			}
			if slot.DisplayName != displayName {
				t.Errorf("node type %q declares credential %q with display name %q, want %q",
					nt.id, slot.Type, slot.DisplayName, displayName)
			}
		}
	}
}

// TestCredentialNodesDeclareTheirCredentials pins the credential slots of
// every node type that consumes a credential.
func TestCredentialNodesDeclareTheirCredentials(t *testing.T) {
	registered := map[string]base.NodeDescription{}
	for _, nt := range registeredNodeTypes() {
		registered[nt.id] = nt.description
	}

	for id, expected := range credentialExpectations {
		description, ok := registered[id]
		if !ok {
			t.Errorf("node type %q is not registered", id)
			continue
		}
		assertCredentialSlots(t, id, expected, description.Credentials)
	}
}

// TestOnlyCredentialNodesDeclareCredentialSlots pins the inverse of
// TestCredentialNodesDeclareTheirCredentials: a node type that is not
// listed in credentialExpectations must declare NO credential slot at
// all.
//
// This is the guard against a "plausible but wrong" declaration — e.g.
// attaching the `postgres` credential to the `set` node, or the `slack`
// credential to `twilio`. The type would exist in the schema registry
// (so TestDeclaredCredentialTypesAreSupported would not catch it), but
// the editor would then offer a picker for a credential the node never
// reads.
func TestOnlyCredentialNodesDeclareCredentialSlots(t *testing.T) {
	for _, nt := range registeredNodeTypes() {
		if _, expected := credentialExpectations[nt.id]; expected {
			continue
		}
		if len(nt.description.Credentials) != 0 {
			t.Errorf("node type %q declares %d credential slot(s) but is not a credential node",
				nt.id, len(nt.description.Credentials))
		}
	}
}

// TestNoCredentialSlotsOnCredentialFreeNodes pins that node types which
// carry no authentication material do not offer a credential picker.
func TestNoCredentialSlotsOnCredentialFreeNodes(t *testing.T) {
	credentialFree := []string{
		// Pure data-shaping / control-flow nodes.
		"n8n-nodes-base.set",
		"n8n-nodes-base.filter",
		"n8n-nodes-base.code",
		"n8n-nodes-base.merge",
		"n8n-nodes-base.splitInBatches",
		"n8n-nodes-base.itemLists",
		"n8n-nodes-base.switch",
		"n8n-nodes-base.function",
		"n8n-nodes-base.json",
		"n8n-nodes-base.xml",
		"n8n-nodes-base.if",
		"n8n-nodes-base.loop",
		"n8n-nodes-base.executeCommand",
		"n8n-nodes-base.pythonCode",
		"n8n-nodes-base.readBinaryFile",
		"n8n-nodes-base.writeBinaryFile",
		"n8n-nodes-base.cron",
		"n8n-nodes-base.start",
		"n8n-nodes-base.noOp",
		"n8n-nodes-base.wait",
		"n8n-nodes-base.executeWorkflow",
		"n8n-nodes-base.executeWorkflowTrigger",
		"n8n-nodes-base.dataTable",
		"n8n-nodes-base.errorTrigger",
		"n8n-nodes-base.respondToWebhook",
		// SQLite is a local file database — no server, no auth.
		"n8n-nodes-base.sqlite",
	}

	for _, nt := range registeredNodeTypes() {
		if !contains(credentialFree, nt.id) {
			continue
		}
		if len(nt.description.Credentials) != 0 {
			t.Errorf("credential-free node type %q declares %d credential slot(s)", nt.id, len(nt.description.Credentials))
		}
	}
}

// assertCredentialSlots compares the declared slots of a node type with
// the expected ones. The comparison is order-independent but exact:
// a missing slot, an extra slot or a wrong required flag fails.
func assertCredentialSlots(t *testing.T, id string, expected, actual []base.NodeCredentialSpec) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("node type %q declares %d credential slot(s), want %d", id, len(actual), len(expected))
		return
	}

	for _, want := range expected {
		found := false
		for _, got := range actual {
			if got.Type != want.Type {
				continue
			}
			found = true
			if got.DisplayName != want.DisplayName {
				t.Errorf("node type %q credential %q has display name %q, want %q",
					id, want.Type, got.DisplayName, want.DisplayName)
			}
			if got.Required != want.Required {
				t.Errorf("node type %q credential %q has required=%v, want %v",
					id, want.Type, got.Required, want.Required)
			}
		}
		if !found {
			t.Errorf("node type %q does not declare credential %q", id, want.Type)
		}
	}
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
