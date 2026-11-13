import { describe, expect, it } from 'vitest'
import { isOutsidePanel, shouldCloseCreator, type PanelElement } from './nodeCreator'

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

/**
 * The click that OPENS the creator must not close it again.
 *
 * The panel is mounted while that click is still bubbling towards the
 * document, so a guard that is attached immediately sees the opening
 * click, finds a target outside the panel and closes the creator in
 * the same tick — the panel flashes and vanishes before the pointer
 * can drag a node onto the canvas.
 */
describe('shouldCloseCreator', () => {
  const search = { id: 'search' }
  const panel = panelWith([search])

  it('closes the creator for a later click on the canvas', () => {
    expect(shouldCloseCreator({ id: 'canvas' }, panel, 1000, 1500)).toBe(true)
  })

  it('keeps the creator open for the click that opened it', () => {
    // The toolbar button is outside the panel, but the click happened
    // before the panel existed.
    expect(shouldCloseCreator({ id: 'toolbar-button' }, panel, 1000, 900)).toBe(false)
  })

  it('keeps the creator open for a click inside the panel', () => {
    expect(shouldCloseCreator(search, panel, 1000, 1500)).toBe(false)
  })

  it('stays quiet when the panel is not mounted', () => {
    expect(shouldCloseCreator({ id: 'canvas' }, null, 1000, 1500)).toBe(false)
  })
})
