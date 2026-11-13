import { describe, expect, it } from 'vitest'
import { isOutsidePanel, type PanelElement } from './nodeCreator'

/**
 * The node creator floats over the canvas: the scrim is visual only,
 * so the panel is closed by a document-level click guard. The guard
 * must ignore clicks inside the panel (search field, node rows) and
 * close the creator for clicks on the canvas.
 */
const panelWith = (children: unknown[]): PanelElement => ({
  contains: (target) => children.includes(target),
})

describe('isOutsidePanel', () => {
  it('reports a click on the panel as inside', () => {
    const search = { id: 'search' }

    expect(isOutsidePanel(search, panelWith([search]))).toBe(false)
  })

  it('reports a click on the canvas as outside', () => {
    const canvas = { id: 'canvas' }

    expect(isOutsidePanel(canvas, panelWith([{ id: 'search' }]))).toBe(true)
  })

  it('reports a missing event target as outside', () => {
    expect(isOutsidePanel(null, panelWith([]))).toBe(true)
  })

  it('stays quiet when the panel is not mounted', () => {
    expect(isOutsidePanel({ id: 'canvas' }, null)).toBe(false)
  })
})
