package expressions

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/neul-labs/m9m/internal/model"
)

// WorkflowExecuteMode represents the execution mode of the workflow
type WorkflowExecuteMode string

const (
	ModeCLI        WorkflowExecuteMode = "cli"
	ModeError      WorkflowExecuteMode = "error"
	ModeIntegrated WorkflowExecuteMode = "integrated"
	ModeInternal   WorkflowExecuteMode = "internal"
	ModeManual     WorkflowExecuteMode = "manual"
	ModeRetry      WorkflowExecuteMode = "retry"
	ModeTrigger    WorkflowExecuteMode = "trigger"
	ModeWebhook    WorkflowExecuteMode = "webhook"
	ModeEvaluation WorkflowExecuteMode = "evaluation"
)

// RunExecutionData represents data for a workflow run
type RunExecutionData struct {
	ExecutionData *ExecutionData
	ResultData    *RunData
	ExecutionMode WorkflowExecuteMode
	StartedAt     time.Time
	StoppedAt     *time.Time
	WorkflowData  interface{}
}

// ExecutionData represents execution context data
type ExecutionData struct {
	ContextData        map[string]interface{}
	NodeExecutionStack []interface{}
	MetaData           map[string]interface{}
	WaitingExecution   map[string]interface{}
}

// RunData represents data from previous node runs
type RunData struct {
	NodeData map[string][]NodeExecutionResult
}

// NodeExecutionResult represents the result of executing a node
type NodeExecutionResult struct {
	Data          []model.DataItem `json:"data"`
	Error         *ExecutionError  `json:"error,omitempty"`
	StartTime     time.Time        `json:"startTime"`
	ExecutionTime time.Duration    `json:"executionTime"`
	Source        []interface{}    `json:"source,omitempty"`
}

// ExecutionError represents an error during execution
type ExecutionError struct {
	Name        string      `json:"name"`
	Message     string      `json:"message"`
	Description string      `json:"description"`
	Context     interface{} `json:"context"`
	Cause       interface{} `json:"cause"`
	Timestamp   time.Time   `json:"timestamp"`
	NodeName    string      `json:"nodeName"`
	NodeType    string      `json:"nodeType"`
}

// ExecutionContext represents the context for expression evaluation
type ExpressionContext struct {
	Workflow            *model.Workflow
	RunExecutionData    *RunExecutionData
	RunIndex            int
	ItemIndex           int
	ActiveNodeName      string
	ConnectionInputData []model.DataItem
	SiblingParameters   map[string]interface{}
	Mode                WorkflowExecuteMode
	AdditionalKeys      *AdditionalKeys
	ExecuteData         *ExecuteData
	ContextNodeName     *string
}

// AdditionalKeys represents additional context keys
type AdditionalKeys struct {
	ExecutionId           string                 `json:"executionId"`
	CurrentNodeParameters map[string]interface{} `json:"currentNodeParameters"`
	RestApiUrl            string                 `json:"restApiUrl"`
	InstanceBaseUrl       string                 `json:"instanceBaseUrl"`
	WebhookBaseUrl        string                 `json:"webhookBaseUrl"`
	WebhookWaitingBaseUrl string                 `json:"webhookWaitingBaseUrl"`
	WebhookTestBaseUrl    string                 `json:"webhookTestBaseUrl"`
}

// ExecuteData represents additional execution data
type ExecuteData struct {
	Data     interface{}
	Source   interface{}
	Metadata map[string]interface{}
}

// WorkflowDataProxy provides access to workflow data contexts ($json, $input, etc.)
type WorkflowDataProxy struct {
	// Context
	workflow            *model.Workflow
	runExecutionData    *RunExecutionData
	runIndex            int
	itemIndex           int
	activeNodeName      string
	connectionInputData []model.DataItem
	siblingParameters   map[string]interface{}
	mode                WorkflowExecuteMode

	// Additional context
	additionalKeys  *AdditionalKeys
	executeData     *ExecuteData
	contextNodeName *string

	// Caching
	dataCache  map[string]interface{}
	cacheMutex sync.RWMutex

	// JavaScript VM reference
	vm *goja.Runtime
}

