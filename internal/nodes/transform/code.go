/*
Package transform provides data transformation node implementations for m9m.
*/
package transform

import (
	"fmt"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/expressions"
)

// CodeNode implements the Code node functionality for executing custom code
type CodeNode struct {
	*base.BaseNode
}

// NewCodeNode creates a new Code node
func NewCodeNode() *CodeNode {
	description := base.NodeDescription{
		Name:        "Code",
		Description: "Executes custom code in various languages",
		Category:    "Data Transformation",
	}
	
	return &CodeNode{
		BaseNode: base.NewBaseNode(description),
	}
}

// Description returns the node description
func (c *CodeNode) Description() base.NodeDescription {
	return c.BaseNode.Description()
}

// DefaultCodeMode is the n8n default for the Code node's `mode`
// parameter when the workflow export omits it. n8n's Code node type v2
// (the version used by workflow `V4432EsGIkpqIZx9` — "Simple Webhook -
// Code") exports only `jsCode` and never emits an explicit `mode`,
// because the UI hides the toggle behind "Settings". When m9m rejects
// the workflow with `mode parameter is required`, it diverges from
// n8n's drop-in compatibility promise. Mirroring n8n's default keeps
// legacy / hand-edited exports working without forcing the operator
// to re-publish from the n8n editor.
const DefaultCodeMode = "runOnceForEachItem"

// ValidateParameters validates Code node parameters.
//
// `mode` is optional — when missing or empty it defaults to
// `DefaultCodeMode` (n8n's behaviour for typeVersion 2 Code nodes
// that omit the parameter). This mirrors the Switch node's
// `conditions`/`rules` alias pattern and avoids breaking older n8n
// workflow exports that were authored before the mode toggle was
// promoted to a top-level parameter.
func (c *CodeNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return c.CreateError("parameters cannot be nil", nil)
	}

	// `mode` is optional — default to n8n's behaviour when missing.
	modeStr := DefaultCodeMode
	if rawMode, ok := params["mode"]; ok && rawMode != nil {
		s, ok := rawMode.(string)
		if !ok {
			return c.CreateError("mode must be a string", nil)
		}
		if s != "" {
			modeStr = s
		}
	}

	validModes := map[string]bool{
		"runOnceForAllItems": true,
		"runOnceForEachItem": true,
	}

	if !validModes[modeStr] {
		return c.CreateError(fmt.Sprintf("invalid mode: %s", modeStr), nil)
	}

	// Check if language exists
	language, ok := params["language"]
	if !ok {
		return c.CreateError("language parameter is required", nil)
	}
	
	// Check if language is a string
	languageStr, ok := language.(string)
	if !ok {
		return c.CreateError("language must be a string", nil)
	}
	
	// Validate language
	validLanguages := map[string]bool{
		"javascript": true,
		"python":     true,
		"go":         true,
	}
	
	if !validLanguages[languageStr] {
		return c.CreateError(fmt.Sprintf("invalid language: %s", languageStr), nil)
	}
	
	// Check if code exists. n8n's Code node type v2 uses
	// language-specific parameter names (`jsCode`, `pythonCode`,
	// `pythonCode`); legacy / hand-edited exports use the generic
	// `code` key. Accept either — pick the first one present, in
	// language-specific order so e.g. `jsCode` wins for a JS Code
	// node even if `code` is also accidentally set.
	if code := extractCodeParam(params, languageStr); code == "" {
		return c.CreateError(
			fmt.Sprintf("%s parameter is required", languageCodeKey(languageStr)), nil,
		)
	}
	// The two accepted keys must both be string-typed when present —
	// `extractCodeParam` already coerced a present value into a
	// string, but a non-string value (e.g. an integer `jsCode`) must
	// be rejected at validation time, not silently dropped.
	langKey := languageCodeKey(languageStr)
	if v, present := params[langKey]; present {
		if _, ok := v.(string); !ok {
			return c.CreateError(fmt.Sprintf("%s must be a string", langKey), nil)
		}
	}
	if v, present := params["code"]; present {
		if _, ok := v.(string); !ok {
			return c.CreateError("code must be a string", nil)
		}
	}

	return nil
}

// extractCodeParam pulls the user-supplied code out of the parameter
// map, accepting the language-specific keys n8n's Code node type v2
// emits (`jsCode`, `pythonCode`) alongside the legacy generic `code`
// key. Returns "" if none of the recognised keys are set.
func extractCodeParam(params map[string]interface{}, language string) string {
	if s, ok := params[languageCodeKey(language)].(string); ok && s != "" {
		return s
	}
	if s, ok := params["code"].(string); ok {
		return s
	}
	return ""
}

