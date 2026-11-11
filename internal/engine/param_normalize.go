/*
Package engine provides the core workflow execution engine for m9m.

This file implements the parameter-shape normalizer. m9m reads node
parameters with two different conventions:

  - Nested (the n8n wire format). A property named `a.b` is stored as
    the object `{"a": {"b": <value>}}`. The metadata-driven editor
    always writes this shape, and most executors read it with
    `params["a"].(map[string]interface{})["b"]`.

  - Flat (a literal dotted key). The same property is stored under the
    single key `"a.b"`. A few executors read this shape, e.g. the HTTP
    Request node reads `response.includeResponseHeadersAndStatus`
    (internal/nodes/http/request.go).

Both shapes can legitimately appear in a saved workflow: the nested one
because the editor writes it, the flat one because hand-written or
legacy workflow JSON uses it. NormalizeNodeParameters makes every value
reachable in BOTH shapes so neither reader can miss it, without ever
changing the n8n wire format of the persisted workflow.
*/
package engine

import (
	"sort"
	"strings"
)

// maxParamNestingDepth bounds how deep the normalizer descends into a
// nested parameter tree when materialising flat dotted-key aliases. It
// guards the engine against pathological (or accidentally cyclic) maps.
const maxParamNestingDepth = 32

// NormalizeNodeParameters returns a parameter map in which every value is
// reachable both as a flat dotted key and as a nested path.
//
// The function is a pure function with respect to its input: the map
// passed in is never mutated. When no change is required the very same
// map is returned; otherwise a private copy is returned and the caller's
// map (including any nested maps it points at) is left untouched.
//
// Normalisation rules
//
//  1. Flat -> nested. A top-level key that contains a dot and whose
//     segments are all non-empty ("response.includeResponseHeadersAndStatus")
//     is materialised as the nested path
//     `{"response": {"includeResponseHeadersAndStatus": ...}}`.
//
//  2. Nested -> flat. Every entry of a nested object also gets a flat
//     "a.b" alias, recursively ("a.b.c" for deeper levels). Arrays are
//     materialised as a whole value and are never descended into,
//     because array elements have no stable flat key.
//
//  3. Precedence. Normalisation NEVER overwrites a value that already
//     exists. If a parameter is present in both shapes with different
//     values, each shape keeps the value it had: an executor that reads
//     the flat key sees the flat value, an executor that reads the
//     nested path sees the nested value. Only missing shapes are
//     filled in. The editor deletes the legacy flat key when it writes
//     a nested value (web/src/components/workflow/parameters/
//     parameterPath.ts), so in practice the two shapes never disagree.
//
//  4. Blocked paths. If an intermediate segment of a nested path holds
//     a non-object value ("a": "text"), the path cannot be created and
//     the flat key is left exactly as it is.
//
//  5. Idempotency. Normalising an already normalised map returns the
//     same map and changes nothing.
//
// # Credential envelope
//
// The engine injects credential fields as extra top-level keys
// ("kafka_brokers" and friends) right before a node executes. The
// normaliser runs after that injection, so an injected key that itself
// contains a dot also becomes reachable under its nested path. Because
// the normaliser works on a copy, injected keys are never written back
// into the user-visible `node.Parameters`, so no synthesised duplicate
// can leak into the persisted workflow JSON.
//
// # Expression evaluation
//
// The normaliser only rearranges parameter maps. It never touches the
// expression engine: `internal/expressions` resolves `{{ $json.x }}`
// against the item payload (ExpressionContext.ConnectionInputData),
// not against node parameters, so normalisation cannot change the
// result of an expression.
func NormalizeNodeParameters(params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return params
	}

	out := params
	dirty := false

	// own returns a private shallow copy of the input map. It is called
	// lazily, right before the first mutation, so a map that needs no
	// normalisation is returned unchanged (no allocation, no copy).
	own := func() map[string]interface{} {
		if !dirty {
			copied := make(map[string]interface{}, len(params))
			for k, v := range params {
				copied[k] = v
			}
			out = copied
			dirty = true
		}
		return out
	}

	// Pass 1 - flat dotted key -> nested path.
	//
	// The nested path is only created when it does not exist yet and
	// when no intermediate segment is blocked by a non-object value.
	for _, key := range sortedParamKeys(params) {
		if !strings.Contains(key, ".") {
			continue
		}
		segments, ok := splitDottedParamKey(key)
		if !ok {
			continue
		}
		if nestedParamPathExists(out, segments) {
			// The nested shape already carries a value - never overwrite it.
			continue
		}
		if !canCreateNestedParamPath(out, segments) {
			// A non-object value blocks the path - leave the caller's data alone.
			continue
		}
		own()
		setNestedParamPath(out, segments, out[key])
	}

	// Pass 2 - nested object -> flat dotted-key alias.
	//
	// Aliases are collected first and applied afterwards so that the
	// "never overwrite" check always sees the original key set.
	aliases := collectFlatParamAliases(out, out, "", 0)
	for _, a := range aliases {
		if _, exists := out[a.key]; exists {
			continue
		}
		own()
		out[a.key] = a.value
	}

	if !dirty {
		return params
	}
	return out
}

