export interface Workflow {
  id: string
  name: string
  description?: string
  active: boolean
  nodes: WorkflowNode[]
  connections: Record<string, NodeConnections>
  settings?: WorkflowSettings
  staticData?: Record<string, unknown>
  pinData?: Record<string, DataItem[]>
  tags?: string[]
  versionId?: string
  isArchived?: boolean
  createdAt: string
  updatedAt: string
  createdBy?: string
  // Debug flag (server-side m9m extension). When true, the engine
  // captures per-node input/output for every step and the
  // n8n-style ExecutionDetail view shows full per-node I/O. When
  // false (default for production), only the start node's input
  // and the last node's output are captured. Not part of n8n's
  // wire format, so we mark it optional.
  debug?: boolean
}

export interface WorkflowNode {
  id: string
  name: string
  type: string
  typeVersion: number
  position: [number, number]
  parameters: Record<string, unknown>
  credentials?: Record<string, NodeCredential>
  webhookId?: string
  notes?: string
  disabled?: boolean
  // n8n node-level execution flags. m9m stores them so a workflow
  // survives an n8n import -> export round-trip without losing
  // settings.
  color?: string
  onError?: string
  retryOnFail?: boolean
  maxTries?: number
  waitBetweenTries?: number
  continueOnFail?: boolean
  alwaysOutputData?: boolean
  executeOnce?: boolean
  root?: string
}

export interface NodeCredential {
  id?: string
  name: string
  type: string
  data?: Record<string, unknown>
}

export interface WorkflowSettings {
  executionOrder?: string
  timezone?: string
  saveDataError?: boolean | string
  saveDataSuccess?: boolean | string
  saveManualExecutions?: boolean | string
}

export interface NodeConnections {
  main?: Connection[][]
}

export interface Connection {
  node: string
  type: string
  index: number
}

export interface DataItem {
  json: Record<string, unknown>
  binary?: Record<string, BinaryData>
  pairedItem?: unknown
  error?: unknown
}

export interface BinaryData {
  data: string
  mimeType: string
  fileSize?: string
  fileName?: string
  directory?: string
  fileExtension?: string
}

export interface WorkflowExecution {
  id: string
  workflowId: string
  status: ExecutionStatus
  mode: ExecutionMode
  startedAt: string
  finishedAt?: string
  data?: DataItem[]
  error?: string
  nodeData?: Record<string, DataItem[]>
  // EdgesTaken records which connections actually carried items
  // during this execution. The keys are the engine's
  // `<sourceNodeID>:<outputIndex>:<targetNodeID>:<inputIndex>`
  // tuples; the value is always true (it's a set, not a count).
  // Used by ExecutionDetail to colour edges green for the path
  // that ran and grey for the path that didn't — without
  // requiring per-node I/O snapshots to be persisted.
  edgesTaken?: Record<string, true>
  metadata?: Record<string, unknown>
  createdAt: string
  updatedAt: string
}

export type ExecutionStatus = 'running' | 'completed' | 'failed' | 'cancelled' | 'waiting'
export type ExecutionMode = 'manual' | 'trigger' | 'test' | 'retry'

export interface WorkflowFilters {
  search?: string
  active?: boolean
  tags?: string[]
  offset?: number
  limit?: number
}

export interface ExecutionFilters {
  workflowId?: string
  status?: ExecutionStatus
  offset?: number
  limit?: number
}

/**
 * n8n wire format of a workflow tag. n8n stores tags as objects,
 * m9m stores the tag names only.
 */
export interface N8nWorkflowTag {
  id: string
  name: string
}

/**
 * n8n wire format of a node credential reference. The modern n8n
 * shape is `{type: {id, name}}`; the id is null when m9m could not
 * resolve the credential.
 */
export interface N8nWorkflowCredential {
  id: string | null
  name: string
}

/**
 * n8n wire format of one node.
 */
export interface N8nWorkflowNode {
  id?: string
  name: string
  type: string
  typeVersion: number
  position: number[]
  parameters: Record<string, unknown>
  credentials?: Record<string, N8nWorkflowCredential | string>
  disabled?: boolean
  notes?: string
  color?: string
  onError?: string
  retryOnFail?: boolean
  maxTries?: number
  waitBetweenTries?: number
  continueOnFail?: boolean
  alwaysOutputData?: boolean
  executeOnce?: boolean
  webhookId?: string
  root?: string
}

/**
 * n8n wire format of one connection.
 */
export interface N8nWorkflowConnection {
  node: string
  type: string
  index: number
}

/**
 * n8n wire format of the connections of one node.
 */
export interface N8nWorkflowConnections {
  main?: N8nWorkflowConnection[][]
}

/**
 * n8n's sticky-note node type.
 *
 * Sticky notes are editor-only annotations: no inputs, no outputs,
 * they never execute. m9m stores them as ordinary workflow nodes so
 * the document stays byte-compatible with n8n (n8n round-trips them
 * exactly the same way).
 */