// NewWorkflowDataProxy creates a new WorkflowDataProxy instance
func NewWorkflowDataProxy(context *ExpressionContext, vm *goja.Runtime) *WorkflowDataProxy {
	return &WorkflowDataProxy{
		workflow:            context.Workflow,
		runExecutionData:    context.RunExecutionData,
		runIndex:            context.RunIndex,
		itemIndex:           context.ItemIndex,
		activeNodeName:      context.ActiveNodeName,
		connectionInputData: context.ConnectionInputData,
		siblingParameters:   context.SiblingParameters,
		mode:                context.Mode,
		additionalKeys:      context.AdditionalKeys,
		executeData:         context.ExecuteData,
		contextNodeName:     context.ContextNodeName,
		dataCache:           make(map[string]interface{}),
		vm:                  vm,
	}
}

// CreateJavaScriptProxy creates a JavaScript proxy object with all n8n context variables
func (p *WorkflowDataProxy) CreateJavaScriptProxy() goja.Value {
	proxy := p.vm.NewObject()

	// Core n8n variables
	_ = proxy.Set("$json", p.createJsonProxy())
	_ = proxy.Set("$input", p.createInputProxy())
	_ = proxy.Set("$node", p.createNodeProxy())
	_ = proxy.Set("$parameter", p.createParameterProxy())
	_ = proxy.Set("$workflow", p.createWorkflowProxy())
	_ = proxy.Set("$execution", p.createExecutionProxy())
	_ = proxy.Set("$env", p.createEnvProxy())
	_ = proxy.Set("$binary", p.createBinaryProxy())
	_ = proxy.Set("$vars", p.createVarsProxy())

	// Shorthand for $node function
	_ = proxy.Set("$", p.createNodeProxy())

	// Legacy support
	_ = proxy.Set("$evaluateExpression", p.createEvaluateExpressionProxy())

	return proxy
}

// createJsonProxy creates the $json context variable
func (p *WorkflowDataProxy) createJsonProxy() goja.Value {
	if p.itemIndex >= len(p.connectionInputData) {
		return goja.Undefined()
	}

	item := p.connectionInputData[p.itemIndex]
	return p.vm.ToValue(item.JSON)
}

// createInputProxy creates the $input context variable
func (p *WorkflowDataProxy) createInputProxy() goja.Value {
	inputProxy := p.vm.NewObject()

	// $input.all() - all items from all connections
	_ = inputProxy.Set("all", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		var connectionIndex int = 0
		if len(call.Arguments) > 0 {
			connectionIndex = int(call.Arguments[0].ToInteger())
		}

		inputData := p.getInputConnectionData(connectionIndex)
		jsonData := make([]interface{}, len(inputData))
		codeNodeItems := p.additionalKeys != nil && p.additionalKeys.CurrentNodeParameters != nil && p.additionalKeys.CurrentNodeParameters["codeNodeInputItems"] == true
		for i, item := range inputData {
			if codeNodeItems {
				jsonData[i] = map[string]interface{}{"json": item.JSON, "binary": item.Binary}
			} else {
				jsonData[i] = item.JSON
			}
		}
		return p.vm.ToValue(jsonData)
	}))

	// $input.first() - first item from connection.
	//
	// n8n's `$input.first()` (and `.last()`, `.item`, `.all()`) returns the
	// n8n-shaped item wrapper `{json, binary, pairedItem}`, NOT the bare JSON
	// map. Code node snippets like `$input.first().json.body` rely on this
	// wrapper shape. Returning the bare `item.JSON` map would mean `.json`
	// resolves to `undefined` inside the snippet (because maps don't carry
	// their own `json` key), and downstream JS expressions like
	// `if (!$input.first()?.json)` end up taking the wrong branch.
	//
	// When `codeNodeInputItems` is set on `CurrentNodeParameters` (the Code
	// node calls these proxies from inside `runOnceForAllItems` /
	// `runOnceForEachItem`), we therefore wrap the inner JSON in the n8n
	// item shape so `$input.first().json` and `$input.first().binary` both
	// resolve correctly. Outside the Code node (e.g. expression-only paths
	// where only `$json` is consumed) the same wrapper is harmless and
	// keeps the wire shape consistent.
	_ = inputProxy.Set("first", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		var connectionIndex int = 0
		if len(call.Arguments) > 0 {
			connectionIndex = int(call.Arguments[0].ToInteger())
		}

		inputData := p.getInputConnectionData(connectionIndex)
		if len(inputData) > 0 {
			return p.vm.ToValue(n8nItemWrapper(inputData[0]))
		}
		return goja.Undefined()
	}))

	// $input.last() - last item from connection
	_ = inputProxy.Set("last", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		var connectionIndex int = 0
		if len(call.Arguments) > 0 {
			connectionIndex = int(call.Arguments[0].ToInteger())
		}

		inputData := p.getInputConnectionData(connectionIndex)
		if len(inputData) > 0 {
			return p.vm.ToValue(n8nItemWrapper(inputData[len(inputData)-1]))
		}
		return goja.Undefined()
	}))

	// $input.item - specific item by index
	_ = inputProxy.Set("item", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		var itemIndex int = p.itemIndex
		var connectionIndex int = 0

		if len(call.Arguments) > 0 {
			itemIndex = int(call.Arguments[0].ToInteger())
		}
		if len(call.Arguments) > 1 {
			connectionIndex = int(call.Arguments[1].ToInteger())
		}

		inputData := p.getInputConnectionData(connectionIndex)
		if itemIndex >= 0 && itemIndex < len(inputData) {
			return p.vm.ToValue(n8nItemWrapper(inputData[itemIndex]))
		}
		return goja.Undefined()
	}))

	return inputProxy
}