// NormalizeNodeParams is a short alias for NormalizeNodeParameters.
func NormalizeNodeParams(params map[string]interface{}) map[string]interface{} {
	return NormalizeNodeParameters(params)
}

// paramAlias is a flat dotted key that should be materialised for a
// nested parameter value.
type paramAlias struct {
	key   string
	value interface{}
}

// collectFlatParamAliases walks the nested maps reachable from root and
// returns the flat dotted-key aliases that are missing from root.
//
// Only top-level keys without a dot are walked: a top-level key that
// contains a dot is itself a flat alias, and pass 1 of the normaliser
// guarantees that the equivalent nested path exists as well.
func collectFlatParamAliases(root map[string]interface{}, current map[string]interface{}, prefix string, depth int) []paramAlias {
	if depth > maxParamNestingDepth || len(current) == 0 {
		return nil
	}

	var aliases []paramAlias
	for _, key := range sortedParamKeys(current) {
		if key == "" || strings.Contains(key, ".") {
			// An empty or dotted segment cannot be represented as a flat
			// dotted key without ambiguity - skip it.
			continue
		}
		flat := key
		if prefix != "" {
			flat = prefix + "." + key
		}
		value := current[key]
		if _, exists := root[flat]; !exists {
			aliases = append(aliases, paramAlias{key: flat, value: value})
		}
		if child, ok := value.(map[string]interface{}); ok {
			aliases = append(aliases, collectFlatParamAliases(root, child, flat, depth+1)...)
		}
	}
	return aliases
}

// splitDottedParamKey splits a flat dotted key into its segments. It
// reports false for keys that cannot be represented as a nested path
// (an empty segment such as "a..b", ".a" or "a.").
func splitDottedParamKey(key string) ([]string, bool) {
	segments := strings.Split(key, ".")
	if len(segments) < 2 {
		return nil, false
	}
	for _, segment := range segments {
		if segment == "" {
			return nil, false
		}
	}
	return segments, true
}

// nestedParamPathExists reports whether the nested path addressed by
// segments already carries a value. A path blocked by a non-object
// intermediate value is reported as "not present"; the caller then
// checks whether the path can be created at all.
func nestedParamPathExists(root map[string]interface{}, segments []string) bool {
	current := root
	for i, segment := range segments {
		value, ok := current[segment]
		if !ok {
			return false
		}
		if i == len(segments)-1 {
			return true
		}
		child, ok := value.(map[string]interface{})
		if !ok {
			return false
		}
		current = child
	}
	return false
}

// canCreateNestedParamPath reports whether the nested path addressed by
// segments can be created without replacing an existing value.
func canCreateNestedParamPath(root map[string]interface{}, segments []string) bool {
	current := root
	for i := 0; i < len(segments)-1; i++ {
		value, ok := current[segments[i]]
		if !ok {
			return true
		}
		child, ok := value.(map[string]interface{})
		if !ok {
			// A non-object value blocks the path.
			return false
		}
		current = child
	}
	return true
}

// setNestedParamPath stores value under the nested path addressed by
// segments, creating the intermediate objects on the way.
//
// The write is copy-on-write: every intermediate object owned by the
// caller is cloned before it is modified, so neither the caller's map
// nor any nested map it points at is mutated. The leaf is only written
// when it does not exist yet.
func setNestedParamPath(root map[string]interface{}, segments []string, value interface{}) {
	current := root
	for i := 0; i < len(segments)-1; i++ {
		segment := segments[i]
		child, exists := current[segment]
		if !exists {
			created := make(map[string]interface{})
			current[segment] = created
			current = created
			continue
		}
		existing, ok := child.(map[string]interface{})
		if !ok {
			// Blocked by a non-object value - never clobber it.
			return
		}
		clone := make(map[string]interface{}, len(existing)+1)
		for k, v := range existing {
			clone[k] = v
		}
		current[segment] = clone
		current = clone
	}
	current[segments[len(segments)-1]] = value
}

// sortedParamKeys returns the map keys in a stable order so that the
// normalisation result does not depend on Go's map iteration order.
func sortedParamKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
