// Package data contains nodes that operate on m9m-managed,
// n8n-compatible data tables (a.k.a. n8n's "Data Tables" feature).
//
// m9m's data tables are a thin layer over the existing
// `storage.WorkflowStorage` key-value interface. Every row is a
// JSON blob keyed at `dataTable:{tableId}:{rowId}`. List / filter
// operations load the full set of rows for a table into memory
// and apply the condition filter — acceptable for the expected
// workload (customer mappings, config tables, small lookup sets).
// The 0.5.0 milestone (see m9m.MD §12) introduces a dedicated
// `m9m_data_table_rows` MySQL table with native column indexes
// when the row count grows past a few thousand per table.
package data

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// DataTableStore is the storage subset the dataTable node needs.
// We keep the surface tiny so the test suite can mock it with a
// map and the production wiring can pass `*storage.MySQLStorage`
// (or any other `storage.WorkflowStorage`) directly.
type DataTableStore interface {
	SaveRaw(key string, value []byte) error
	GetRaw(key string) ([]byte, error)
	ListKeys(prefix string) ([]string, error)
	DeleteRaw(key string) error
}

// dataTablePrefix is the KV key prefix the node uses. Centralised
// so callers (sync-service, REST handlers, tests) can compute the
// same prefix without duplicating the string.
const dataTablePrefix = "dataTable:"

// dataTableKey is the per-row key shape. The table id is the
// n8n-style opaque string (e.g. "STeu50G0h27glAL8").
func dataTableKey(tableID, rowID string) string {
	return fmt.Sprintf("%s%s:%s", dataTablePrefix, tableID, rowID)
}

// tablePrefix is the prefix used by ListKeys to enumerate a
// table's rows.
func tablePrefix(tableID string) string {
	return fmt.Sprintf("%s%s:", dataTablePrefix, tableID)
}

// rowIDFromKey strips the prefix and returns just the row id.
// Returns empty string if the key isn't a dataTable row key.
func rowIDFromKey(key string) string {
	if !strings.HasPrefix(key, dataTablePrefix) {
		return ""
	}
	rest := strings.TrimPrefix(key, dataTablePrefix)
	idx := strings.IndexByte(rest, ':')
	if idx < 0 {
		return ""
	}
	return rest[idx+1:]
}

// DataTableNode implements `n8n-nodes-base.dataTable` — the
// n8n Data Tables node. The same struct serves both the
// "trigger" path (none — dataTable is a regular action node, not
// a trigger) and the workflow-time action path
// (`Execute(inputData, params)`).
//
// Operations supported: get, insert, update, upsert, delete.
// Filter conditions: equals, notEquals, contains, greaterThan,
// lessThan, isTrue, isFalse, exists, notExists.
// matchType: allConditions (AND, default), anyCondition (OR).
type DataTableNode struct {
	*base.BaseNode
	store DataTableStore

	// currentInput is the input data slice of the in-flight
	// Execute call. Used by `evaluateConditions` so filter
	// expressions like `{{ $json.id }}` see the inbound item.
	// Set in Execute, cleared in a deferred reset so a
	// subsequent call from a different goroutine can't read
	// stale data. NOT a thread-safe cache; the engine is
	// expected to serialise Execute calls per node instance.
	currentInput []model.DataItem
}

// NewDataTableNode constructs a new dataTable node. The store
// must be non-nil for production use; nil is permitted for
// parameter-shape unit tests that never call Execute.
func NewDataTableNode(store DataTableStore) *DataTableNode {
	return &DataTableNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Data Table",
			Description: "Read, write, update, and delete rows in an m9m data table (n8n-compatible Data Tables).",
			Category:    "Data",
			Properties:  dataTableProperties(),
			Inputs:      []string{"main"},
			Outputs:     []string{"main"},
		}),
		store: store,
	}
}

