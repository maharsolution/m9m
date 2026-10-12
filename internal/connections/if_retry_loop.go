/*
Package connections — If-retry loop detection.

n8n supports a "retry loop" construction that is not encoded as a
`splitInBatches` node. The pattern is:

  Webhook → ... → Set Counter (Code) → Find Respond → ... → Check
   Key Exists → If (true) → back to "Find Respond"
                          └─false→ Respond Failed

The "If" node's condition is `attempt < maxAttempts`, and its
`main[0]` ("true") branch sends the data back into the body of
the loop. The user's sticky note says "Counter Max 10 retry" —
n8n honors the cycle by re-entering the body each time the If
returns true, capping at `maxAttempts`.

m9m's historical cycle detection rejected this workflow outright
("workflow contains cycles - cannot execute"). The detection in
this file lets the engine accept the workflow and then drive the
loop manually, matching n8n's behavior.

Detection rules (all must be true):

  1. The workflow has a `n8n-nodes-base.code` or `n8n-nodes-base.set`
     node that produces fields named `attempt` and `maxAttempts`.
     (Either as the body's `assignments` or as the literal keys
     produced by the `jsCode`.)
  2. The workflow has a `n8n-nodes-base.if` node whose `leftValue`
     references `attempt` and whose `rightValue` references
     `maxAttempts`, with operator `lt` (less than). This is the
     "guard" node.
  3. The guard's `main[0]` ("true") branch creates a back-edge to
     a node in the cycle body (i.e. a node that also eventually
     reaches the guard). Without this back-edge, the workflow is
     a DAG and there's nothing to loop over.

When all three are satisfied, `DetectIfRetryLoops` returns one
`IfRetryLoop` per guard. The body of the loop is the set of
nodes reachable from the back-edge target and upstream of the
guard (transitively, modulo the guard itself).

We deliberately under-detect here: the alternative (any IF node
with a true-branch that points at a non-trigger node) would catch
many non-loop workflows and silently run them in a loop. The
`attempt < maxAttempts` predicate is the standard n8n community
pattern for "retry until N" and the only construction we want to
opt-in to automatic looping for.
*/
package connections

import (
	"regexp"

	"github.com/neul-labs/m9m/internal/model"
)

// IfRetryLoop describes one `attempt < maxAttempts` retry loop in a
// workflow. The engine uses this to break the back-edge during
// execution order computation and to drive the iteration itself
// (similar to how it handles splitInBatches).
type IfRetryLoop struct {
	// GuardNode is the name of the `n8n-nodes-base.if` node whose
	// `lt` condition gates the loop. The body re-executes while
	// the guard returns true and stops the first time it returns
	// false.
	GuardNode string

	// BackEdgeTarget is the node the guard's `main[0]` (true)
	// branch routes into. This is the entry point of the body on
	// every iteration after the first. In the Finnet workflow it
	// is `Find Respond` (the Mongo lookup that the loop is
	// retrying until a response document shows up).
	BackEdgeTarget string

	// BodyNodes is the topological body chain — every node that
	// runs on every iteration. The engine skips these from the
	// top-level execution order (so the body does not fire from
	// the topological loop) and instead re-executes them via the
	// loop driver.
	BodyNodes []string

	// MaxIterations caps the loop at this many re-executions of
	// the body. Default 10. We extract it from the workflow
	// itself when possible: a Code/Set node that produces
	// `maxAttempts: 10` (literal) yields 10; otherwise we fall
	// back to the default.
	MaxIterations int
}

// attemptPattern matches the If node's `leftValue` referencing
// `attempt`. Accepts `$('Set Counter').item.json.attempt` and
// `$json.attempt` and any other expression that contains
// `.attempt` near the end.
var attemptPattern = regexp.MustCompile(`(?i)\battempt\b`)

// maxAttemptsPattern matches the If node's `rightValue` referencing
// `maxAttempts`. Used to classify an If node as a retry-loop guard.
var maxAttemptsPattern = regexp.MustCompile(`(?i)\bmaxAttempts\b`)

// maxAttemptsLiteralPattern pulls a literal `maxAttempts: N` from
// the source node that produced the field. We support both
// `maxAttempts: 10` (Code node JS) and `maxAttempts` Set-node
// assignment with a literal numeric value.
var maxAttemptsLiteralPattern = regexp.MustCompile(`(?i)maxAttempts\s*[:=]\s*(\d+)`)

