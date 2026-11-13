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

/**
 * True when a click has to close the node creator.
 *
 * The click that OPENED the creator must never close it again. The
 * panel is mounted while that very click is still bubbling towards
 * the document, so a document-level guard would see it, find a target
 * outside the panel and close the creator in the same tick — the
 * creator opened and vanished again before the pointer even moved.
 * That is what made "drag a node onto the canvas" impossible: there
 * was no panel left to drag from.
 *
 * `openedAt` (when the panel was mounted) and `eventTimeStamp` (the
 * click's own timestamp) have to come from the same clock:
 * `performance.now()` and `event.timeStamp` do — both are relative to
 * the page's time origin. A click whose timestamp is at or before the
 * mount therefore happened before the panel existed and is ignored.
 */
export function shouldCloseCreator(
  target: unknown,
  panel: PanelElement | null,
  openedAt: number,
  eventTimeStamp: number
): boolean {
  if (!panel) return false
  // The click that opened the creator predates the panel.
  if (eventTimeStamp <= openedAt) return false
  return isOutsidePanel(target, panel)
}