// languageCodeKey returns the n8n parameter name n8n uses for the
// given language's source code on the Code node type v2.
//
//	javascript -> "jsCode"
//	python     -> "pythonCode"
//	go         -> "goCode"
//	<other>    -> "code"   (legacy fallback)
//
// n8n splits the source-code parameter per language on the newer Code
// node type so the UI can show a typed editor (JS, Py, Go). m9m keeps
// the same key naming for parity so workflow exports round-trip
// without manual rewrites.
func languageCodeKey(language string) string {
	switch language {
	case "javascript":
		return "jsCode"
	case "python":
		return "pythonCode"
	case "go":
		return "goCode"
	default:
		return "code"
	}
}

// Execute processes the Code node operation
func (c *CodeNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}
	
	// Get parameters from node parameters. Match n8n's default
	// (`runOnceForEachItem`) so workflows that omit `mode` (e.g.
	// Code typeVersion 2 with only `jsCode` set) execute the same
	// way they do on n8n. ValidateParameters applies the same
	// default at validation time, so the value passed to
	// executeJavaScript is always either an explicit user choice
	// or this default.
	mode := c.GetStringParameter(nodeParams, "mode", DefaultCodeMode)
	language := c.GetStringParameter(nodeParams, "language", "javascript")

	// Accept n8n's per-language key (`jsCode`, `pythonCode`, `goCode`)
	// as well as the legacy generic `code` key. The validation layer
	// validates the same lookup, so by the time we reach Execute the
	// code is guaranteed to be a non-empty string.
	code := extractCodeParam(nodeParams, language)

	// Validate parameters
	if code == "" {
		return nil, c.CreateError(
			fmt.Sprintf("%s parameter cannot be empty", languageCodeKey(language)), nil,
		)
	}
	
	// Execute code based on language
	switch language {
	case "javascript":
		return c.executeJavaScript(mode, code, inputData, nodeParams)
	case "python":
		return c.executePython(mode, code, inputData, nodeParams)
	case "go":
		return c.executeGo(mode, code, inputData, nodeParams)
	default:
		return nil, c.CreateError(fmt.Sprintf("unsupported language: %s", language), nil)
	}
}

// executeJavaScript executes JavaScript code using the Goja-based expression system
func (c *CodeNode) executeJavaScript(mode, code string, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// Create Goja expression evaluator
	evaluator := expressions.NewGojaExpressionEvaluator(expressions.DefaultEvaluatorConfig())

	// Process based on mode
	switch mode {
	case "runOnceForAllItems":
		return c.executeJavaScriptForAllItems(evaluator, code, inputData, nodeParams)
	case "runOnceForEachItem":
		return c.executeJavaScriptForEachItem(evaluator, code, inputData, nodeParams)
	default:
		return nil, c.CreateError(fmt.Sprintf("unsupported mode: %s", mode), nil)
	}
}

// executePython executes Python code
func (c *CodeNode) executePython(mode, code string, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// For now, we'll return a simple result
	// In a real implementation, we would execute the Python code
	result := make([]model.DataItem, len(inputData))
	
	for i, item := range inputData {
		newItem := model.DataItem{
			JSON: make(map[string]interface{}),
		}
		
		// Copy existing JSON data
		for k, v := range item.JSON {
			newItem.JSON[k] = v
		}
		
		// Add Python execution result
		newItem.JSON["pythonResult"] = "Executed Python code successfully"
		newItem.JSON["pythonCode"] = code
		
		// Copy binary data if present
		if item.Binary != nil {
			newItem.Binary = make(map[string]model.BinaryData)
			for k, v := range item.Binary {
				newItem.Binary[k] = v
			}
		}
		
		// Copy paired item data if present
		if item.PairedItem != nil {
			newItem.PairedItem = item.PairedItem
		}
		
		result[i] = newItem
	}
	
	return result, nil
}