export const STICKY_NOTE_TYPE = 'n8n-nodes-base.stickyNote'

/** Default sticky-note geometry (matches n8n's StickyNote.node.ts). */
export const STICKY_NOTE_DEFAULT_WIDTH = 240
export const STICKY_NOTE_DEFAULT_HEIGHT = 160

/** n8n's default sticky-note palette index. */
export const STICKY_NOTE_DEFAULT_COLOR = 1

/**
 * Sticky-note parameters.
 *
 * n8n keeps the geometry and the colour inside `node.parameters`
 * (NOT in top-level node fields) — see n8n's
 * `packages/nodes-base/nodes/StickyNote/StickyNote.node.ts`, which
 * declares `content`, `height`, `width` and `color` as node
 * properties. m9m does the same, which is what makes an m9m-built
 * workflow import into n8n (and back) without losing the note.
 */
export interface StickyNoteParameters {
  content?: string
  /** Palette index 1..7, or a custom hex colour (n8n allows both). */
  color?: number | string
  width?: number
  height?: number
}

/**
 * The seven sticky-note colours, in n8n's palette order. The
 * 1-based index is what n8n persists in `parameters.color`.
 *
 * n8n paints a sticky note from THREE semantic tokens
 * (`--sticky--color--background`, `--sticky--border-color`,
 * `--sticky--color--text`) and swaps the whole palette with the
 * theme — pale fills + dark text in light, deep fills + light text in
 * dark. m9m follows that model:
 *
 *  * `hex` / `border` — the light-theme fill and hairline (n8n's
 *    `yellow-100` / `yellow-200` … ramp, see DESIGN.md §2.5).
 *  * `hexDark` / `borderDark` — the dark-theme fill and hairline
 *    (n8n's `yellow-900` / `yellow-800` … ramp).
 *  * `background` / `borderColor` — the CSS custom properties the
 *    component actually binds to. They resolve per theme, so one
 *    binding serves both palettes.
 *
 * Every pairing below was measured with the WCAG 2.1 relative-
 * luminance formula: the note body clears 4.5:1 against its own fill
 * in both themes (12.9:1 … 16.7:1 light, 8.0:1 … 14.9:1 dark) and the
 * hairline clears 3:1 against the canvas (WCAG 1.4.11).
 */
export const STICKY_NOTE_COLORS: ReadonlyArray<{
  index: number
  name: string
  hex: string
  border: string
  hexDark: string
  borderDark: string
  /** Theme-aware CSS custom property for the note fill. */
  background: string
  /** Theme-aware CSS custom property for the note hairline. */
  borderColor: string
}> = [
  {
    index: 1,
    name: 'Yellow',
    hex: 'oklch(97.27% 0.0706 103.29)',
    border: 'oklch(62% 0.1293 101.4)',
    hexDark: 'oklch(42.15% 0.0952 57.94)',
    borderDark: 'oklch(52% 0.1115 57.94)',
    background: 'var(--sticky--color--background--variant-1, oklch(97.27% 0.0706 103.29))',
    borderColor: 'var(--sticky--border-color--variant-1, oklch(62% 0.1293 101.4))',
  },
  {
    index: 2,
    name: 'Gold',
    hex: 'oklch(92.41% 0.0463 78.84)',
    border: 'oklch(63% 0.0975 76.95)',
    hexDark: 'oklch(24.57% 0.0436 73.41)',
    borderDark: 'oklch(52% 0.0793 73.41)',
    background: 'var(--sticky--color--background--variant-2, oklch(92.41% 0.0463 78.84))',
    borderColor: 'var(--sticky--border-color--variant-2, oklch(63% 0.0975 76.95))',
  },
  {
    index: 3,
    name: 'Red',
    hex: 'oklch(93.65% 0.032 17.74)',
    border: 'oklch(64% 0.0827 18.93)',
    hexDark: 'oklch(25.75% 0.0918 25.84)',
    borderDark: 'oklch(53% 0.1408 25.84)',
    background: 'var(--sticky--color--background--variant-3, oklch(93.65% 0.032 17.74))',
    borderColor: 'var(--sticky--border-color--variant-3, oklch(64% 0.0827 18.93))',
  },
  {
    index: 4,
    name: 'Green',
    hex: 'oklch(96.24% 0.0434 156.74)',
    border: 'oklch(61% 0.149 154.62)',
    hexDark: 'oklch(26.54% 0.0651 152.77)',
    borderDark: 'oklch(51% 0.0957 152.28)',
    background: 'var(--sticky--color--background--variant-4, oklch(96.24% 0.0434 156.74))',
    borderColor: 'var(--sticky--border-color--variant-4, oklch(61% 0.149 154.62))',
  },
  {
    index: 5,
    name: 'Blue',
    hex: 'oklch(93.19% 0.0316 255.59)',
    border: 'oklch(63% 0.0588 253.97)',
    hexDark: 'oklch(37.96% 0.1453 265.47)',
    borderDark: 'oklch(53% 0.1982 265.47)',
    background: 'var(--sticky--color--background--variant-5, oklch(93.19% 0.0316 255.59))',
    borderColor: 'var(--sticky--border-color--variant-5, oklch(63% 0.0588 253.97))',
  },
  {
    index: 6,
    name: 'Purple',
    hex: 'oklch(89.46% 0.0562 293.06)',
    border: 'oklch(64% 0.1055 293.7)',
    hexDark: 'oklch(28.4% 0.1414 291.29)',
    borderDark: 'oklch(54% 0.232 292.71)',
    background: 'var(--sticky--color--background--variant-6, oklch(89.46% 0.0562 293.06))',
    borderColor: 'var(--sticky--border-color--variant-6, oklch(64% 0.1055 293.7))',
  },
  {
    index: 7,
    name: 'Grey',
    hex: 'oklch(97.02% 0 89.88)',
    border: 'oklch(63% 0 89.88)',
    hexDark: 'oklch(26.86% 0 89.88)',
    borderDark: 'oklch(52% 0 89.88)',
    background: 'var(--sticky--color--background--variant-7, oklch(97.02% 0 89.88))',
    borderColor: 'var(--sticky--border-color--variant-7, oklch(63% 0 89.88))',
  },
]

