// Tests for BUG-CAP-2026-09-09-001 — EdgesTaken renderer.

import { describe, it, expect } from 'vitest'
import {
  buildEdgeStateMap,
  styleForEdgeState,
  applyEdgesTaken,
  summaryForExecution,
} from '../src/lib/edgesTakenRenderer'

describe('buildEdgeStateMap', () => {
  it('returns empty map for null', () => {
    expect(buildEdgeStateMap(null).size).toBe(0)
  })
  it('returns empty map for undefined', () => {
    expect(buildEdgeStateMap(undefined).size).toBe(0)
  })
  it('returns empty map for empty input', () => {
    expect(buildEdgeStateMap({}).size).toBe(0)
  })
  it('flattens the per-edge entries', () => {
    const m = buildEdgeStateMap({ edges: { e1: 'taken', e2: 'errored' } })
    expect(m.get('e1')).toBe('taken')
    expect(m.get('e2')).toBe('errored')
  })
})

describe('styleForEdgeState', () => {
  it('returns undefined for unknown', () => {
    expect(styleForEdgeState('unknown')).toBeUndefined()
  })
  it('returns undefined for undefined', () => {
    expect(styleForEdgeState(undefined)).toBeUndefined()
  })
  it('returns a stroke for taken', () => {
    const s = styleForEdgeState('taken')
    expect(s).toBeDefined()
    expect(s.stroke).toMatch(/^#/)
  })
  it('returns a dashed stroke for skipped', () => {
    const s = styleForEdgeState('skipped')
    expect(s.strokeDasharray).toBeTruthy()
  })
  it('returns a heavier stroke for errored', () => {
    const s = styleForEdgeState('errored')
    expect(parseFloat(s.strokeWidth)).toBeGreaterThan(1)
  })
})

describe('applyEdgesTaken', () => {
  it('does not mutate when no taken', () => {
    const edges = [{ id: 'e1', style: { stroke: '#abc' } }, { id: 'e2' }]
    const result = applyEdgesTaken(edges, null)
    expect(result).toBe(edges)
    expect(edges[0].style.stroke).toBe('#abc')
  })
  it('overrides the stroke for a known edge', () => {
    const edges = [{ id: 'e1', style: { stroke: '#abc' } }]
    applyEdgesTaken(edges, { edges: { e1: 'taken' } })
    expect(edges[0].style.stroke).toMatch(/^#/)
    expect(edges[0].style.stroke).not.toBe('#abc')
  })
  it('leaves unknown edges alone', () => {
    const edges = [{ id: 'e1', style: { stroke: '#abc' } }]
    applyEdgesTaken(edges, { edges: { e2: 'taken' } })
    expect(edges[0].style.stroke).toBe('#abc')
  })
  it('returns the same array reference for chaining', () => {
    const edges = [{ id: 'e1' }]
    expect(applyEdgesTaken(edges, null)).toBe(edges)
  })
})

describe('summaryForExecution', () => {
  it('returns empty for null', () => {
    expect(summaryForExecution(null)).toBe('')
  })
  it('returns empty for empty', () => {
    expect(summaryForExecution({})).toBe('')
  })
  it('returns empty when no edges', () => {
    expect(summaryForExecution({ edges: {} })).toBe('')
  })
  it('formats counts', () => {
    expect(
      summaryForExecution({ edges: { a: 'taken', b: 'taken', c: 'skipped' } })
    ).toBe('2 taken, 1 skipped')
  })
  it('omits zero buckets', () => {
    expect(summaryForExecution({ edges: { a: 'taken' } })).toBe('1 taken')
  })
  it('formats errored', () => {
    expect(summaryForExecution({ edges: { a: 'errored' } })).toBe('1 errored')
  })
})