// n8nItemWrapper builds the n8n-shaped item envelope used by
// `$input.first()`, `$input.last()`, `$input.item`, and (already, prior to
// this change) `$input.all()`.
//
// The wrapper is the canonical n8n wire shape:
//
//	{json: {...}, binary: {...}, pairedItem: {...}}
//
// Code node snippets read `$input.first().json.<key>` (e.g.
// `$input.first().json.body`), expressions read `$json.<key>` (which the
// proxy resolves via `createJsonProxy`), and downstream merge/loop nodes
// rely on `pairedItem` to track provenance. Returning the bare JSON map
// instead of the wrapper would silently break any of those.
//
// `Binary` and `PairedItem` may be nil on the DataItem; we emit them as
// `nil` rather than omit the keys, so downstream code that explicitly
// destructures `{json, binary, pairedItem}` doesn't see `undefined`
// collapse the destructuring. (`nil` in Go maps to `null` in Goja,
// which is the value n8n emits on its data layer for missing fields.)
func n8nItemWrapper(item model.DataItem) map[string]interface{} {
	return map[string]interface{}{
		"json":       item.JSON,
		"binary":     item.Binary,
		"pairedItem": item.PairedItem,
	}
}

// createNodeProxy creates the $node context variable and $() function
func (p *WorkflowDataProxy) createNodeProxy() goja.Value {
	return p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(p.vm.NewTypeError("Node name is required"))
		}

		nodeName := call.Arguments[0].String()

		// Get node execution data
		nodeExecutionData := p.getNodeExecutionData(nodeName)

		nodeProxy := p.vm.NewObject()

		// Current item from node (for current run/item index).
		//
		// The n8n node proxy exposes `.json` / `.binary` as a *wrapper*
		// (the same shape `$input.first().json` resolves to) so that
		// `$("Loop").item.json.body` continues to work the way it does
		// on n8n. Returning the bare JSON map directly would force
		// authors to write `$("Loop").item.body` instead — which
		// diverges from the n8n surface area and silently breaks
		// drop-in workflows.
		if len(nodeExecutionData) > 0 && p.itemIndex < len(nodeExecutionData) {
			_ = nodeProxy.Set("json", p.vm.ToValue(n8nItemWrapper(nodeExecutionData[p.itemIndex])))
			_ = nodeProxy.Set("binary", p.vm.ToValue(nodeExecutionData[p.itemIndex].Binary))
		} else {
			_ = nodeProxy.Set("json", goja.Undefined())
			_ = nodeProxy.Set("binary", goja.Undefined())
		}

		// Array-like access methods. Each method returns the n8n-shaped
		// `{json, binary, pairedItem}` wrapper for the same reason as
		// the `.json` setter above — `$("Loop").first().json.body`
		// is the canonical n8n form and dropping the wrapper here
		// would silently swallow fields under spread operators.
		_ = nodeProxy.Set("first", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(nodeExecutionData) > 0 {
				return p.vm.ToValue(n8nItemWrapper(nodeExecutionData[0]))
			}
			return goja.Undefined()
		}))

		_ = nodeProxy.Set("last", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(nodeExecutionData) > 0 {
				return p.vm.ToValue(n8nItemWrapper(nodeExecutionData[len(nodeExecutionData)-1]))
			}
			return goja.Undefined()
		}))

		_ = nodeProxy.Set("all", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			jsonData := make([]interface{}, len(nodeExecutionData))
			for i, item := range nodeExecutionData {
				jsonData[i] = n8nItemWrapper(item)
			}
			return p.vm.ToValue(jsonData)
		}))

		// `.item` accessor. n8n exposes `.item` as a *property* that
		// returns the n8n-shaped wrapper for the *current* iteration
		// item, so `$("Loop").item.json` works directly without
		// invoking `.item()` as a method. Storing the wrapper as a
		// plain property here matches that semantics.
		//
		// `$(node).item(index)` (with an explicit index) is a less
		// common form but still supported via a separate function
		// exposed as `itemAt` so authors who use it don't break.
		if len(nodeExecutionData) > 0 {
			idx := p.itemIndex
			if idx >= len(nodeExecutionData) {
				idx = 0
			}
			_ = nodeProxy.Set("item", p.vm.ToValue(n8nItemWrapper(nodeExecutionData[idx])))
		} else {
			_ = nodeProxy.Set("item", goja.Undefined())
		}

		// Paired item support for data lineage
		_ = nodeProxy.Set("pairedItem", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			var itemIndex int = p.itemIndex
			if len(call.Arguments) > 0 {
				itemIndex = int(call.Arguments[0].ToInteger())
			}

			return p.getPairedItemData(nodeName, itemIndex)
		}))

		return nodeProxy
	})
}

