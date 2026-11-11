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
 */
export const STICKY_NOTE_COLORS: ReadonlyArray<{
  index: number
  name: string
  hex: string
}> = [
  { index: 1, name: 'Yellow', hex: '#fef9c3' },
  { index: 2, name: 'Gold', hex: '#fef3c7' },
  { index: 3, name: 'Red', hex: '#fee2e2' },
  { index: 4, name: 'Green', hex: '#dcfce7' },
  { index: 5, name: 'Blue', hex: '#dbeafe' },
  { index: 6, name: 'Purple', hex: '#ede9fe' },
  { index: 7, name: 'Grey', hex: '#f1f5f9' },
]

/**
 * Resolves a sticky note's `parameters.color` to a CSS colour.
 * Accepts n8n's palette index (1..7) and a custom hex string.
 */
export function stickyNoteColor(value: unknown): string {
  if (typeof value === 'string' && value.trim()) {
    return value.trim()
  }
  const index = typeof value === 'number' ? value : STICKY_NOTE_DEFAULT_COLOR
  const match = STICKY_NOTE_COLORS.find((c) => c.index === index)
  return match?.hex ?? STICKY_NOTE_COLORS[0].hex
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
