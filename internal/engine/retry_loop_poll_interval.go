/*
Package engine — If-retry loop poll interval.

The If-retry loop (see internal/connections/if_retry_loop.go) polls
the loop body until the reply arrives or the attempt guard trips.
n8n polls as fast as the body allows; m9m used to sleep a hard-coded
2 s between iterations, which alone accounted for ~20 s of the
`POST /webhook/post_balance` latency (10 iterations × 2 s).

The interval is resolved with this precedence:

 1. An explicit test override (SetRetryLoopPollIntervalForTesting).
 2. An explicit workflow/node setting — `waitBetweenTries`,
    `retryDelay`, `pollInterval` or `options.waitBetweenTries` on
    the guard node, the counter node or the body entry. An
    explicit setting is honoured verbatim (clamped to
    [10 ms, 5 min]) so workflows that ask for a slow poll keep
    their behaviour.
 3. The M9M_RETRY_LOOP_POLL_INTERVAL environment variable, clamped
    to [10 ms, 5 s].
 4. DefaultRetryLoopPollInterval (200 ms).

The 10 ms floor makes busy-spinning impossible.
*/
package engine

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/neul-labs/m9m/internal/connections"
	"github.com/neul-labs/m9m/internal/model"
)

const (
	// DefaultRetryLoopPollInterval is the poll cadence used when
	// neither the workflow nor the environment asks for one.
	DefaultRetryLoopPollInterval = 200 * time.Millisecond

	// MinRetryLoopPollInterval is the hard floor for any resolved
	// interval — the loop never busy-spins.
	MinRetryLoopPollInterval = 10 * time.Millisecond

	// MaxRetryLoopPollInterval caps the environment override.
	MaxRetryLoopPollInterval = 5 * time.Second

	// MaxRetryLoopConfiguredInterval caps an explicit workflow
	// setting so a typo cannot stall a webhook for hours.
	MaxRetryLoopConfiguredInterval = 5 * time.Minute

	// RetryLoopPollIntervalEnv is the environment variable that
	// overrides the default poll interval.
	RetryLoopPollIntervalEnv = "M9M_RETRY_LOOP_POLL_INTERVAL"
)

// retryLoopPollIntervalOverride holds the test override in
// nanoseconds; zero means "not set".
var retryLoopPollIntervalOverride atomic.Int64

// SetRetryLoopPollIntervalForTesting forces the retry-loop poll
// interval for the whole process. Pass 0 to clear the override.
//
// This exists so the unit tests can exercise the loop without real
// sleeps.
func SetRetryLoopPollIntervalForTesting(d time.Duration) {
	if d <= 0 {
		retryLoopPollIntervalOverride.Store(0)
		return
	}
	retryLoopPollIntervalOverride.Store(int64(d))
}

// ResetRetryLoopPollIntervalForTesting clears the test override.
func ResetRetryLoopPollIntervalForTesting() {
	retryLoopPollIntervalOverride.Store(0)
}

// resolveRetryLoopPollInterval returns the interval to wait between
// two iterations of an If-retry loop.
func resolveRetryLoopPollInterval(workflow *model.Workflow, guardNode *model.Node, loop connections.IfRetryLoop) time.Duration {
	// The in-process test override is honoured EXACTLY — no floor,
	// no ceiling. It is set by the process itself (unit tests), not
	// by a workflow or an environment variable, so it cannot make a
	// production run busy-spin. Production values (workflow
	// settings, environment) keep the >= 10 ms floor below.
	if override := retryLoopPollIntervalOverride.Load(); override > 0 {
		return time.Duration(override)
	}

	// Explicit workflow/node setting wins over the environment so a
	// workflow that asks for a specific cadence keeps it.
	if d, ok := retryLoopIntervalFromWorkflow(workflow, guardNode, loop); ok {
		return clampRetryLoopInterval(d, MinRetryLoopPollInterval, MaxRetryLoopConfiguredInterval)
	}

	if raw := strings.TrimSpace(os.Getenv(RetryLoopPollIntervalEnv)); raw != "" {
		if d, ok := parseRetryLoopInterval(raw); ok {
			return clampRetryLoopInterval(d, MinRetryLoopPollInterval, MaxRetryLoopPollInterval)
		}
		log.Printf("[m9m] invalid %s=%q — falling back to %s", RetryLoopPollIntervalEnv, raw, DefaultRetryLoopPollInterval)
	}

	return DefaultRetryLoopPollInterval
}

// retryLoopIntervalFromWorkflow looks for an explicit poll interval
// in the workflow's node parameters.
func retryLoopIntervalFromWorkflow(workflow *model.Workflow, guardNode *model.Node, loop connections.IfRetryLoop) (time.Duration, bool) {
	candidates := make([]*model.Node, 0, 3)
	if guardNode != nil {
		candidates = append(candidates, guardNode)
	}
	if workflow != nil {
		byName := make(map[string]*model.Node, len(workflow.Nodes))
		for i := range workflow.Nodes {
			byName[workflow.Nodes[i].Name] = &workflow.Nodes[i]
		}
		if loop.CounterNode != "" {
			if node := byName[loop.CounterNode]; node != nil {
				candidates = append(candidates, node)
			}
		}
		if loop.BackEdgeTarget != "" {
			if node := byName[loop.BackEdgeTarget]; node != nil {
				candidates = append(candidates, node)
			}
		}
	}

	for _, node := range candidates {
		if node == nil {
			continue
		}
		if d, ok := retryLoopIntervalFromParams(node.Parameters); ok {
			return d, true
		}
	}
	return 0, false
}

// retryLoopIntervalFromParams scans a parameter map for one of the
// known interval keys.
func retryLoopIntervalFromParams(params map[string]interface{}) (time.Duration, bool) {
	if params == nil {
		return 0, false
	}
	keys := []string{"waitBetweenTries", "retryDelay", "pollInterval", "pollingInterval"}
	for _, key := range keys {
		if raw, ok := params[key]; ok {
			if d, ok := parseRetryLoopInterval(raw); ok {
				return d, true
			}
		}
	}
	if opts, ok := params["options"].(map[string]interface{}); ok {
		for _, key := range keys {
			if raw, ok := opts[key]; ok {
				if d, ok := parseRetryLoopInterval(raw); ok {
					return d, true
				}
			}
		}
	}
	return 0, false
}

// parseRetryLoopInterval converts a parameter value into a
// duration. Strings are parsed as Go durations ("250ms", "2s");
// bare numbers are milliseconds.
func parseRetryLoopInterval(raw interface{}) (time.Duration, bool) {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		if d, err := time.ParseDuration(s); err == nil {
			return d, true
		}
		if ms, err := strconv.ParseFloat(s, 64); err == nil {
			return time.Duration(ms * float64(time.Millisecond)), true
		}
		return 0, false
	case float64:
		return time.Duration(v * float64(time.Millisecond)), true
	case float32:
		return time.Duration(float64(v) * float64(time.Millisecond)), true
	case int:
		return time.Duration(v) * time.Millisecond, true
	case int32:
		return time.Duration(v) * time.Millisecond, true
	case int64:
		return time.Duration(v) * time.Millisecond, true
	case uint:
		return time.Duration(v) * time.Millisecond, true
	case uint32:
		return time.Duration(v) * time.Millisecond, true
	case uint64:
		return time.Duration(v) * time.Millisecond, true
	}
	return 0, false
}

// clampRetryLoopInterval bounds a duration to [min, max].
func clampRetryLoopInterval(d, min, max time.Duration) time.Duration {
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}