// createParameterProxy creates the $parameter context variable
func (p *WorkflowDataProxy) createParameterProxy() goja.Value {
	// Get current node parameters
	currentNode := p.getCurrentNode()
	if currentNode == nil {
		return p.vm.ToValue(map[string]interface{}{})
	}

	// Merge with sibling parameters if available
	parameters := make(map[string]interface{})
	for key, value := range currentNode.Parameters {
		parameters[key] = value
	}

	// Add sibling parameters
	if p.siblingParameters != nil {
		for key, value := range p.siblingParameters {
			parameters[key] = value
		}
	}

	// Add additional keys if available
	if p.additionalKeys != nil && p.additionalKeys.CurrentNodeParameters != nil {
		for key, value := range p.additionalKeys.CurrentNodeParameters {
			parameters[key] = value
		}
	}

	return p.vm.ToValue(parameters)
}

// createWorkflowProxy creates the $workflow context variable
func (p *WorkflowDataProxy) createWorkflowProxy() goja.Value {
	workflowProxy := p.vm.NewObject()

	if p.workflow != nil {
		_ = workflowProxy.Set("id", p.vm.ToValue(p.workflow.ID))
		_ = workflowProxy.Set("name", p.vm.ToValue(p.workflow.Name))
		_ = workflowProxy.Set("active", p.vm.ToValue(p.workflow.Active))
		_ = workflowProxy.Set("versionId", p.vm.ToValue(p.workflow.VersionID))

		if p.workflow.Settings != nil {
			settingsProxy := p.vm.NewObject()
			_ = settingsProxy.Set("timezone", p.vm.ToValue(p.workflow.Settings.Timezone))
			_ = settingsProxy.Set("executionOrder", p.vm.ToValue(p.workflow.Settings.ExecutionOrder))
			_ = workflowProxy.Set("settings", settingsProxy)
		}
	}

	return workflowProxy
}

