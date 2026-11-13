/**
 * executionCanvasFit — sizing helpers for the execution-detail graph.
 *
 * n8n parity: the execution detail view shows the workflow graph
 * across the full width of the panel. m9m does the same — the graph
 * column fills the free space next to the Node Details View and the
 * graph is re-fitted whenever that space changes.
 *
 * Vue Flow derives the viewport transform from the *measured*
 * container (`state.dimensions`), so a fit issued while the container
 * is still collapsed produces a broken transform, and Vue Flow never
 * recomputes it on its own. The helpers below are DOM-free so the
 * scheduling logic can be unit-tested.
 */
import type { FitViewParams } from '@vue-flow/core'

/** Fit options accepted by Vue Flow's `fitView()`. */
export type ExecutionFitViewOptions = FitViewParams

/**
 * Default fit options for the execution graph.
 *
 * - `padding: 0.2` leaves a small margin around the outermost nodes
 * - `minZoom: 0.3` lets a large workflow shrink into the panel
 * - `maxZoom: 1.2` avoids over-magnifying a two-node workflow
 * - `includeHiddenNodes: false` ignores collapsed nodes
 */
export const EXECUTION_FIT_VIEW_OPTIONS: ExecutionFitViewOptions = {
  padding: 0.2,
  minZoom: 0.3,
  maxZoom: 1.2,
  includeHiddenNodes: false,
}

/** The `fitView` shape we drive — Vue Flow's store method. */
export type ExecutionFitViewFn = (
  options?: ExecutionFitViewOptions,
) => Promise<boolean> | boolean | void

/**
 * A container without area cannot be fitted — Vue Flow would compute a
 * degenerate (NaN) transform, so such fits are skipped.
 */
export function isFittableSize(width: number, height: number): boolean {
  return Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0
}

export interface FitScheduler {
  /** Request a (debounced) refit. */
  schedule: () => void
  /** Drop a pending refit (component teardown). */
  cancel: () => void
  /** Run a pending refit right now. */
  flush: () => Promise<boolean> | boolean | void
}

/**
 * createFitScheduler wraps `fitView` in a debounce.
 *
 * A resize can fire dozens of times per second; every single one
 * would otherwise trigger a full viewport recomputation. The scheduler
 * collapses a burst of requests into one refit after `delay` ms.
 */
export function createFitScheduler(
  fitView: ExecutionFitViewFn,
  options: ExecutionFitViewOptions = EXECUTION_FIT_VIEW_OPTIONS,
  delay = 120,
): FitScheduler {
  let timer: ReturnType<typeof setTimeout> | null = null

  const invoke = () => {
    timer = null
    return fitView(options)
  }

  const schedule = () => {
    if (timer !== null) clearTimeout(timer)
    timer = setTimeout(invoke, delay)
  }

  const cancel = () => {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  // Run a pending fit right now and drop the debounced one, so the
  // caller never sees the same fit twice.
  const flush = () => {
    cancel()
    return fitView(options)
  }

  return { schedule, cancel, flush }
}
