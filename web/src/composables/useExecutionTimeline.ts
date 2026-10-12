// useExecutionTimeline for BUG-CAP-2026-09-09-004.
//
// The execution-history list view shows a flat list of
// rows. A "timeline" view groups rows by date and shows
// per-day counts + a sparkline of the day's success /
// failure ratio. This composable turns the raw
// `Execution[]` into the data shape the timeline view
// renders.
//
// It's a pure transformation — no Vue, no DOM, no
// network — so it has a small unit test in
// `web/test/useExecutionTimeline.test.mjs`.

import { computed, type ComputedRef, type Ref } from 'vue'
import type { WorkflowExecution } from '@/types'

export interface TimelineBucket {
  // The bucket's date as ISO `YYYY-MM-DD`.
  date: string
  // The bucket's display label (e.g. "Today", "Yesterday",
  // "12 Oct 2026").
  label: string
  // Counts.
  total: number
  success: number
  failed: number
  running: number
  // Success rate in [0, 1].
  successRate: number
  // Per-execution entry (so the view can drill in).
  executions: WorkflowExecution[]
}

// buildTimeline buckets a flat list of executions by
// calendar day (UTC, then localised at render time).
export function buildTimeline(executions: WorkflowExecution[]): TimelineBucket[] {
  const buckets = new Map<string, TimelineBucket>()
  for (const exec of executions) {
    const date = new Date(exec.startedAt)
    const key = date.toISOString().slice(0, 10)
    let bucket = buckets.get(key)
    if (!bucket) {
      bucket = {
        date: key,
        label: labelFor(key),
        total: 0,
        success: 0,
        failed: 0,
        running: 0,
        successRate: 0,
        executions: [],
      }
      buckets.set(key, bucket)
    }
    bucket.executions.push(exec)
    bucket.total++
    switch (exec.status) {
      case 'completed':
        bucket.success++
        break
      case 'failed':
        bucket.failed++
        break
      case 'running':
        bucket.running++
        break
    }
  }
  for (const b of buckets.values()) {
    const decided = b.success + b.failed
    b.successRate = decided === 0 ? 0 : b.success / decided
  }
  // Newest day first.
  return Array.from(buckets.values()).sort((a, b) => (a.date < b.date ? 1 : -1))
}

// labelFor returns a human-readable label for an ISO date.
// Today and Yesterday are special-cased; other dates are
// rendered as e.g. "12 Oct 2026" (no timezone, so the
// "today" check uses the browser's local day).
export function labelFor(iso: string): string {
  const today = new Date()
  const todayKey = today.toISOString().slice(0, 10)
  if (iso === todayKey) return 'Today'
  const yest = new Date(today.getTime() - 86_400_000)
  if (iso === yest.toISOString().slice(0, 10)) return 'Yesterday'
  // `new Date(iso)` parses `YYYY-MM-DD` as UTC midnight.
  const d = new Date(iso)
  return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}

// filterByDateRange applies a `[from, to]` window to the
// timeline. Either bound is optional.
export function filterByDateRange(
  buckets: TimelineBucket[],
  from: string | null,
  to: string | null
): TimelineBucket[] {
  return buckets.filter((b) => {
    if (from && b.date < from) return false
    if (to && b.date > to) return false
    return true
  })
}

// useExecutionTimeline is the Vue composable form.
// Returns a `ComputedRef<TimelineBucket[]>` that auto-
// updates when the underlying ref changes.
export function useExecutionTimeline(executions: Ref<WorkflowExecution[]>): ComputedRef<TimelineBucket[]> {
  return computed(() => buildTimeline(executions.value))
}