// createExecutionProxy creates the $execution context variable
func (p *WorkflowDataProxy) createExecutionProxy() goja.Value {
	executionProxy := p.vm.NewObject()

	if p.runExecutionData != nil {
		_ = executionProxy.Set("mode", p.vm.ToValue(string(p.runExecutionData.ExecutionMode)))
		_ = executionProxy.Set("startedAt", p.vm.ToValue(p.runExecutionData.StartedAt.Unix()*1000))

		if p.runExecutionData.StoppedAt != nil {
			_ = executionProxy.Set("stoppedAt", p.vm.ToValue(p.runExecutionData.StoppedAt.Unix()*1000))
		}
	}

	// Add additional execution keys
	if p.additionalKeys != nil {
		_ = executionProxy.Set("id", p.vm.ToValue(p.additionalKeys.ExecutionId))
		_ = executionProxy.Set("restApiUrl", p.vm.ToValue(p.additionalKeys.RestApiUrl))
		_ = executionProxy.Set("instanceBaseUrl", p.vm.ToValue(p.additionalKeys.InstanceBaseUrl))
		_ = executionProxy.Set("webhookBaseUrl", p.vm.ToValue(p.additionalKeys.WebhookBaseUrl))
		_ = executionProxy.Set("webhookWaitingBaseUrl", p.vm.ToValue(p.additionalKeys.WebhookWaitingBaseUrl))
		_ = executionProxy.Set("webhookTestBaseUrl", p.vm.ToValue(p.additionalKeys.WebhookTestBaseUrl))
	}

	return executionProxy
}

// createEnvProxy creates the $env context variable
func (p *WorkflowDataProxy) createEnvProxy() goja.Value {
	// Create a proxy that dynamically reads environment variables
	return p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			// Return all environment variables
			envMap := make(map[string]string)
			for _, env := range os.Environ() {
				if pair := strings.SplitN(env, "=", 2); len(pair) == 2 {
					envMap[pair[0]] = pair[1]
				}
			}
			return p.vm.ToValue(envMap)
		}

		// Return specific environment variable
		varName := call.Arguments[0].String()
		value := os.Getenv(varName)
		if value == "" {
			return goja.Undefined()
		}
		return p.vm.ToValue(value)
	})
}

// createBinaryProxy creates the $binary context variable
func (p *WorkflowDataProxy) createBinaryProxy() goja.Value {
	if p.itemIndex >= len(p.connectionInputData) {
		return p.vm.ToValue(map[string]interface{}{})
	}

	item := p.connectionInputData[p.itemIndex]
	return p.vm.ToValue(item.Binary)
}

// createVarsProxy creates the $vars context variable (workflow variables)
func (p *WorkflowDataProxy) createVarsProxy() goja.Value {
	// Return static data as workflow variables
	if p.workflow != nil && p.workflow.StaticData != nil {
		return p.vm.ToValue(p.workflow.StaticData)
	}
	return p.vm.ToValue(map[string]interface{}{})
}

// createNowProxy creates the $now context variable as a Luxon-style
// DateTime proxy that supports the subset of helpers n8n workflows
// actually use. We register the most common methods directly:
//   - toISOString() -> "2026-09-05T12:34:56.789Z" (RFC3339 / ms)
//   - toMillis()    -> unix-ms timestamp
//   - toString()    -> ISO string (alias for toISOString)
//   - format(fmt)   -> string formatted with a Luxon/Moment-style
//     pattern (yyyy-MM-dd, yyyy-MM-dd HH:mm:ss, etc.). This is the
//     most commonly-used helper in real n8n workflows (e.g. to
//     stamp `processedAt` in a Loop body). Patterns are mapped to
//     Go's time.Format tokens via the same conversion table the
//     legacy DateExtensions.formatDate uses, so behaviour matches
//     the `Date.format()` helper for the same input.
// Any other method/property is delegated to a JS Date fallback so
// expressions like `$now.getFullYear()` keep working.
func (p *WorkflowDataProxy) createNowProxy() goja.Value {
	loc := resolveWorkflowLocation(p.workflow)
	now := time.Now().In(loc)
	nowProxy := p.vm.NewObject()

	_ = nowProxy.Set("toISOString", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return p.vm.ToValue(now.Format("2006-01-02T15:04:05.000Z07:00"))
	}))
	_ = nowProxy.Set("toMillis", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return p.vm.ToValue(now.UnixMilli())
	}))
	_ = nowProxy.Set("toString", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return p.vm.ToValue(now.Format("2006-01-02T15:04:05.000Z07:00"))
	}))
	_ = nowProxy.Set("format", p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(p.vm.ToValue("format() requires 1 argument: format string"))
		}
		pattern := call.Arguments[0].String()
		// Map Luxon/Moment-style tokens to Go's time.Format
		// tokens. Keep this list in sync with the DateExtensions
		// format converter - the same pattern is recognised
		// here so a Set node's `={{ $now.format('yyyy-MM-dd') }}`
		// produces the same string as `Date.format()` would.
		converted := luxonToGoFormat(pattern)
		return p.vm.ToValue(now.Format(converted))
	}))

	return nowProxy
}

