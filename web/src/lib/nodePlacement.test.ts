import { describe, expect, it } from 'vitest'
import {
  centerOfRect,
  freeNodePosition,
  isSpotTaken,
  uniqueNodeName,
  NODE_SPACING_X,
} from './nodePlacement'
import type { WorkflowNode } from '@/types'

/**
 * A node that is clicked in the node creator is added to the canvas
 * (n8n parity). The node needs a name that is unique inside the
 * workflow — the name is the key of the connections map.
 */
describe('uniqueNodeName', () => {
  it('keeps the bare display name for the first instance', () => {
    expect(uniqueNodeName('HTTP Request', [])).toBe('HTTP Request')
  })

  it('numbers duplicates without a space', () => {
    expect(uniqueNodeName('HTTP Request', ['HTTP Request'])).toBe('HTTP Request1')
    expect(uniqueNodeName('HTTP Request', ['HTTP Request', 'HTTP Request1'])).toBe(
      'HTTP Request2'
    )
  })

  it('fills the first free number', () => {
    expect(uniqueNodeName('Code', ['Code', 'Code2'])).toBe('Code1')
  })

  it('accepts a set of taken names', () => {
    expect(uniqueNodeName('Code', new Set(['Code']))).toBe('Code1')
  })
})

/**
 * A node added by clicking lands in the middle of the visible canvas,
 * so it is always visible.
 */
describe('centerOfRect', () => {
  it('returns the centre of the rectangle', () => {
    expect(centerOfRect({ left: 100, top: 50, width: 400, height: 200 })).toEqual({
      x: 300,
      y: 150,
    })
  })

  it('handles a zero-sized rectangle', () => {
    expect(centerOfRect({ left: 10, top: 20, width: 0, height: 0 })).toEqual({
      x: 10,
      y: 20,
    })
  })
})

const nodeAt = (x: number, y: number): WorkflowNode => ({
  id: `node_${x}_${y}`,
  name: `Node ${x}-${y}`,
  type: 'n8n-nodes-base.webhook',
  typeVersion: 1,
  position: [x, y],
  parameters: {},
})

describe('freeNodePosition', () => {
  it('keeps the centre of the canvas on an empty canvas', () => {
    expect(freeNodePosition([100, 100], [])).toEqual([100, 100])
  })

  it('steps to the right when the centre is taken', () => {
    const nodes = [nodeAt(100, 100)]

    expect(freeNodePosition([100, 100], nodes)).toEqual([100 + NODE_SPACING_X, 100])
  })

  it('finds the first free spot', () => {
    const nodes = [nodeAt(100, 100), nodeAt(100 + NODE_SPACING_X, 100)]

    expect(freeNodePosition([100, 100], nodes)).toEqual([
      100 + 2 * NODE_SPACING_X,
      100,
    ])
  })

  it('keeps a spot that is far enough away', () => {
    const nodes = [nodeAt(1000, 1000)]

    expect(isSpotTaken([100, 100], nodes)).toBe(false)
    expect(freeNodePosition([100, 100], nodes)).toEqual([100, 100])
  })
})