// dataTableProperties mirrors n8n's `INodeTypeDescription.properties`
// for the dataTable node. The `dataTableId` field is a free-form
// string (n8n renders it as a resource locator pointing at the
// `/projects/.../datatables/{id}` URL, but the executor only needs
// the id).
func dataTableProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Get", Value: "get"},
		{Name: "Insert", Value: "insert"},
		{Name: "Update", Value: "update"},
		{Name: "Upsert", Value: "upsert"},
		{Name: "Delete", Value: "delete"},
	}
	matchTypes := []base.Option{
		{Name: "All Conditions (AND)", Value: "allConditions"},
		{Name: "Any Condition (OR)", Value: "anyCondition"},
	}
	props := []base.NodeProperty{
		base.StringProp("Data Table ID", "dataTableId", "", "Opaque table id (e.g. STeu50G0h27glAL8).", "STeu50G0h27glAL8", true),
		base.StringOpt("Operation", "operation", "get", "Data table operation.", ops, true),
		base.StringOpt("Match Type", "matchType", "allConditions", "How to combine multiple filter conditions.", matchTypes, false).WithDisplayOptions(base.NodeProperty_DisplayOptions_Show("operation", []string{"get", "update", "upsert", "delete"})),
		base.JsonProp("Filters", "filters", "{\"conditions\": []}", "Filter conditions (get / update / upsert / delete). Each condition: {keyName, keyValue, condition}."),
		base.JsonProp("Data to Send", "dataToSend", "{}", "Row data (insert / update / upsert)."),
		base.StringProp("Column to Match On (upsert)", "columnToMatchOn", "", "Column name used as the upsert key. If empty, falls back to the first key of `dataToSend`.", "id", false).WithDisplayOptions(base.NodeProperty_DisplayOptions_Show("operation", []string{"upsert"})),
		base.NumberProp("Return All", "returnAll", 0, "If 1, return all matching rows. If 0, return at most `limit` rows.", false).WithDisplayOptions(base.NodeProperty_DisplayOptions_Show("operation", []string{"get"})),
		base.NumberProp("Limit", "limit", 50, "Max rows to return when returnAll=0.", false).WithDisplayOptions(base.NodeProperty_DisplayOptions_Show("operation", []string{"get"})),
	}
	return append(props, base.CommonSettings()...)
}

// Execute dispatches to the configured operation. Parameter
// evaluation is split into a small helper
// (`evaluateParams`) so the same code path can later be reused
// from a `ContextAwareNodeExecutor` interface without changing
// the dispatch logic.
func (n *DataTableNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if n.store == nil {
		return nil, n.CreateError("dataTable: storage backend is not wired (CLI / demo / health mode). Run via `m9m serve` or wire a store.", nil)
	}
	n.currentInput = inputData
	defer func() { n.currentInput = nil }()

	evaluated, err := n.evaluateParams(nodeParams, inputData)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("evaluate params: %v", err), nil)
	}

	tableID := strings.TrimSpace(n.GetStringParameter(evaluated, "dataTableId", ""))
	if tableID == "" {
		return nil, n.CreateError("dataTableId is required", nil)
	}
	operation := n.GetStringParameter(evaluated, "operation", "get")

	switch operation {
	case "get":
		return n.doGet(tableID, evaluated)
	case "insert":
		return n.doInsert(tableID, evaluated, inputData)
	case "update":
		return n.doUpdate(tableID, evaluated)
	case "upsert":
		return n.doUpsert(tableID, evaluated)
	case "delete":
		return n.doDelete(tableID, evaluated)
	default:
		return nil, n.CreateError(fmt.Sprintf("unsupported operation: %s", operation), nil)
	}
}

// evaluateParams walks the param map and resolves any n8n-style
// expressions (e.g. `={{ $json.id }}` or `{{ ... }}`) using the
// first input item as the evaluation context. The dataTable
// node is conventionally used as a lookup, so this is what
// users expect: filter values flow from the inbound payload.
func (n *DataTableNode) evaluateParams(params map[string]interface{}, inputData []model.DataItem) (map[string]interface{}, error) {
	evaluator := expressions.NewExpressionEvaluator()
	if len(inputData) == 0 {
		// No input → fall back to the base class's plain
		// resolver (which also evaluates expressions, just
		// against an empty context).
		return n.EvaluateExpressions(params, &expressions.ExecutionContext{})
	}
	ctx := &expressions.ExecutionContext{
		InputData: inputData,
		ItemIndex: 0,
	}
	return n.evaluateMap(params, evaluator, ctx)
}

func (n *DataTableNode) evaluateMap(params map[string]interface{}, eval *expressions.ExpressionEvaluator, ctx *expressions.ExecutionContext) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		switch t := v.(type) {
		case string:
			if expressions.IsExpression(t) {
				r, err := eval.Evaluate(t, ctx)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", k, err)
				}
				out[k] = r
			} else {
				out[k] = t
			}
		case map[string]interface{}:
			nested, err := n.evaluateMap(t, eval, ctx)
			if err != nil {
				return nil, err
			}
			out[k] = nested
		case []interface{}:
			arr, err := n.evaluateSlice(t, eval, ctx)
			if err != nil {
				return nil, err
			}
			out[k] = arr
		default:
			out[k] = v
		}
	}
	return out, nil
}