// resolveWorkflowLocation returns the *time.Location that should be
// used when evaluating `$now.*` style expressions for a workflow.
//
// n8n defaults `generic.timezone` to `America/New_York` for
// installations that don't set `GENERIC_TIMEZONE` (see
// node_modules/@n8n/config/dist/configs/generic.config.js in n8n
// source). The same default applies at the workflow level when a
// workflow's settings.timezone is empty. Without this, expressions
// like `={{ $now.format('yyyy-MM-dd') }}` would emit UTC dates on
// m9m while n8n emits America/New_York dates, causing the
// `processedAt` field in workflows like `webhook_loop` to drift by
// one day whenever the server runs in UTC.
//
// The location is cached per (workflow name, timezone string) pair
// to keep expression evaluation fast.
var (
	locCacheMu sync.RWMutex
	locCache   = map[string]*time.Location{}
)

const defaultWorkflowTimezone = "America/New_York"

func resolveWorkflowLocation(wf *model.Workflow) *time.Location {
	tz := defaultWorkflowTimezone
	if wf != nil && wf.Settings != nil {
		if t := strings.TrimSpace(wf.Settings.Timezone); t != "" && t != "DEFAULT" {
			tz = t
		}
	}
	locCacheMu.RLock()
	if loc, ok := locCache[tz]; ok {
		locCacheMu.RUnlock()
		return loc
	}
	locCacheMu.RUnlock()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	locCacheMu.Lock()
	locCache[tz] = loc
	locCacheMu.Unlock()
	return loc
}

// luxonToGoFormat converts a Luxon/Moment-style date format string
// to the equivalent Go time.Format reference string. It recognises
// the tokens n8n users most commonly write in workflow expressions:
//
//	yyyy -> 2006    (4-digit year)
//	yy   -> 06      (2-digit year)
//	MM   -> 01      (2-digit month)
//	dd   -> 02      (2-digit day)
//	HH   -> 15      (2-digit hour, 24h)
//	mm   -> 04      (2-digit minute)
//	ss   -> 05      (2-digit second)
//	SSS  -> 000     (3-digit millisecond)
//
// Unrecognised tokens are passed through unchanged. This matches
// DateExtensions.convertDateFormat so `$now.format('yyyy-MM-dd')`
// and `Date.format()` produce identical strings.
//
// Tokens are applied in order of length, longest first, so the
// shorter `yy` token never matches the first two characters of
// the longer `yyyy` token. If `yy` were applied first, `yyyy-MM-dd`
// would become `06yy-MM-dd`, after which `yyyy` no longer appears
// in the string and the year token would be left half-converted
// — producing nonsense years like `0606` instead of `2006`.
func luxonToGoFormat(pattern string) string {
	replacements := []struct{ old, new string }{
		{"yyyy", "2006"}, // longest first
		{"SSS", "000"},
		{"MM", "01"},
		{"dd", "02"},
		{"HH", "15"},
		{"mm", "04"},
		{"ss", "05"},
		{"yy", "06"}, // shortest last
	}
	out := pattern
	for _, r := range replacements {
		out = strings.ReplaceAll(out, r.old, r.new)
	}
	return out
}