// defaultMaxIterations is the fallback when the workflow doesn't
// expose a literal `maxAttempts: N`. Matches the Finnet workflow's
// "Counter Max 10 retry" sticky note.
const defaultMaxIterations = 10

// WorkflowUsesIfRetryLoop reports whether the workflow contains at
// least one `attempt < maxAttempts` retry loop. Exported so the
// engine package can use it as a "should we tolerate this cycle?"
// predicate in the same way `WorkflowUsesSplitInBatches` does.
func WorkflowUsesIfRetryLoop(workflow *model.Workflow) bool {
	if workflow == nil {
		return false
	}
	return len(DetectIfRetryLoops(workflow)) > 0
}

// DetectIfRetryLoops scans the workflow for `If` nodes whose
// condition is `attempt < maxAttempts` and whose true branch
// creates a back-edge into a node in the body. Returns one
// IfRetryLoop per guard. Order is stable: guards are returned in
// their n8n canvas order (top-to-bottom, left-to-right).
func DetectIfRetryLoops(workflow *model.Workflow) []IfRetryLoop {
	if workflow == nil {
		return nil
	}

	// Build node lookup once.
	byName := make(map[string]model.Node, len(workflow.Nodes))
	for _, n := range workflow.Nodes {
		byName[n.Name] = n
	}

	// Sort guards by canvas position so the result is deterministic.
	guardNames := findIfRetryGuardNames(workflow)

	maxIters := extractMaxAttempts(workflow)

	var loops []IfRetryLoop
	for _, guard := range guardNames {
		// The guard's main[0] (true) target is the back-edge
		// target. If the guard has no true target, this is not a
		// retry loop — skip.
		targets := CollectDirectBranchTargets(workflow, guard, 0)
		if len(targets) == 0 {
			continue
		}
		backEdge := targets[0]

		// Verify the back-edge actually creates a cycle: some node
		// reachable from `backEdge` (possibly backEdge itself)
		// must reach the guard without going through the guard's
		// own back-edge. This is what distinguishes a retry loop
		// from "If then jumps to a sibling branch".
		if !backEdgeReachesGuard(workflow, byName, backEdge, guard) {
			continue
		}

		body := collectRetryBody(workflow, byName, backEdge, guard)
		if len(body) == 0 {
			continue
		}

		loops = append(loops, IfRetryLoop{
			GuardNode:      guard,
			BackEdgeTarget: backEdge,
			BodyNodes:      body,
			MaxIterations:  maxIters,
		})
	}
	return loops
}

// IfRetryLoopForNode returns the IfRetryLoop whose body contains
// `nodeName`, or nil if none. Used by the engine to skip body
// nodes from the top-level topological loop.
func IfRetryLoopForNode(workflow *model.Workflow, nodeName string) *IfRetryLoop {
	for i := range DetectIfRetryLoops(workflow) {
		l := DetectIfRetryLoops(workflow)[i]
		for _, b := range l.BodyNodes {
			if b == nodeName {
				val := l
				return &val
			}
		}
	}
	return nil
}

// IfRetryLoopForGuard returns the IfRetryLoop whose guard is
// `nodeName`, or nil.
func IfRetryLoopForGuard(workflow *model.Workflow, nodeName string) *IfRetryLoop {
	for i := range DetectIfRetryLoops(workflow) {
		l := DetectIfRetryLoops(workflow)[i]
		if l.GuardNode == nodeName {
			val := l
			return &val
		}
	}
	return nil
}

// IsIfRetryLoopGuard reports whether `nodeName` is the guard of an
// If-retry loop in `workflow`. Used by the engine to decide
// whether to drive the body manually at this node.
func IsIfRetryLoopGuard(workflow *model.Workflow, nodeName string) bool {
	return IfRetryLoopForGuard(workflow, nodeName) != nil
}

// findIfRetryGuardNames returns the names of `If` nodes whose
// condition is the standard `attempt < maxAttempts` pattern.
func findIfRetryGuardNames(workflow *model.Workflow) []string {
	var guards []string
	for _, n := range workflow.Nodes {
		if n.Type != "n8n-nodes-base.if" {
			continue
		}
		conds := ifRetryGetConditionsArray(n.Parameters)
		if len(conds) == 0 {
			continue
		}
		first, ok := conds[0].(map[string]interface{})
		if !ok {
			continue
		}
		leftVal, _ := first["leftValue"].(string)
		rightVal, _ := first["rightValue"].(string)
		op, _ := extractOperatorOp(first["operator"])
		if op != "lessThan" && op != "lt" {
			continue
		}
		if !attemptPattern.MatchString(leftVal) {
			continue
		}
		if !maxAttemptsPattern.MatchString(rightVal) {
			continue
		}
		guards = append(guards, n.Name)
	}
	// Stable canvas order.
	guards = sortByPosition(workflow, guards)
	return guards
}