func (n *DataTableNode) evaluateSlice(in []interface{}, eval *expressions.ExpressionEvaluator, ctx *expressions.ExecutionContext) ([]interface{}, error) {
	out := make([]interface{}, 0, len(in))
	for _, v := range in {
		switch t := v.(type) {
		case string:
			if expressions.IsExpression(t) {
				r, err := eval.Evaluate(t, ctx)
				if err != nil {
					return nil, err
				}
				out = append(out, r)
			} else {
				out = append(out, t)
			}
		case map[string]interface{}:
			nested, err := n.evaluateMap(t, eval, ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, nested)
		default:
			out = append(out, v)
		}
	}
	return out, nil
}

// row is the in-memory shape of a dataTable row. The id is
// generated on insert; the rest is whatever the user put in
// `dataToSend` (or whatever n8n wrote via the Data Tables UI).
type row struct {
	ID  string                 `json:"id"`
	Data map[string]interface{} `json:"data"`
}

// loadTable reads all rows of a table into memory. Returns an
// empty slice if the table doesn't exist yet (matching n8n: a
// missing table is not an error, it's just empty).
func (n *DataTableNode) loadTable(tableID string) ([]*row, error) {
	keys, err := n.store.ListKeys(tablePrefix(tableID))
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	// Sort for stable, deterministic order — important so two
	// workflow runs that happen to read the same table see the
	// same row order, which makes test failures reproducible.
	sort.Strings(keys)
	out := make([]*row, 0, len(keys))
	for _, k := range keys {
		raw, err := n.store.GetRaw(k)
		if err != nil {
			return nil, fmt.Errorf("get %s: %w", k, err)
		}
		var r row
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("decode %s: %w", k, err)
		}
		out = append(out, &r)
	}
	return out, nil
}

// saveRow persists a single row. JSON-encode with a stable key
// order isn't required (Go's encoding/json sorts map keys
// alphabetically already), so what you write is exactly what
// you'll read back.
func (n *DataTableNode) saveRow(tableID string, r *row) error {
	buf, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return n.store.SaveRaw(dataTableKey(tableID, r.ID), buf)
}

// condition is the wire shape of a single filter predicate.
// `KeyName` is the column. `KeyValue` is the comparison value
// (n8n-style: ignored by `isTrue` / `isFalse` / `exists` /
// `notExists`). `Condition` is the operator name.
type condition struct {
	KeyName   string      `json:"keyName"`
	KeyValue  interface{} `json:"keyValue"`
	Condition string      `json:"condition"`
}

type filterSpec struct {
	Conditions []condition `json:"conditions"`
}

func parseFilter(raw interface{}) ([]condition, string, error) {
	if raw == nil {
		return nil, "allConditions", nil
	}
	var spec filterSpec
	switch t := raw.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" || s == "{}" {
			return nil, "allConditions", nil
		}
		if err := json.Unmarshal([]byte(s), &spec); err != nil {
			return nil, "", fmt.Errorf("invalid filters JSON: %w", err)
		}
	case map[string]interface{}:
		buf, err := json.Marshal(t)
		if err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(buf, &spec); err != nil {
			return nil, "", err
		}
	default:
		return nil, "", fmt.Errorf("filters must be an object or string, got %T", raw)
	}
	return spec.Conditions, "allConditions", nil
}

// matchType is read separately from the conditions block — n8n
// keeps it as a sibling key in the params map (not inside the
// `filters` object).
func (n *DataTableNode) getMatchType(evaluated map[string]interface{}) string {
	mt := strings.ToLower(strings.TrimSpace(n.GetStringParameter(evaluated, "matchType", "allConditions")))
	if mt != "anycondition" {
		return "allConditions"
	}
	return "anyCondition"
}

// evaluateConditions walks the condition list and evaluates any
// n8n-style expression in the `keyValue` field. This is what
// makes `{{ $json.id }}` flow from the inbound webhook payload
// into the filter — without it, the literal string would be
// compared against the row, returning nothing.
//
// `n.currentInput` is set by Execute and cleared in a defer
// when the call returns. Engine execution is serialised per
// node, so reading it here is safe.
func (n *DataTableNode) evaluateConditions(conds []condition, _ map[string]interface{}) []condition {
	if len(conds) == 0 {
		return conds
	}
	evaluator := expressions.NewExpressionEvaluator()
	ctx := &expressions.ExecutionContext{
		InputData: n.currentInput,
		ItemIndex: 0,
	}
	out := make([]condition, 0, len(conds))
	for _, c := range conds {
		kv := c.KeyValue
		if s, ok := kv.(string); ok && expressions.IsExpression(s) {
			if r, err := evaluator.Evaluate(s, ctx); err == nil {
				kv = r
			}
		}
		out = append(out, condition{KeyName: c.KeyName, KeyValue: kv, Condition: c.Condition})
	}
	return out
}