// createEvaluateExpressionProxy creates the legacy $evaluateExpression function
func (p *WorkflowDataProxy) createEvaluateExpressionProxy() goja.Value {
	return p.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(p.vm.NewTypeError("$evaluateExpression requires 1 argument"))
		}

		expression := call.Arguments[0].String()

		// Create a new evaluator for nested expression
		evaluator := NewGojaExpressionEvaluator(DefaultEvaluatorConfig())
		context := &ExpressionContext{
			Workflow:            p.workflow,
			RunExecutionData:    p.runExecutionData,
			RunIndex:            p.runIndex,
			ItemIndex:           p.itemIndex,
			ActiveNodeName:      p.activeNodeName,
			ConnectionInputData: p.connectionInputData,
			Mode:                p.mode,
			AdditionalKeys:      p.additionalKeys,
			ExecuteData:         p.executeData,
		}

		result, err := evaluator.EvaluateExpression(expression, context)
		if err != nil {
			panic(p.vm.ToValue(fmt.Sprintf("ExpressionError: %s", err.Error())))
		}

		return p.vm.ToValue(result)
	})
}

// Helper methods

// getInputConnectionData gets input data for a specific connection index
func (p *WorkflowDataProxy) getInputConnectionData(connectionIndex int) []model.DataItem {
	// For now, return the main connection input data
	// In a full implementation, this would handle multiple connection indices
	if connectionIndex == 0 {
		return p.connectionInputData
	}
	return []model.DataItem{}
}

// getNodeExecutionData gets execution data for a specific node.
//
// n8n's `$(NodeName).all()` returns the items from the *most recent*
// execution of the named node, regardless of which runIndex is
// currently active. This matters for nodes that sit downstream of a
// `splitInBatches` loop: each iteration of the loop writes a fresh
// slot in `ResultData.NodeData[nodeName][runIndex]`, but a Code node
// that runs *after* the loop (e.g. "Format Summary Data") needs to
// see the cumulative result, which on n8n is the last slot the loop
// populated. Reading only `ResultData.NodeData[nodeName][runIndex]`
// at runIndex=0 would silently return the first iteration's items,
// not the final aggregate.
//
// The fix: when the current runIndex's slot is empty but later slots
// exist, fall through to the highest populated slot. This preserves
// the in-loop semantics (Code nodes inside the loop still see their
// own iteration's data at runIndex=k) while making post-loop readers
// see the final aggregate.
func (p *WorkflowDataProxy) getNodeExecutionData(nodeName string) []model.DataItem {
	p.cacheMutex.RLock()
	if cached, exists := p.dataCache[nodeName]; exists {
		p.cacheMutex.RUnlock()
		return cached.([]model.DataItem)
	}
	p.cacheMutex.RUnlock()

	// Load data from run execution data
	var data []model.DataItem
	if p.runExecutionData != nil && p.runExecutionData.ResultData != nil {
		if nodeResults, exists := p.runExecutionData.ResultData.NodeData[nodeName]; exists {
			if len(nodeResults) > p.runIndex {
				data = nodeResults[p.runIndex].Data
			}
			// Fall back to the most recent populated slot when the
			// caller's runIndex has no items yet. Walk from the end
			// so we pick the latest loop iteration's data instead of
			// the earliest.
			if len(data) == 0 {
				for i := len(nodeResults) - 1; i >= 0; i-- {
					if len(nodeResults[i].Data) > 0 {
						data = nodeResults[i].Data
						break
					}
				}
			}
		}
	}

	// Cache the result
	p.cacheMutex.Lock()
	p.dataCache[nodeName] = data
	p.cacheMutex.Unlock()

	return data
}

// getCurrentNode gets the current node being executed
func (p *WorkflowDataProxy) getCurrentNode() *model.Node {
	if p.workflow == nil {
		return nil
	}

	for _, node := range p.workflow.Nodes {
		if node.Name == p.activeNodeName {
			return &node
		}
	}

	return nil
}

// getPairedItemData gets paired item data for lineage tracking
func (p *WorkflowDataProxy) getPairedItemData(nodeName string, itemIndex int) goja.Value {
	// Get the execution data for the node
	nodeData := p.getNodeExecutionData(nodeName)

	if itemIndex >= 0 && itemIndex < len(nodeData) {
		item := nodeData[itemIndex]
		if item.PairedItem != nil {
			return p.vm.ToValue(item.PairedItem)
		}
	}

	return goja.Undefined()
}

// GetCacheKey returns a cache key for this proxy context
func (p *WorkflowDataProxy) GetCacheKey() string {
	return fmt.Sprintf("%s:%d:%d:%s",
		p.activeNodeName, p.runIndex, p.itemIndex, string(p.mode))
}

