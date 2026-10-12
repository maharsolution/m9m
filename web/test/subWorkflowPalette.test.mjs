// Tests for BUG-ARCH-2026-10-12-004 — sub-workflow palette.

import { describe, it, expect } from 'vitest'
import {
  getSubWorkflowPaletteEntries,
  validateSubWorkflowRef,
  isSubWorkflowType,
  nodeTypeForSubWorkflow,
} from '../src/data/subWorkflowPalette'

describe('getSubWorkflowPaletteEntries', () => {
  it('returns the expected entries', () => {
    const entries = getSubWorkflowPaletteEntries()
    expect(entries.length).toBeGreaterThanOrEqual(2)
    const types = entries.map((e) => e.type)
    expect(types).toContain('n8n-nodes-base.executeWorkflow')
    expect(types).toContain('n8n-nodes-base.executeWorkflowTrigger')
  })

  it('every entry has a default name and parameters', () => {
    for (const e of getSubWorkflowPaletteEntries()) {
      expect(e.defaultName).toBeTruthy()
      expect(typeof e.defaultParameters).toBe('object')
    }
  })
})

describe('validateSubWorkflowRef', () => {
  const known = ['wf-1', 'wf-2', 'wf-3']

  it('rejects null/undefined', () => {
    expect(validateSubWorkflowRef(null, known, 'self')).toMatch(/Pick/)
    expect(validateSubWorkflowRef(undefined, known, 'self')).toMatch(/Pick/)
  })

  it('rejects empty value', () => {
    expect(validateSubWorkflowRef({ value: '' }, known, 'self')).toMatch(/Pick/)
  })

  it('rejects self-reference', () => {
    expect(validateSubWorkflowRef({ value: 'self' }, known, 'self')).toMatch(/itself/)
  })

  it('rejects unknown workflow', () => {
    expect(validateSubWorkflowRef({ value: 'wf-99' }, known, 'self')).toMatch(/exist/)
  })

  it('accepts a valid reference', () => {
    expect(validateSubWorkflowRef({ value: 'wf-1' }, known, 'self')).toBe('')
  })
})

describe('isSubWorkflowType', () => {
  it('matches executeWorkflow', () => {
    expect(isSubWorkflowType('n8n-nodes-base.executeWorkflow')).toBe(true)
  })
  it('matches executeWorkflowTrigger', () => {
    expect(isSubWorkflowType('n8n-nodes-base.executeWorkflowTrigger')).toBe(true)
  })
  it('does not match other types', () => {
    expect(isSubWorkflowType('n8n-nodes-base.httpRequest')).toBe(false)
    expect(isSubWorkflowType('')).toBe(false)
  })
})

describe('nodeTypeForSubWorkflow', () => {
  it('produces a NodeType with the expected fields', () => {
    const entry = getSubWorkflowPaletteEntries()[0]
    const t = nodeTypeForSubWorkflow(entry)
    expect(t.name).toBe(entry.type)
    expect(t.displayName).toBe(entry.displayName)
    expect(t.group).toContain('flow')
  })
})