// evalCondition returns true iff the condition holds for `row`.
// The unknown-operator branch is a no-op (returns false) — n8n's
// behaviour is similar: a typo'd condition silently matches
// nothing rather than crashing the workflow.
func evalCondition(c condition, r *row) bool {
	switch c.Condition {
	case "equals", "":
		return reflectEqual(r.Data[c.KeyName], c.KeyValue)
	case "notEquals":
		return !reflectEqual(r.Data[c.KeyName], c.KeyValue)
	case "contains":
		return reflectContains(r.Data[c.KeyName], c.KeyValue)
	case "greaterThan":
		return reflectCompare(r.Data[c.KeyName], c.KeyValue) > 0
	case "lessThan":
		return reflectCompare(r.Data[c.KeyName], c.KeyValue) < 0
	case "isTrue":
		return reflectBoolTrue(r.Data[c.KeyName])
	case "isFalse":
		return reflectBoolFalse(r.Data[c.KeyName])
	case "exists":
		_, ok := r.Data[c.KeyName]
		return ok
	case "notExists":
		_, ok := r.Data[c.KeyName]
		return !ok
	}
	return false
}

func filterRows(rows []*row, conds []condition, matchType string) []*row {
	if len(conds) == 0 {
		return rows
	}
	out := make([]*row, 0, len(rows))
	for _, r := range rows {
		var hit bool
		if matchType == "anyCondition" {
			for _, c := range conds {
				if evalCondition(c, r) {
					hit = true
					break
				}
			}
		} else {
			hit = true
			for _, c := range conds {
				if !evalCondition(c, r) {
					hit = false
					break
				}
			}
		}
		if hit {
			out = append(out, r)
		}
	}
	return out
}

// doGet — list rows of a table, optionally filtered.
func (n *DataTableNode) doGet(tableID string, evaluated map[string]interface{}) ([]model.DataItem, error) {
	rows, err := n.loadTable(tableID)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	conds, _, err := parseFilter(evaluated["filters"])
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	conds = n.evaluateConditions(conds, evaluated)
	rows = filterRows(rows, conds, n.getMatchType(evaluated))

	// returnAll=1 → no limit. Otherwise clamp to `limit`
	// (default 50).
	returnAll := n.GetIntParameter(evaluated, "returnAll", 0) == 1
	limit := n.GetIntParameter(evaluated, "limit", 50)
	if !returnAll && limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}

	out := make([]model.DataItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.DataItem{JSON: map[string]interface{}{
			"id":   r.ID,
			"row":  r.Data,
			"json": r.Data, // n8n's dataTable node emits `json` as well
		}})
	}
	return out, nil
}

// doInsert — write a new row. Returns the inserted row's id.
func (n *DataTableNode) doInsert(tableID string, evaluated map[string]interface{}, inputData []model.DataItem) ([]model.DataItem, error) {
	data, err := extractDataToSend(evaluated["dataToSend"], inputData)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	r := &row{ID: uuid.NewString(), Data: data}
	if err := n.saveRow(tableID, r); err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	return []model.DataItem{{JSON: map[string]interface{}{
		"id":          r.ID,
		"row":         r.Data,
		"insertedId":  r.ID,
		"insertedRow": r.Data,
	}}}, nil
}

// doUpdate — patch all rows matching the filter.
func (n *DataTableNode) doUpdate(tableID string, evaluated map[string]interface{}) ([]model.DataItem, error) {
	patch, err := extractDataToSend(evaluated["dataToSend"], nil)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	if len(patch) == 0 {
		return nil, n.CreateError("update requires non-empty `dataToSend`", nil)
	}
	rows, err := n.loadTable(tableID)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	conds, _, err := parseFilter(evaluated["filters"])
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	conds = n.evaluateConditions(conds, evaluated)
	targets := filterRows(rows, conds, n.getMatchType(evaluated))
	if len(targets) == 0 {
		return []model.DataItem{{JSON: map[string]interface{}{"updatedCount": 0}}}, nil
	}
	for _, r := range targets {
		for k, v := range patch {
			r.Data[k] = v
		}
		if err := n.saveRow(tableID, r); err != nil {
			return nil, n.CreateError(err.Error(), nil)
		}
	}
	return []model.DataItem{{JSON: map[string]interface{}{
		"updatedCount": len(targets),
		"updatedIds":   idList(targets),
	}}}, nil
}