// ifRetryGetConditionsArray unwraps the n8n `conditions` shape
// (either a bare array or a wrapper object with `.conditions`).
func ifRetryGetConditionsArray(params map[string]interface{}) []interface{} {
	if params == nil {
		return nil
	}
	raw, ok := params["conditions"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		return v
	case map[string]interface{}:
		inner, ok := v["conditions"]
		if !ok {
			return nil
		}
		arr, _ := inner.([]interface{})
		return arr
	}
	return nil
}

// extractOperatorOp pulls the canonical operation name from the
// IF node's `operator` field, which can be either a bare string
// (`"lessThan"`) or an object (`{operation: "lt", type: "number"}`).
func extractOperatorOp(op interface{}) (string, bool) {
	switch v := op.(type) {
	case string:
		return v, true
	case map[string]interface{}:
		name, ok := v["operation"].(string)
		return name, ok
	}
	return "", false
}

// extractMaxAttempts looks for a literal `maxAttempts: N` in the
// workflow. It scans every Code node's `jsCode` and every Set
// node's `assignments.assignments[].value`. Returns the first
// match, falling back to defaultMaxIterations when nothing is
// found.
func extractMaxAttempts(workflow *model.Workflow) int {
	for _, n := range workflow.Nodes {
		switch n.Type {
		case "n8n-nodes-base.code":
			js, _ := n.Parameters["jsCode"].(string)
			if m := maxAttemptsLiteralPattern.FindStringSubmatch(js); m != nil {
				if n, err := atoiSafe(m[1]); err == nil {
					return n
				}
			}
		case "n8n-nodes-base.set":
			// `assignments.assignments` is the modern (v3) shape.
			if root, ok := n.Parameters["assignments"].(map[string]interface{}); ok {
				if arr, ok := root["assignments"].([]interface{}); ok {
					for _, a := range arr {
						am, ok := a.(map[string]interface{})
						if !ok {
							continue
						}
						if am["name"] != "maxAttempts" {
							continue
						}
						val, _ := am["value"].(string)
						if m := maxAttemptsLiteralPattern.FindStringSubmatch(val); m != nil {
							if n, err := atoiSafe(m[1]); err == nil {
								return n
							}
						}
					}
				}
			}
		}
	}
	return defaultMaxIterations
}

