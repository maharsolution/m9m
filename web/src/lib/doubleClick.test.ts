import { describe, expect, it } from 'vitest'
import { createDoubleClickDetector } from './doubleClick'

/**
 * The canvas journey: one click selects, two clicks open the NDV, a
 * drag is never a double click.
 */
describe('createDoubleClickDetector', () => {
  it('does not report a double click for a single click', () => {
    const detector = createDoubleClickDetector()

    detector.press(100, 100)
    detector.release()

    expect(detector.click('node-1', 1000, 100, 100)).toBe(false)
  })

  it('reports a double click for two quick clicks on the same node', () => {
    const detector = createDoubleClickDetector()

    detector.press(100, 100)
    detector.release()
    expect(detector.click('node-1', 1000, 100, 100)).toBe(false)

    detector.press(102, 101)
    detector.release()
    expect(detector.click('node-1', 1200, 102, 101)).toBe(true)
  })

  it('does not report a double click across two different nodes', () => {
    const detector = createDoubleClickDetector()

    detector.press(100, 100)
    detector.release()
    detector.click('node-1', 1000, 100, 100)

    detector.press(400, 100)
    detector.release()
    expect(detector.click('node-2', 1100, 400, 100)).toBe(false)
  })

  it('does not report a double click when the clicks are too far apart in time', () => {
    const detector = createDoubleClickDetector()

    detector.press(100, 100)
    detector.release()
    detector.click('node-1', 1000, 100, 100)

    detector.press(100, 100)
    detector.release()
    expect(detector.click('node-1', 1600, 100, 100)).toBe(false)
  })

  it('treats a drag as a drag, never as a click', () => {
    const detector = createDoubleClickDetector()

    // Press, move well beyond the threshold (a node drag), release.
    detector.press(100, 100)
    detector.move(160, 140)
    detector.release()

    expect(detector.dragged).toBe(true)
    expect(detector.click('node-1', 1000, 160, 140)).toBe(false)
  })

  it('does not pair a click that follows a drag with an earlier click', () => {
    const detector = createDoubleClickDetector()

    detector.press(100, 100)
    detector.release()
    detector.click('node-1', 1000, 100, 100)

    // The user drags the node: the press moves, so the release is not
    // a click and the pending pair is dropped.
    detector.press(100, 100)
    detector.move(200, 200)
    detector.release()
    expect(detector.click('node-1', 1100, 200, 200)).toBe(false)

    // The next clean click is only the first half of a pair.
    detector.press(200, 200)
    detector.release()
    expect(detector.click('node-1', 1200, 200, 200)).toBe(false)
  })

  it('resets the drag flag on the next press', () => {
    const detector = createDoubleClickDetector()

    detector.press(100, 100)
    detector.move(160, 140)
    detector.release()
    expect(detector.dragged).toBe(true)

    detector.press(160, 140)
    expect(detector.dragged).toBe(false)
  })

  it('consumes the pair so a third click does not re-trigger', () => {
    const detector = createDoubleClickDetector()

    detector.press(100, 100)
    detector.release()
    detector.click('node-1', 1000, 100, 100)

    detector.press(100, 100)
    detector.release()
    expect(detector.click('node-1', 1100, 100, 100)).toBe(true)

    detector.press(100, 100)
    detector.release()
    expect(detector.click('node-1', 1200, 100, 100)).toBe(false)
  })
})
