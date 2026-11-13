/**
 * Connection (edge) selection helpers for the workflow canvas.
 *
 * n8n parity: a connection between two nodes is a first class
 * selection. Clicking it selects it, `Delete` / `Backspace` removes
 * it, and the hover affordance removes it with a single click.
 *
 * The helpers below hold the *decision* logic — which connections a
 * delete request has to remove — without touching the DOM, so they
 * can be unit tested.
 */

import { parseEdgeId } from './workflowGraph'

/** A connection of the workflow, addressed by node id and port. */
export interface EdgeConnection {
  sourceNodeId: string
  sourceOutput: number
  targetNodeId: string
  targetInput: number
}

/** What a delete request has to remove. */
export interface DeletionPlan {
  /** Ids of the nodes to remove. */
  nodeIds: string[]
  /** The connections to remove. */
  connections: EdgeConnection[]
}

/**
 * True when the event carries a multi-select modifier.
 *
 * n8n adds an element to the selection with shift / ctrl / ⌘.
 */
export function isMultiSelectModifier(
  event: Pick<KeyboardEvent, 'shiftKey' | 'ctrlKey' | 'metaKey'>,
): boolean {
  return Boolean(event.shiftKey || event.ctrlKey || event.metaKey)
}

/**
 * Replaces or toggles the selected connection ids.
 *
 * A plain click selects exactly one connection (n8n parity), a
 * modified click adds the connection to the selection — or removes it
 * when it was already selected.
 */
export function toggleEdgeSelection(
  current: string[],
  edgeId: string,
  addToSelection: boolean,
): string[] {
  if (!addToSelection) {
    return [edgeId]
  }

  return current.includes(edgeId)
    ? current.filter((id) => id !== edgeId)
    : [...current, edgeId]
}

/**
 * Resolves the selected edge ids into the connections they stand for.
 *
 * Ids that do not belong to a connection (a stale selection, an edge
 * of another document) are ignored.
 */
export function resolveEdgeConnections(edgeIds: string[]): EdgeConnection[] {
  const connections: EdgeConnection[] = []

  edgeIds.forEach((edgeId) => {
    const parsed = parseEdgeId(edgeId)
    if (!parsed) {
      return
    }

    connections.push({
      sourceNodeId: parsed.sourceNodeId,
      sourceOutput: parsed.sourceOutput,
      targetNodeId: parsed.targetNodeId,
      targetInput: parsed.targetInput,
    })
  })

  return connections
}

/**
 * Builds the plan of a delete request.
 *
 * n8n removes *every* selected element: the selected nodes and the
 * selected connections. The plan is pure data — the store applies it.
 */
export function planDeletion(
  selectedNodeIds: string[],
  selectedEdgeIds: string[],
): DeletionPlan {
  return {
    nodeIds: [...new Set(selectedNodeIds)],
    connections: resolveEdgeConnections(selectedEdgeIds),
  }
}