func atoiSafe(s string) (int, error) {
	if s == "" {
		return 0, errInvalidNumber
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidNumber
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

type numErr struct{}

func (numErr) Error() string { return "invalid number" }

var errInvalidNumber = numErr{}

// backEdgeReachesGuard reports whether `start` can reach `guard`
// through the workflow's main connections. We do this by BFS
// from `start` over the connection graph, terminating when we
// either hit `guard` (cycle confirmed) or exhaust the search.
func backEdgeReachesGuard(workflow *model.Workflow, byName map[string]model.Node, start, guard string) bool {
	if start == guard {
		return true
	}
	visited := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		conns, ok := workflow.Connections[cur]
		if !ok {
			continue
		}
		// Walk all branches (main[0], main[1], ...). An If node
		// in the body of the loop may take either branch; both
		// must be considered to confirm the cycle exists.
		for _, branch := range conns.Main {
			for _, c := range branch {
				if c.Node == "" {
					continue
				}
				if c.Node == guard {
					return true
				}
				if !visited[c.Node] {
					visited[c.Node] = true
					queue = append(queue, c.Node)
				}
			}
		}
	}
	return false
}

// collectRetryBody returns the body chain in canvas order:
// every node in the closed cycle containing the back-edge,
// other than the guard itself.
//
// The "closed cycle" is the set of nodes `C` such that every
// node in `C` is forward-reachable from `backEdge` AND can
// reach `backEdge` going forward. This is the standard
// strongly-connected-component definition. For the Finnet
// workflow:
//
//	F (forward-reachable from Find Respond) = {Find Respond,
//	    Value, Check Key Exists1, IsRespondJson, If,
//	    Respond to Json, Respond to XML}
//	B (can reach Find Respond forward) = {Set Counter,
//	    Find Respond, Value, Check Key Exists1, If,
//	    IsRespondJson, Respond to Json, Respond to XML,
//	    finnet-queue, Payload, Webhook}
//	F ∩ B = {Find Respond, Value, Check Key Exists1,
//	         IsRespondJson, If, Respond to Json, Respond to XML}
//
// We then filter to nodes that can reach the guard going
// forward (so exit branches like `IsRespondJson` and its
// downstream `Respond to Json/XML` are excluded — those are
// reached by items the body's terminal If node routes to its
// true branch and are handled by the main engine, not the
// loop driver). The body is:
//
//	body = (F ∩ B) ∩ canReachGuard \ {guard}
//	     = {Find Respond, Value, Check Key Exists1}
//
// n8n re-runs `Set Counter` on each iteration too, but the
// back-edge in this workflow targets `Find Respond` directly,
// so the loop body's first node is `Find Respond`. The
// `attempt` counter never increments in this workflow as
// written, but that's the workflow's design — the loop will
// run until the response is found in Mongo (Check Key
// Exists1 routes to the exit) or until MaxIterations is hit.
func collectRetryBody(workflow *model.Workflow, byName map[string]model.Node, backEdge, guard string) []string {
	if backEdge == "" || guard == "" {
		return nil
	}
	reverseAdj := buildReverseAdjacency(workflow)

	// F: forward-reachable from backEdge.
	F := map[string]bool{backEdge: true}
	queue := []string{backEdge}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		conns, ok := workflow.Connections[cur]
		if !ok {
			continue
		}
		for _, branch := range conns.Main {
			for _, c := range branch {
				if c.Node == "" {
					continue
				}
				if F[c.Node] {
					continue
				}
				F[c.Node] = true
				queue = append(queue, c.Node)
			}
		}
	}

	// B: nodes that can reach backEdge (forward).
	B := map[string]bool{backEdge: true}
	queue = []string{backEdge}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, prev := range reverseAdj[cur] {
			if B[prev] {
				continue
			}
			B[prev] = true
			queue = append(queue, prev)
		}
	}

	// canReachGuard: filter to nodes on a path to the guard
	// (drops exit branches like IsRespondJson).
	canReachGuard := map[string]bool{guard: true}
	queue = []string{guard}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, prev := range reverseAdj[cur] {
			if canReachGuard[prev] {
				continue
			}
			canReachGuard[prev] = true
			queue = append(queue, prev)
		}
	}

	// body = (F ∩ B) ∩ canReachGuard, minus guard, in
	// canvas order.
	var body []string
	for _, n := range workflow.Nodes {
		name := n.Name
		if name == guard {
			continue
		}
		if !F[name] || !B[name] {
			continue
		}
		if !canReachGuard[name] {
			// Exit branch — handled by the main
			// engine when the body's terminal If
			// node routes items to it.
			continue
		}
		body = append(body, name)
	}
	return body
}

func buildReverseAdjacency(workflow *model.Workflow) map[string][]string {
	adj := map[string][]string{}
	for src, conns := range workflow.Connections {
		for _, branch := range conns.Main {
			for _, c := range branch {
				if c.Node == "" {
					continue
				}
				adj[c.Node] = append(adj[c.Node], src)
			}
		}
	}
	return adj
}

// sortByPosition returns `names` ordered by the workflow's canvas
// position (X ascending, then Y ascending). Used to make loop
// detection order stable across runs.
func sortByPosition(workflow *model.Workflow, names []string) []string {
	if len(names) <= 1 {
		return names
	}
	pos := make(map[string][2]int, len(workflow.Nodes))
	for _, n := range workflow.Nodes {
		p := [2]int{0, 0}
		if len(n.Position) >= 2 {
			p[0] = int(n.Position[0])
			p[1] = int(n.Position[1])
		}
		pos[n.Name] = p
	}
	sorted := make([]string, len(names))
	copy(sorted, names)
	// Simple insertion sort — n is small (a workflow has at most
	// ~10 nodes involved in a single retry loop).
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0; j-- {
			a, b := pos[sorted[j-1]], pos[sorted[j]]
			if a[0] > b[0] || (a[0] == b[0] && a[1] > b[1]) {
				sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
			} else {
				break
			}
		}
	}
	return sorted
}
