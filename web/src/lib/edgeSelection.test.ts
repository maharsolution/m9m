import { describe, expect, it } from 'vitest'
import {
  isMultiSelectModifier,
  planDeletion,
  resolveEdgeConnections,
  toggleEdgeSelection,
} from './edgeSelection'

/** The id Vue Flow gives to the connection of the test workflow. */
const EDGE_ID = 'workflow-edge:node_webhook:0:node_set:0'

describe('edge selection — helpers', () => {
  it('treats shift, ctrl and meta as the multi-select modifier', () => {
    expect(isMultiSelectModifier({ shiftKey: true, ctrlKey: false, metaKey: false })).toBe(true)
    expect(isMultiSelectModifier({ shiftKey: false, ctrlKey: true, metaKey: false })).toBe(true)
    expect(isMultiSelectModifier({ shiftKey: false, ctrlKey: false, metaKey: true })).toBe(true)
    expect(isMultiSelectModifier({ shiftKey: false, ctrlKey: false, metaKey: false })).toBe(false)
  })

  it('replaces the selection on a plain click', () => {
    expect(toggleEdgeSelection(['a', 'b'], 'c', false)).toEqual(['c'])
  })

  it('adds a connection to the selection on a modified click', () => {
    expect(toggleEdgeSelection(['a'], 'b', true)).toEqual(['a', 'b'])
  })

  it('removes a connection from the selection when it is clicked twice', () => {
    expect(toggleEdgeSelection(['a', 'b'], 'b', true)).toEqual(['a'])
  })

  it('resolves an edge id into the connection it stands for', () => {
    expect(resolveEdgeConnections([EDGE_ID])).toEqual([
      {
        sourceNodeId: 'node_webhook',
        sourceOutput: 0,
        targetNodeId: 'node_set',
        targetInput: 0,
      },
    ])
  })

  it('ignores ids that are no connection', () => {
    expect(resolveEdgeConnections(['not-an-edge'])).toEqual([])
  })

  /**
   * n8n deletes every selected element: the nodes *and* the
   * connections that were picked.
   */
  it('plans the deletion of the nodes and of the connections', () => {
    const plan = planDeletion(['node_webhook'], [EDGE_ID])

    expect(plan.nodeIds).toEqual(['node_webhook'])
    expect(plan.connections).toHaveLength(1)
    expect(plan.connections[0].targetNodeId).toBe('node_set')
  })

  it('plans the deletion of a connection alone', () => {
    const plan = planDeletion([], [EDGE_ID])

    expect(plan.nodeIds).toEqual([])
    expect(plan.connections).toHaveLength(1)
  })

  it('lists a node only once', () => {
    expect(planDeletion(['a', 'a'], []).nodeIds).toEqual(['a'])
  })
})
