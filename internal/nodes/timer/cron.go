/*
Package timer provides timer and trigger node implementations for m9m.
*/
package timer

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// CronNode implements the Cron node functionality
type CronNode struct {
	*base.BaseNode
}

// NewCronNode creates a new Cron node
func NewCronNode() *CronNode {
	description := base.NodeDescription{
		Name:        "Cron",
		Description: "Triggers the workflow on a schedule using a standard five-field cron expression.",
		Category:    "Triggers",
		Group:       []string{"trigger"},
		Properties:  cronProperties(),
		Inputs:      []string{},
		Outputs:     []string{"main"},
	}

	return &CronNode{
		BaseNode: base.NewBaseNode(description),
	}
}

// cronProperties returns the property descriptors for the Cron
// trigger. The executor reads a single parameter — `cronExpression` —
// so the panel exposes exactly that field (the previous `rule.*`
// names were never read by Execute and silently did nothing).
func cronProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("Cron expression", "cronExpression", "*/5 * * * *",
			"Cron expression that decides when the workflow runs: minute hour day-of-month month day-of-week. Supports @daily style descriptors.",
			"*/5 * * * *", true),
	}
	return append(props, nodeprops.Settings()...)
}

// Description returns the node description
func (c *CronNode) Description() base.NodeDescription {
	return c.BaseNode.Description()
}

// ValidateParameters validates Cron node parameters
func (c *CronNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return c.CreateError("parameters cannot be nil", nil)
	}

	cronExpression := c.GetStringParameter(params, "cronExpression", "")
	if cronExpression == "" {
		return c.CreateError("cronExpression is required", nil)
	}

	// Validate the cron expression using robfig/cron
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(cronExpression); err != nil {
		return c.CreateError(fmt.Sprintf("invalid cron expression %q: %v", cronExpression, err), nil)
	}

	return nil
}

// Execute processes the Cron node operation
func (c *CronNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	cronExpression := c.GetStringParameter(nodeParams, "cronExpression", "")

	triggerData := model.DataItem{
		JSON: map[string]interface{}{
			"cronExpression": cronExpression,
			"triggeredAt":    time.Now().UTC().Format(time.RFC3339),
			"trigger":        true,
		},
	}

	return []model.DataItem{triggerData}, nil
}

// ParseCronExpression parses a cron expression and returns the next execution time.
func (c *CronNode) ParseCronExpression(cronExpression string) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(cronExpression)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return schedule.Next(time.Now()), nil
}
