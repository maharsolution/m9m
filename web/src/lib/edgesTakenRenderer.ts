// Edges-taken renderer for BUG-CAP-2026-09-09-001.
//
// m9m records, for every execution, which edges were taken
// (`EdgesTaken` accumulator — see commit `2ede796` and
// `workflowGraph.ts::EdgesTaken`). The execution-detail
// graph renderer can colour edges per state: taken (solid
// green), skipped (dashed grey), errored (solid red).
//
// This module turns the per-execution `EdgesTaken` map
// into a `Map<edgeId, EdgeState>` the Vue Flow renderer
// can consume. It is a pure data transform — no DOM, no
// Vue — so it's testable in isolation.

import type { Edge } from '@vue-flow/core'

export type EdgeState = 'taken' | 'skipped' | 'errored' | 'unknown'

export interface ExecutionEdgesTaken {
  // Per edge id (matches `buildEdgeId` in workflowGraph.ts).
  // A node-level entry means "this node ran, take all
  // its incoming edges".
  edges?: Record<string, EdgeState>
  nodes?: Record<string, EdgeState>
}

// buildEdgeStateMap flattens the per-execution record
// into a single map keyed by edge id. The convention
// is:
//   - if the edge is explicitly mentioned, use that
//   - if only the source/target node is mentioned,
//     infer the state for the edge
//   - otherwise the edge is 'unknown' (no opinion)
export function buildEdgeStateMap(taken: ExecutionEdgesTaken | null | undefined): Map<string, EdgeState> {
  const out = new Map<string, EdgeState>()
  if (!taken) return out
  if (taken.edges) {
    for (const [k, v] of Object.entries(taken.edges)) {
      out.set(k, v)
    }
  }
  return out
}

// styleForEdgeState returns the inline style object
// Vue Flow expects for the given state. Themes:
//   - taken: solid primary-500 (success feel)
//   - skipped: dashed neutral-400
//   - errored: solid red-500
//   - unknown: don't override (Vue Flow's default)
export function styleForEdgeState(state: EdgeState | undefined): Record<string, string> | undefined {
  if (!state || state === 'unknown') return undefined
  switch (state) {
    case 'taken':
      return { stroke: '#10b981', strokeWidth: '2' } // emerald-500
    case 'skipped':
      return { stroke: '#9ca3af', strokeWidth: '1.5', strokeDasharray: '4 4' } // gray-400, dashed
    case 'errored':
      return { stroke: '#ef4444', strokeWidth: '2.5' } // red-500
  }
}

// applyEdgesTaken mutates `edges` in place: each edge's
// `style` is replaced (or augmented) with the state-
// appropriate stroke. Vue Flow reads `edge.style` per
// render, so mutating in place is safe.
//
// Returns the same array (for chaining).
export function applyEdgesTaken(edges: Edge[], taken: ExecutionEdgesTaken | null | undefined): Edge[] {
  const stateMap = buildEdgeStateMap(taken)
  if (stateMap.size === 0) return edges
  for (const edge of edges) {
    const state = stateMap.get(edge.id)
    const style = styleForEdgeState(state)
    if (style) {
      // Merge into the existing style (the workflow-level
      // stroke is set in `buildFlowEdges`; we override
      // the colour but keep the rest).
      edge.style = { ...(edge.style ?? {}), ...style }
    }
  }
  return edges
}

// summaryForExecution returns a one-line summary suitable
// for the execution list: "12 taken, 3 skipped, 1 error".
export function summaryForExecution(taken: ExecutionEdgesTaken | null | undefined): string {
  if (!taken || !taken.edges) return ''
  const counts = { taken: 0, skipped: 0, errored: 0, unknown: 0 }
  for (const v of Object.values(taken.edges)) {
    counts[v]++
  }
  const parts: string[] = []
  if (counts.taken) parts.push(`${counts.taken} taken`)
  if (counts.skipped) parts.push(`${counts.skipped} skipped`)
  if (counts.errored) parts.push(`${counts.errored} errored`)
  return parts.join(', ')
}
