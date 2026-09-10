package database

import (
	"fmt"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
)

// resolveQueryTemplate evaluates n8n-style `{{ ... }}` expressions in a
// SQL query string against the inbound data item so that parameterised
// queries like `select * from {{ $json.body.table_name }}` resolve to
// `select * from credentials` before the driver sees them.
//
// Without this, the MySQL/Postgres drivers receive the literal template
// (e.g. `{{ $json.body.table_name }} limit 10`) and fail with syntax
// errors. The HTTP node evaluates expressions in the same way via its
// internal evaluateParameters helper; the database nodes historically
// skipped that step.
func resolveQueryTemplate(template string, item model.DataItem) (string, error) {
	if template == "" {
		return template, nil
	}
	if !expressions.IsExpression(template) {
		return template, nil
	}
	execCtx := &expressions.ExecutionContext{
		InputData: []model.DataItem{item},
		ItemIndex: 0,
		Variables: make(map[string]interface{}),
	}
	evaluator := expressions.NewExpressionEvaluator()
	resolved, err := evaluator.Evaluate(template, execCtx)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate query expression %q: %w", template, err)
	}
	if s, ok := resolved.(string); ok {
		return s, nil
	}
	return fmt.Sprintf("%v", resolved), nil
}