/** Resolves the palette entry for a `parameters.color` value. */
function stickyNotePaletteEntry(value: unknown) {
  const index = typeof value === 'number' ? value : STICKY_NOTE_DEFAULT_COLOR
  return STICKY_NOTE_COLORS.find((c) => c.index === index) ?? STICKY_NOTE_COLORS[0]
}

/**
 * Resolves a sticky note's `parameters.color` to a CSS colour.
 * Accepts n8n's palette index (1..7) and a custom hex string.
 *
 * Palette entries resolve to the theme-aware custom property, so the
 * note repaints itself when the theme flips (n8n parity: the whole
 * palette swaps, not just the text).
 */
export function stickyNoteColor(value: unknown): string {
  if (typeof value === 'string' && value.trim()) {
    return value.trim()
  }
  return stickyNotePaletteEntry(value).background
}

/**
 * Resolves the hairline border of a sticky note. Custom colours keep
 * the neutral border; palette entries use their own n8n border tone.
 */
export function stickyNoteBorder(value: unknown): string {
  if (typeof value === 'string' && value.trim()) {
    return 'var(--border-color--strong, oklch(0% 0 0 / 0.2))'
  }
  return stickyNotePaletteEntry(value).borderColor
}

/**
 * Relative luminance of a `#rrggbb` colour (WCAG 2.1).
 */
function hexRelativeLuminance(hex: string): number | null {
  const match = /^#?([0-9a-f]{6})$/i.exec(hex.trim())
  if (!match) return null
  const value = parseInt(match[1], 16)
  const channels = [(value >> 16) & 255, (value >> 8) & 255, value & 255].map((c) => {
    const srgb = c / 255
    return srgb <= 0.03928 ? srgb / 12.92 : ((srgb + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
}

/**
 * The text colour of a sticky note.
 *
 * Palette entries use the shared `--sticky--color--text` token, which
 * the theme layer flips (Graphite on the pale light palette, Neutral on
 * the deep dark palette) — exactly like n8n's `--sticky--color--text`.
 *
 * A free-form hex note keeps its own colour in both themes, so the
 * text colour is derived from the note's luminance the way n8n's
 * `getContrastTextColor` does: above 0.179 relative luminance the
 * note gets dark text, below it the note gets light text.
 */
export function stickyNoteTextColor(value: unknown): string {
  if (typeof value === 'string' && value.trim()) {
    const luminance = hexRelativeLuminance(value.trim())
    if (luminance === null) {
      return 'var(--sticky--color--text, var(--text-color))'
    }
    return luminance > 0.179
      ? 'var(--sticky--color--text--on-light, oklch(20.46% 0 89.88))'
      : 'var(--sticky--color--text--on-dark, oklch(97.02% 0 89.88))'
  }
  return 'var(--sticky--color--text, var(--text-color))'
}

/**
 * n8n wire format of one workflow document. The same shape is used
 * by the export endpoint and accepted by the import endpoint.
 */
export interface N8nWorkflowDocument {
  id?: string | number
  name?: string
  active?: boolean
  nodes: N8nWorkflowNode[]
  connections: Record<string, N8nWorkflowConnections>
  settings?: Record<string, unknown>
  staticData?: Record<string, unknown>
  pinData?: Record<string, unknown>
  meta?: Record<string, unknown>
  tags?: Array<N8nWorkflowTag | string>
  versionId?: string
}
