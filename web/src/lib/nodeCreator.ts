/**
 * Node-creator overlay helpers.
 *
 * The node creator is a floating panel over the workflow canvas. Its
 * scrim dims the canvas but is *visual only* — it never takes pointer
 * events, so a node can be dragged from the creator onto the canvas
 * and the canvas stays pannable / selectable while the creator is open
 * (n8n parity).
 *
 * Because the scrim cannot take the click, the creator is closed by a
 * document-level click guard. The decision is pure logic so it can be
 * unit tested without a DOM.
 */

/** The part of the panel element the click guard needs. */
export interface PanelElement {
  contains(target: unknown): boolean
}

/**
 * True when a pointer event landed outside the node-creator panel.
 *
 * A missing panel means the creator is not mounted, so there is
 * nothing to close and the guard stays quiet.
 */
export function isOutsidePanel(target: unknown, panel: PanelElement | null): boolean {
  if (!panel) return false
  if (target === null || target === undefined) return true
  return !panel.contains(target)
}