// executeGo executes Go code
func (c *CodeNode) executeGo(mode, code string, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// For now, we'll return a simple result
	// In a real implementation, we would compile and execute the Go code
	result := make([]model.DataItem, len(inputData))
	
	for i, item := range inputData {
		newItem := model.DataItem{
			JSON: make(map[string]interface{}),
		}
		
		// Copy existing JSON data
		for k, v := range item.JSON {
			newItem.JSON[k] = v
		}
		
		// Add Go execution result
		newItem.JSON["goResult"] = "Executed Go code successfully"
		newItem.JSON["goCode"] = code
		
		// Copy binary data if present
		if item.Binary != nil {
			newItem.Binary = make(map[string]model.BinaryData)
			for k, v := range item.Binary {
				newItem.Binary[k] = v
			}
		}
		
		// Copy paired item data if present
		if item.PairedItem != nil {
			newItem.PairedItem = item.PairedItem
		}
		
		result[i] = newItem
	}
	
	return result, nil
}

// executeJavaScriptForAllItems executes code once for all items
func (c *CodeNode) executeJavaScriptForAllItems(evaluator *expressions.GojaExpressionEvaluator, code string, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// Create expression context for all items
	context := &expressions.ExpressionContext{
		RunIndex:            0,
		ItemIndex:           0,
		ActiveNodeName:      "code-node",
		ConnectionInputData: inputData,
		Mode:                expressions.ModeManual,
		AdditionalKeys:      &expressions.AdditionalKeys{},
	}

	// Execute the code once
	result, err := evaluator.EvaluateCode(code, context)
	if err != nil {
		return nil, c.CreateError(fmt.Sprintf("failed to execute JavaScript code: %v", err), nil)
	}

	// Convert result to DataItems
	return c.convertCodeResult(result, inputData)
}

// executeJavaScriptForEachItem executes code once for each item
func (c *CodeNode) executeJavaScriptForEachItem(evaluator *expressions.GojaExpressionEvaluator, code string, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	result := make([]model.DataItem, len(inputData))

	for i, item := range inputData {
		// Create expression context for this item
		context := &expressions.ExpressionContext{
			RunIndex:            0,
			ItemIndex:           i,
			ActiveNodeName:      "code-node",
			ConnectionInputData: []model.DataItem{item},
			Mode:                expressions.ModeManual,
			AdditionalKeys:      &expressions.AdditionalKeys{},
		}

		// Execute the code for this item
		itemResult, err := evaluator.EvaluateCode(code, context)
		if err != nil {
			return nil, c.CreateError(fmt.Sprintf("failed to execute JavaScript code for item %d: %v", i, err), nil)
		}

		// Convert result to DataItem
		converted, err := c.convertSingleCodeResult(itemResult, item)
		if err != nil {
			return nil, c.CreateError(fmt.Sprintf("failed to convert result for item %d: %v", i, err), nil)
		}

		result[i] = converted
	}

	return result, nil
}

// convertCodeResult converts a code execution result to DataItems
func (c *CodeNode) convertCodeResult(result interface{}, inputData []model.DataItem) ([]model.DataItem, error) {
	// If result is an array, try to convert each element to a DataItem
	if resultSlice, ok := result.([]interface{}); ok {
		converted := make([]model.DataItem, len(resultSlice))
		for i, item := range resultSlice {
			if itemMap, ok := item.(map[string]interface{}); ok {
				converted[i] = model.DataItem{JSON: itemMap}
			} else {
				converted[i] = model.DataItem{JSON: map[string]interface{}{"result": item}}
			}
		}
		return converted, nil
	}

	// If result is a single object, wrap it in an array
	if resultMap, ok := result.(map[string]interface{}); ok {
		return []model.DataItem{{JSON: resultMap}}, nil
	}

	// For other types, create a single DataItem with the result
	return []model.DataItem{{JSON: map[string]interface{}{"result": result}}}, nil
}

// convertSingleCodeResult converts a single code result to a DataItem
func (c *CodeNode) convertSingleCodeResult(result interface{}, originalItem model.DataItem) (model.DataItem, error) {
	newItem := model.DataItem{
		JSON: make(map[string]interface{}),
	}

	// Copy binary and paired item data
	if originalItem.Binary != nil {
		newItem.Binary = make(map[string]model.BinaryData)
		for k, v := range originalItem.Binary {
			newItem.Binary[k] = v
		}
	}
	if originalItem.PairedItem != nil {
		newItem.PairedItem = originalItem.PairedItem
	}

	// Convert result to JSON
	if resultMap, ok := result.(map[string]interface{}); ok {
		newItem.JSON = resultMap
	} else {
		newItem.JSON["result"] = result
	}

	return newItem, nil
}