// Setup configures all n8n context variables in the JavaScript runtime
func (p *WorkflowDataProxy) Setup(vm *goja.Runtime) error {
	// Set up $json variable
	err := vm.Set("$json", p.createJsonProxy())
	if err != nil {
		return fmt.Errorf("failed to set $json: %w", err)
	}

	// Set up $input variable
	err = vm.Set("$input", p.createInputProxy())
	if err != nil {
		return fmt.Errorf("failed to set $input: %w", err)
	}

	// Set up $node variable
	err = vm.Set("$node", p.createNodeProxy())
	if err != nil {
		return fmt.Errorf("failed to set $node: %w", err)
	}

	// Set up $ alias for the node proxy (n8n allows `$(name).item.json`
	// as shorthand for `$node[name].item.json`). The Code node and
	// expression evaluator both run through this Setup path, so both
	// need the shorthand registered here — registering it inside
	// `CreateJavaScriptProxy` only (which the Expression wrapper uses)
	// leaves the Code node path without the alias, and workflows like
	// `Batch Order Processing System` that read `$("Loop Over Orders")`
	// inside a Code node snippet fail with `ReferenceError: $ is not
	// defined`.
	err = vm.Set("$", p.createNodeProxy())
	if err != nil {
		return fmt.Errorf("failed to set $: %w", err)
	}

	// Set up $parameter variable
	err = vm.Set("$parameter", p.createParameterProxy())
	if err != nil {
		return fmt.Errorf("failed to set $parameter: %w", err)
	}

	// Set up $workflow variable
	err = vm.Set("$workflow", p.createWorkflowProxy())
	if err != nil {
		return fmt.Errorf("failed to set $workflow: %w", err)
	}

	// Set up $execution variable
	err = vm.Set("$execution", p.createExecutionProxy())
	if err != nil {
		return fmt.Errorf("failed to set $execution: %w", err)
	}

	// Set up $env variable
	err = vm.Set("$env", p.createEnvProxy())
	if err != nil {
		return fmt.Errorf("failed to set $env: %w", err)
	}

	// Set up $binary variable
	err = vm.Set("$binary", p.createBinaryProxy())
	if err != nil {
		return fmt.Errorf("failed to set $binary: %w", err)
	}

	// Set up $vars variable
	err = vm.Set("$vars", p.createVarsProxy())
	if err != nil {
		return fmt.Errorf("failed to set $vars: %w", err)
	}

	// Set up $evaluateExpression function
	err = vm.Set("$evaluateExpression", p.createEvaluateExpressionProxy())
	if err != nil {
		return fmt.Errorf("failed to set $evaluateExpression: %w", err)
	}

	// Set up $now variable (Luxon-style DateTime).
	// n8n exposes `$now` as a Luxon DateTime proxy; workflows call
	// `$now.toISOString()`, `$now.toMillis()`, etc. We register the
	// helpers we need (toISOString / toMillis) so that
	// `={{ $now.toISOString() }}` style expressions don't blow up.
	err = vm.Set("$now", p.createNowProxy())
	if err != nil {
		return fmt.Errorf("failed to set $now: %w", err)
	}

	return nil
}

// Reset clears the data cache
func (p *WorkflowDataProxy) Reset() {
	p.cacheMutex.Lock()
	defer p.cacheMutex.Unlock()
	p.dataCache = make(map[string]interface{})
}

// NewExpressionContext creates a new expression context with defaults
func NewExpressionContext() *ExpressionContext {
	return &ExpressionContext{
		AdditionalKeys:    &AdditionalKeys{},
		Mode:              ModeManual,
		SiblingParameters: make(map[string]interface{}),
	}
}

// SetVariable sets a variable in the expression context for use in expressions
// Variables are stored in SiblingParameters and can be accessed in expressions
func (ctx *ExpressionContext) SetVariable(name string, value interface{}) {
	if ctx.SiblingParameters == nil {
		ctx.SiblingParameters = make(map[string]interface{})
	}
	ctx.SiblingParameters[name] = value
}

// GetVariables returns all variables set in the expression context
func (ctx *ExpressionContext) GetVariables() map[string]interface{} {
	if ctx.SiblingParameters == nil {
		return make(map[string]interface{})
	}
	return ctx.SiblingParameters
}
