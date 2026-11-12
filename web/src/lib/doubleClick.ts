/**
 * Double-click detection for the canvas.
 *
 * n8n parity: a single click on a node *selects* it, a double click
 * opens the node details view. The browser's own `dblclick` event is
 * deliberately not used — after a node drag the pointer can land on
 * the same card twice and the browser would happily report a double
 * click for what was really a drag.
 *
 * Instead every pointer press is measured: a press that travels
 * further than `moveThreshold` is a drag and never counts as a click.
 * Two clean clicks on the same target, close enough in time and
 * space, form a double click.
 *
 * The module is pure logic (no DOM) so it can be unit tested.
 */

export interface DoubleClickDetectorOptions {
  /** Maximum milliseconds between the two clicks. */
  delay?: number
  /** Maximum pixels between the two clicks. */
  distance?: number
  /** Maximum pixels the pointer may travel during a single press. */
  moveThreshold?: number
}

export interface DoubleClickDetector {
  /** Starts a press at the given viewport coordinates. */
  press(x: number, y: number): void
  /** Reports pointer movement during the current press. */
  move(x: number, y: number): void
  /** Ends the current press (the drag flag survives until the next press). */
  release(): void
  /** True when the current/last press moved far enough to be a drag. */
  readonly dragged: boolean
  /**
   * Reports a click on `target`. Returns true when the click completes
   * a double click (the previous click was on the same target, inside
   * the time and distance window, and neither press was a drag).
   */
  click(target: string, at: number, x: number, y: number): boolean
  /** Forgets the pending click. */
  reset(): void
}

const DEFAULT_DELAY = 400
const DEFAULT_DISTANCE = 8
const DEFAULT_MOVE_THRESHOLD = 4

export function createDoubleClickDetector(
  options: DoubleClickDetectorOptions = {}
): DoubleClickDetector {
  const delay = options.delay ?? DEFAULT_DELAY
  const distance = options.distance ?? DEFAULT_DISTANCE
  const moveThreshold = options.moveThreshold ?? DEFAULT_MOVE_THRESHOLD

  let pressStart: { x: number; y: number } | null = null
  let pressMoved = false
  let pending: { target: string; at: number; x: number; y: number } | null = null

  const press = (x: number, y: number) => {
    pressStart = { x, y }
    pressMoved = false
  }

  const move = (x: number, y: number) => {
    if (!pressStart) return
    const dx = x - pressStart.x
    const dy = y - pressStart.y
    if (Math.sqrt(dx * dx + dy * dy) > moveThreshold) {
      pressMoved = true
    }
  }

  const release = () => {
    pressStart = null
  }

  const reset = () => {
    pressStart = null
    pressMoved = false
    pending = null
  }

  const click = (target: string, at: number, x: number, y: number): boolean => {
    // A drag is never a click — and it also breaks the pending pair,
    // so the click that follows a drag starts a fresh sequence.
    if (pressMoved) {
      pending = null
      return false
    }

    const previous = pending
    pending = { target, at, x, y }

    if (!previous) return false
    if (previous.target !== target) return false
    if (at - previous.at > delay) return false

    const dx = x - previous.x
    const dy = y - previous.y
    if (Math.sqrt(dx * dx + dy * dy) > distance) return false

    // Consume the pair — a third click starts a new sequence.
    pending = null
    return true
  }

  return {
    press,
    move,
    release,
    click,
    reset,
    get dragged() {
      return pressMoved
    },
  }
}
