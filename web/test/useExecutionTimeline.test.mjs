// Tests for BUG-CAP-2026-09-09-004 — useExecutionTimeline.

import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import {
  buildTimeline,
  labelFor,
  filterByDateRange,
  useExecutionTimeline,
} from '../src/composables/useExecutionTimeline'

const sample = [
  { id: '1', workflowId: 'wf-1', startedAt: '2026-10-12T08:00:00Z', status: 'completed' },
  { id: '2', workflowId: 'wf-1', startedAt: '2026-10-12T10:00:00Z', status: 'failed' },
  { id: '3', workflowId: 'wf-2', startedAt: '2026-10-12T11:00:00Z', status: 'completed' },
  { id: '4', workflowId: 'wf-1', startedAt: '2026-10-11T09:00:00Z', status: 'completed' },
  { id: '5', workflowId: 'wf-1', startedAt: '2026-10-10T09:00:00Z', status: 'running' },
]

describe('buildTimeline', () => {
  it('returns empty array for empty input', () => {
    expect(buildTimeline([])).toEqual([])
  })

  it('groups executions by date', () => {
    const tl = buildTimeline(sample)
    expect(tl.length).toBe(3)
  })

  it('orders newest day first', () => {
    const tl = buildTimeline(sample)
    expect(tl[0].date).toBe('2026-10-12')
    expect(tl[2].date).toBe('2026-10-10')
  })

  it('counts by status', () => {
    const tl = buildTimeline(sample)
    const day = tl[0]
    expect(day.total).toBe(3)
    expect(day.success).toBe(2)
    expect(day.failed).toBe(1)
    expect(day.running).toBe(0)
  })

  it('computes successRate', () => {
    const tl = buildTimeline(sample)
    const day = tl[0]
    // 2 success / (2+1) failed = 0.6667
    expect(day.successRate).toBeCloseTo(2 / 3, 3)
  })

  it('handles all-running bucket', () => {
    const tl = buildTimeline([{ id: '1', workflowId: 'wf', startedAt: '2026-10-10T08:00:00Z', status: 'running' }])
    expect(tl[0].successRate).toBe(0)
  })
})

describe('labelFor', () => {
  it('returns "Today" for today', () => {
    const today = new Date().toISOString().slice(0, 10)
    expect(labelFor(today)).toBe('Today')
  })

  it('returns "Yesterday" for yesterday', () => {
    const yest = new Date(Date.now() - 86_400_000).toISOString().slice(0, 10)
    expect(labelFor(yest)).toBe('Yesterday')
  })

  it('returns a long-form date for older days', () => {
    const label = labelFor('2020-01-15')
    expect(label).toContain('2020')
    expect(label).not.toBe('Today')
    expect(label).not.toBe('Yesterday')
  })
})

describe('filterByDateRange', () => {
  it('returns all when both bounds are null', () => {
    const tl = buildTimeline(sample)
    expect(filterByDateRange(tl, null, null).length).toBe(3)
  })
  it('filters by from', () => {
    const tl = buildTimeline(sample)
    expect(filterByDateRange(tl, '2026-10-11', null).length).toBe(2)
  })
  it('filters by to', () => {
    const tl = buildTimeline(sample)
    expect(filterByDateRange(tl, null, '2026-10-11').length).toBe(2)
  })
  it('filters by both', () => {
    const tl = buildTimeline(sample)
    expect(filterByDateRange(tl, '2026-10-11', '2026-10-11').length).toBe(1)
  })
})

describe('useExecutionTimeline', () => {
  it('returns a ComputedRef that updates with the input', () => {
    const r = ref([])
    const tl = useExecutionTimeline(r)
    expect(tl.value).toEqual([])
    r.value = sample
    expect(tl.value.length).toBe(3)
  })
})
