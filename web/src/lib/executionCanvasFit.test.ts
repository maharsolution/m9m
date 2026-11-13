import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createFitScheduler,
  EXECUTION_FIT_VIEW_OPTIONS,
  isFittableSize,
} from './executionCanvasFit'

describe('executionCanvasFit', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('collapses a burst of fit requests into a single fit', () => {
    const fitView = vi.fn().mockResolvedValue(true)
    const scheduler = createFitScheduler(fitView, EXECUTION_FIT_VIEW_OPTIONS, 100)

    scheduler.schedule()
    scheduler.schedule()
    scheduler.schedule()

    expect(fitView).not.toHaveBeenCalled()

    vi.advanceTimersByTime(100)

    expect(fitView).toHaveBeenCalledTimes(1)
    expect(fitView).toHaveBeenCalledWith(EXECUTION_FIT_VIEW_OPTIONS)
  })

  it('drops a pending fit when cancelled', () => {
    const fitView = vi.fn().mockResolvedValue(true)
    const scheduler = createFitScheduler(fitView, EXECUTION_FIT_VIEW_OPTIONS, 100)

    scheduler.schedule()
    scheduler.cancel()
    vi.advanceTimersByTime(500)

    expect(fitView).not.toHaveBeenCalled()
  })

  it('fits immediately on flush', () => {
    const fitView = vi.fn().mockResolvedValue(true)
    const scheduler = createFitScheduler(fitView, EXECUTION_FIT_VIEW_OPTIONS, 100)

    scheduler.schedule()
    scheduler.flush()

    expect(fitView).toHaveBeenCalledTimes(1)

    // The debounced call must not fire a second time afterwards.
    vi.advanceTimersByTime(500)
    expect(fitView).toHaveBeenCalledTimes(1)
  })

  it('uses the execution fit options by default', () => {
    expect(EXECUTION_FIT_VIEW_OPTIONS).toEqual({
      padding: 0.2,
      minZoom: 0.3,
      maxZoom: 1.2,
      includeHiddenNodes: false,
    })
  })

  it('refuses to fit a collapsed container', () => {
    expect(isFittableSize(0, 400)).toBe(false)
    expect(isFittableSize(400, 0)).toBe(false)
    expect(isFittableSize(Number.NaN, 400)).toBe(false)
    expect(isFittableSize(400, 300)).toBe(true)
  })
})
