/**
 * Snapshot-based undo/redo history for the workflow editor.
 *
 * m9m's editor keeps the whole workflow document in one Pinia store,
 * so the simplest correct undo model is a snapshot stack: before a
 * meaningful mutation we push the *pre-mutation* document onto the
 * `past` stack; `undo()` pops it and restores it, `redo()` moves it
 * back onto `past` and restores the newer document.
 *
 * Snapshots are taken on discrete edits only (add/delete node,
 * connect/disconnect, paste, layout, rename …) — never per mouse
 * move. Rapid successive edits of the same kind (typing in a
 * parameter field, holding an arrow key to nudge a node) are
 * *coalesced* into a single history entry so one Ctrl+Z does not
 * undo a single keystroke.
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { NodeConnections, WorkflowNode, WorkflowSettings } from '@/types'

/** Maximum number of undo steps kept in memory. */
export const HISTORY_LIMIT = 50

/**
 * Two edits with the same coalesce key that happen within this
 * window are merged into one history entry.
 */
export const COALESCE_WINDOW_MS = 600

/**
 * The part of the workflow document the history restores. `name`,
 * `nodes`, `connections` and `settings` — everything the editor can
 * mutate structurally.
 */
export interface WorkflowSnapshot {
  name: string
  nodes: WorkflowNode[]
  connections: Record<string, NodeConnections>
  settings?: WorkflowSettings
}

export interface HistoryEntry {
  /** Human-readable label of the action this entry undoes. */
  label: string
  snapshot: WorkflowSnapshot
  timestamp: number
  /** When set, consecutive entries with the same key coalesce. */
  coalesceKey?: string
}

export interface RecordOptions {
  /**
   * Merge this entry with the previous one when it carries the same
   * key and was recorded recently.
   */
  coalesceKey?: string
}

function cloneNode(node: WorkflowNode): WorkflowNode {
  return {
    ...node,
    position: [node.position[0], node.position[1]],
    parameters: JSON.parse(JSON.stringify(node.parameters ?? {})),
    credentials: node.credentials
      ? (JSON.parse(JSON.stringify(node.credentials)) as WorkflowNode['credentials'])
      : undefined,
  }
}

function cloneConnections(
  connections: Record<string, NodeConnections>
): Record<string, NodeConnections> {
  const result: Record<string, NodeConnections> = {}
  Object.entries(connections ?? {}).forEach(([name, nodeConnections]) => {
    result[name] = {
      main: nodeConnections.main?.map((outputs) => outputs.map((c) => ({ ...c }))),
    }
  })
  return result
}

export function cloneSnapshot(snapshot: WorkflowSnapshot): WorkflowSnapshot {
  return {
    name: snapshot.name,
    nodes: (snapshot.nodes ?? []).map(cloneNode),
    connections: cloneConnections(snapshot.connections),
    settings: snapshot.settings ? { ...snapshot.settings } : undefined,
  }
}

export const useHistoryStore = defineStore('history', () => {
  const past = ref<HistoryEntry[]>([])
  const future = ref<HistoryEntry[]>([])

  const canUndo = computed(() => past.value.length > 0)
  const canRedo = computed(() => future.value.length > 0)

  /** Label of the action the next undo would revert. */
  const undoLabel = computed(() => past.value[past.value.length - 1]?.label ?? '')
  /** Label of the action the next redo would re-apply. */
  const redoLabel = computed(() => future.value[future.value.length - 1]?.label ?? '')

  /**
   * Records the *current* (pre-mutation) document so the mutation
   * that follows can be undone.
   *
   * `snapshot` may be a factory: it is only invoked when the entry is
   * actually recorded, so a coalesced burst of edits (a resize drag,
   * a held-down arrow key) does not pay for a deep clone per event.
   */
  function record(
    label: string,
    snapshot: WorkflowSnapshot | (() => WorkflowSnapshot),
    options: RecordOptions = {}
  ) {
    const now = Date.now()
    const coalesceKey = options.coalesceKey

    if (coalesceKey) {
      const previous = past.value[past.value.length - 1]
      if (
        previous &&
        previous.coalesceKey === coalesceKey &&
        now - previous.timestamp < COALESCE_WINDOW_MS
      ) {
        // Same kind of edit, moments ago: keep the *oldest* snapshot
        // (the state before the whole burst of edits) and just extend
        // the window.
        previous.timestamp = now
        return
      }
    }

    const resolved = typeof snapshot === 'function' ? snapshot() : snapshot

    past.value.push({
      label,
      snapshot: cloneSnapshot(resolved),
      timestamp: now,
      coalesceKey,
    })

    if (past.value.length > HISTORY_LIMIT) {
      past.value.splice(0, past.value.length - HISTORY_LIMIT)
    }

    // A new edit invalidates the redo stack.
    future.value = []
  }

  /**
   * Pops the previous snapshot and returns it. The caller restores
   * the returned snapshot; the document it passed in is pushed onto
   * the redo stack.
   */
  function undo(current: WorkflowSnapshot): HistoryEntry | null {
    const entry = past.value.pop()
    if (!entry) {
      return null
    }
    future.value.push({
      label: entry.label,
      snapshot: cloneSnapshot(current),
      timestamp: Date.now(),
    })
    return entry
  }

  /** Pops the next snapshot and returns it (redo counterpart). */
  function redo(current: WorkflowSnapshot): HistoryEntry | null {
    const entry = future.value.pop()
    if (!entry) {
      return null
    }
    past.value.push({
      label: entry.label,
      snapshot: cloneSnapshot(current),
      timestamp: Date.now(),
    })
    return entry
  }

  /** Drops the whole history (e.g. after loading a different workflow). */
  function reset() {
    past.value = []
    future.value = []
  }

  return {
    past,
    future,
    canUndo,
    canRedo,
    undoLabel,
    redoLabel,
    record,
    undo,
    redo,
    reset,
  }
})