// doUpsert — update if a row matches the match-column; insert
// otherwise. The match column is `columnToMatchOn` (default:
// first key of `dataToSend`).
func (n *DataTableNode) doUpsert(tableID string, evaluated map[string]interface{}) ([]model.DataItem, error) {
	patch, err := extractDataToSend(evaluated["dataToSend"], nil)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	if len(patch) == 0 {
		return nil, n.CreateError("upsert requires non-empty `dataToSend`", nil)
	}
	col := strings.TrimSpace(n.GetStringParameter(evaluated, "columnToMatchOn", ""))
	if col == "" {
		for k := range patch {
			col = k
			break
		}
	}
	if col == "" {
		return nil, n.CreateError("upsert: cannot infer `columnToMatchOn` from an empty dataToSend", nil)
	}
	want, has := patch[col]
	if !has {
		return nil, n.CreateError(fmt.Sprintf("upsert: dataToSend is missing the match column %q", col), nil)
	}

	rows, err := n.loadTable(tableID)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	for _, r := range rows {
		if reflectEqual(r.Data[col], want) {
			for k, v := range patch {
				r.Data[k] = v
			}
			if err := n.saveRow(tableID, r); err != nil {
				return nil, n.CreateError(err.Error(), nil)
			}
			return []model.DataItem{{JSON: map[string]interface{}{
				"action":   "updated",
				"id":       r.ID,
				"row":      r.Data,
				"matched":  true,
			}}}, nil
		}
	}
	r := &row{ID: uuid.NewString(), Data: patch}
	if err := n.saveRow(tableID, r); err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	return []model.DataItem{{JSON: map[string]interface{}{
		"action":      "inserted",
		"id":          r.ID,
		"row":         r.Data,
		"insertedId":  r.ID,
		"matched":     false,
	}}}, nil
}

// doDelete — remove all rows matching the filter. Refuses to
// run with no filter (would wipe the table).
func (n *DataTableNode) doDelete(tableID string, evaluated map[string]interface{}) ([]model.DataItem, error) {
	conds, _, err := parseFilter(evaluated["filters"])
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	conds = n.evaluateConditions(conds, evaluated)
	if len(conds) == 0 {
		return nil, n.CreateError("delete requires at least one filter condition (refusing to wipe the table)", nil)
	}
	rows, err := n.loadTable(tableID)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	targets := filterRows(rows, conds, n.getMatchType(evaluated))
	if len(targets) == 0 {
		return []model.DataItem{{JSON: map[string]interface{}{"deletedCount": 0}}}, nil
	}
	for _, r := range targets {
		if err := n.store.DeleteRaw(dataTableKey(tableID, r.ID)); err != nil {
			return nil, n.CreateError(fmt.Sprintf("delete %s: %v", r.ID, err), nil)
		}
	}
	return []model.DataItem{{JSON: map[string]interface{}{
		"deletedCount": len(targets),
		"deletedIds":   idList(targets),
	}}}, nil
}

func idList(rows []*row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// extractDataToSend reads the `dataToSend` parameter (which is
// either a JSON string or a map) and returns a plain map.
// `inputFallback` is used by `insert` so a workflow that flows
// data into the node can omit `dataToSend` and use the inbound
// item directly.
func extractDataToSend(raw interface{}, inputFallback []model.DataItem) (map[string]interface{}, error) {
	if raw != nil {
		switch t := raw.(type) {
		case string:
			s := strings.TrimSpace(t)
			if s != "" && s != "{}" {
				var m map[string]interface{}
				if err := json.Unmarshal([]byte(s), &m); err != nil {
					return nil, fmt.Errorf("dataToSend: %w", err)
				}
				return m, nil
			}
		case map[string]interface{}:
			return t, nil
		}
	}
	if len(inputFallback) > 0 {
		return inputFallback[0].JSON, nil
	}
	return nil, fmt.Errorf("dataToSend (or input data) is required")
}

// ValidateParameters mirrors the new surface: dataTableId is
// always required; the operation must be one of the five
// supported verbs.
func (n *DataTableNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return n.CreateError("parameters cannot be nil", nil)
	}
	if strings.TrimSpace(n.GetStringParameter(params, "dataTableId", "")) == "" {
		return n.CreateError("dataTableId is required", nil)
	}
	operation := n.GetStringParameter(params, "operation", "get")
	switch operation {
	case "get", "insert", "update", "upsert", "delete":
	default:
		return n.CreateError(fmt.Sprintf("invalid operation: %s", operation), nil)
	}
	return nil
}
