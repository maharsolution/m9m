import { computed, ref, shallowRef } from 'vue'
import { defineStore } from 'pinia'
import type {
  NodeConnections,
  StickyNoteParameters,
  Workflow,
  WorkflowNode,
  WorkflowSettings,
} from '@/types'
import {
  STICKY_NOTE_DEFAULT_COLOR,
  STICKY_NOTE_DEFAULT_HEIGHT,
  STICKY_NOTE_DEFAULT_WIDTH,
  STICKY_NOTE_TYPE,
} from '@/types'
import { useWorkflowStore } from './workflow'
import { useHistoryStore, type WorkflowSnapshot } from './history'
import { computeAutoLayout } from '@/lib/autoLayout'
import {
  addConnectionByNodeId,
  addConnectionToWorkflow,
  addNodeToWorkflow,
  createWorkflowDraft,
  removeConnectionByNodeId,
  removeNodeFromWorkflow,
  updateNodeInWorkflow,
} from '@/lib/workflowGraph'

/**
 * The clipboard payload. It is transient editor state — it is never
 * persisted with the workflow — but it lives in a `shallowRef` so the
 * toolbar can react to "there is something to paste".
 */
interface EditorClipboard {
  nodes: WorkflowNode[]
  links: Array<{
    sourceId: string
    targetId: string
    sourceOutput: number
    targetInput: number
  }>
}

let nodeIdCounter = 0

/**
 * Unique node id. `crypto.randomUUID` is unavailable on insecure
 * origins (the editor is often served over plain http), so a
 * timestamp + counter fallback is used there.
 */
function generateNodeId(): string {
  const cryptoApi = globalThis.crypto
  if (cryptoApi && typeof cryptoApi.randomUUID === 'function') {
    return cryptoApi.randomUUID()
  }
  nodeIdCounter += 1
  return `node_${Date.now().toString(36)}_${nodeIdCounter.toString(36)}`
}

/**
 * n8n's naming convention: the first instance keeps the bare display
 * name ("HTTP Request"), duplicates get a numeric suffix without a
 * space ("HTTP Request1"). Node names are the keys of the n8n
 * `connections` map, so they must stay unique.
 */
function uniqueNodeName(baseName: string, taken: Set<string>): string {
  if (!taken.has(baseName)) {
    return baseName
  }
  let counter = 1
  while (taken.has(`${baseName}${counter}`)) {
    counter += 1
  }
  return `${baseName}${counter}`
}

function cloneWorkflowNode(node: WorkflowNode): WorkflowNode {
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

/**
 * True when the update only moves the node. Position updates happen
 * on every mouse-move during a drag, so they must not produce a
 * history entry each — the drag itself is recorded once, when it
 * starts.
 */
function isPositionOnly(updates: Partial<WorkflowNode>): boolean {
  const keys = Object.keys(updates).filter(
    (key) => updates[key as keyof WorkflowNode] !== undefined
  )
  return keys.length > 0 && keys.every((key) => key === 'position')
}

export const useWorkflowEditorStore = defineStore('workflowEditor', () => {
  const workflowStore = useWorkflowStore()
  const historyStore = useHistoryStore()

  const selectedNodeIds = ref<string[]>([])
  const isDirty = ref(false)
  const editorClipboard = shallowRef<EditorClipboard | null>(null)

  const selectedNodes = computed(() => {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return []
    }

    return workflow.nodes.filter((node) => selectedNodeIds.value.includes(node.id))
  })

  const selectedNode = computed(() => {
    const workflow = workflowStore.currentWorkflow
    if (!workflow || selectedNodeIds.value.length !== 1) {
      return null
    }

    return workflow.nodes.find((node) => node.id === selectedNodeIds.value[0]) ?? null
  })

  /** Number of currently selected nodes (shown as a badge on canvas). */
  const selectedNodeCount = computed(() => selectedNodeIds.value.length)

  /** True when something is on the editor clipboard. */
  const hasClipboard = computed(() => (editorClipboard.value?.nodes.length ?? 0) > 0)

  /** Undo/redo availability, mirrored for the toolbar buttons. */
  const canUndo = computed(() => historyStore.canUndo)
  const canRedo = computed(() => historyStore.canRedo)

  // ------------------------------------------------------------------
  // History
  // ------------------------------------------------------------------

  function snapshotDocument(): WorkflowSnapshot {
    const workflow = workflowStore.currentWorkflow
    return {
      name: workflow?.name ?? '',
      nodes: workflow?.nodes.map(cloneWorkflowNode) ?? [],
      connections: cloneConnections(workflow?.connections ?? {}),
      settings: workflow?.settings ? { ...workflow.settings } : undefined,
    }
  }

  /**
   * Records the *current* document so the mutation that follows can
   * be undone. Call this BEFORE mutating the workflow.
   */
  function recordHistory(label: string, options: { coalesceKey?: string } = {}) {
    historyStore.record(label, () => snapshotDocument(), options)
  }

  function restoreSnapshot(snapshot: WorkflowSnapshot) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    workflow.name = snapshot.name
    workflow.nodes = snapshot.nodes.map(cloneWorkflowNode)
    workflow.connections = cloneConnections(snapshot.connections)
    if (snapshot.settings) {
      workflow.settings = { ...snapshot.settings } as WorkflowSettings
    }

    selectedNodeIds.value = selectedNodeIds.value.filter((id) =>
      workflow.nodes.some((node) => node.id === id)
    )
    isDirty.value = true
  }

  function undo(): boolean {
    if (!workflowStore.currentWorkflow) {
      return false
    }
    const entry = historyStore.undo(snapshotDocument())
    if (!entry) {
      return false
    }
    restoreSnapshot(entry.snapshot)
    return true
  }

  function redo(): boolean {
    if (!workflowStore.currentWorkflow) {
      return false
    }
    const entry = historyStore.redo(snapshotDocument())
    if (!entry) {
      return false
    }
    restoreSnapshot(entry.snapshot)
    return true
  }

  /**
   * Called when a node drag starts. It captures the pre-drag state
   * once, so a drag is a single undo step instead of one per frame.
   */
  function beginNodeDrag() {
    recordHistory('Move node')
  }

  function resetEditorState() {
    selectedNodeIds.value = []
    ndvNodeId.value = null
    isDirty.value = false
    historyStore.reset()
  }

  function markDirty() {
    isDirty.value = true
  }

  function markClean() {
    isDirty.value = false
  }

  /**
   * Opens a NEW workflow in the editor.
   *
   * `sourceWorkflow` is only used by the Duplicate action - the draft is
   * always a new workflow, so it starts with Debug OFF (createWorkflowDraft
   * enforces that). The per-workflow Debug toggle is untouched: it flips
   * the flag of the workflow that is already saved.
   */
  function createNewWorkflow(sourceWorkflow?: Workflow, overrides: Partial<Workflow> = {}) {
    workflowStore.setCurrentWorkflow(createWorkflowDraft(sourceWorkflow, overrides))
    selectedNodeIds.value = []
    isDirty.value = true
    historyStore.reset()
  }

  // ------------------------------------------------------------------
  // Nodes
  // ------------------------------------------------------------------

  function addNode(node: WorkflowNode) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    recordHistory('Add node')
    addNodeToWorkflow(workflow, node)
    selectedNodeIds.value = [node.id]
    isDirty.value = true
  }

  function updateNode(
    nodeId: string,
    updates: Partial<WorkflowNode>,
    options: { label?: string; coalesceKey?: string } = {}
  ) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    // Special handling for `credentials`: merge instead of replace, so a
    // caller can bind one credential type without dropping the others.
    if (updates.credentials) {
      const existing = workflow.nodes.find((n) => n.id === nodeId)
      if (existing) {
        updates = {
          ...updates,
          credentials: {
            ...(existing.credentials ?? {}),
            ...updates.credentials,
          },
        }
      }
    }

    if (!isPositionOnly(updates)) {
      const changedKeys = Object.keys(updates).sort().join(',')
      recordHistory(options.label ?? 'Update node', {
        coalesceKey: options.coalesceKey ?? `node:${nodeId}:${changedKeys}`,
      })
    }

    if (updateNodeInWorkflow(workflow, nodeId, updates)) {
      isDirty.value = true
    }
  }

  /**
   * Bind a credential to a node. Pass `null` for the credential id to
   * unbind that type. Other credential types already bound to the node
   * are preserved.
   */
  function bindCredential(
    nodeId: string,
    credType: string,
    credential: { id: string; name: string } | null
  ) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) return

    const node = workflow.nodes.find((n) => n.id === nodeId)
    if (!node) return

    const next = { ...(node.credentials ?? {}) } as Record<
      string,
      { id?: string; name: string }
    >
    if (credential === null) {
      delete next[credType]
    } else {
      next[credType] = { id: credential.id, name: credential.name }
    }
    updateNode(nodeId, { credentials: next as any })
  }

  function removeNode(nodeId: string) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    recordHistory('Delete node')
    if (removeNodeFromWorkflow(workflow, nodeId)) {
      selectedNodeIds.value = selectedNodeIds.value.filter((id) => id !== nodeId)
      // The NDV cannot show a node that no longer exists.
      if (ndvNodeId.value === nodeId) {
        ndvNodeId.value = null
      }
      isDirty.value = true
    }
  }

  /** Removes every selected node (and its connections) in one step. */
  function removeSelectedNodes(): number {
    const workflow = workflowStore.currentWorkflow
    if (!workflow || selectedNodeIds.value.length === 0) {
      return 0
    }

    const ids = [...selectedNodeIds.value]
    recordHistory(ids.length > 1 ? 'Delete nodes' : 'Delete node')

    ids.forEach((id) => removeNodeFromWorkflow(workflow, id))
    selectedNodeIds.value = []
    isDirty.value = true
    return ids.length
  }

  /** Moves the selected nodes by the given offset (arrow-key nudge). */
  function nudgeSelected(dx: number, dy: number): boolean {
    const workflow = workflowStore.currentWorkflow
    if (!workflow || selectedNodeIds.value.length === 0) {
      return false
    }

    recordHistory('Move nodes', { coalesceKey: 'nudge' })
    selectedNodeIds.value.forEach((id) => {
      const node = workflow.nodes.find((n) => n.id === id)
      if (!node) return
      updateNodeInWorkflow(workflow, id, {
        position: [node.position[0] + dx, node.position[1] + dy],
      })
    })
    isDirty.value = true
    return true
  }

  // ------------------------------------------------------------------
  // Sticky notes
  // ------------------------------------------------------------------

  function defaultStickyPosition(workflow: Workflow): [number, number] {
    if (workflow.nodes.length === 0) {
      return [80, 80]
    }
    const lowest = Math.max(...workflow.nodes.map((node) => node.position[1]))
    const leftmost = Math.min(...workflow.nodes.map((node) => node.position[0]))
    return [leftmost, lowest + STICKY_NOTE_DEFAULT_HEIGHT + 40]
  }

  /**
   * Adds an n8n sticky note. The note is stored as a regular workflow
   * node of type `n8n-nodes-base.stickyNote` with the geometry and
   * the colour inside `parameters` — exactly n8n's wire format.
   */
  function addStickyNote(position?: [number, number]): string | null {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return null
    }

    recordHistory('Add sticky note')

    const taken = new Set(workflow.nodes.map((node) => node.name))
    const node: WorkflowNode = {
      id: generateNodeId(),
      name: uniqueNodeName('Sticky Note', taken),
      type: STICKY_NOTE_TYPE,
      typeVersion: 1,
      position: position ?? defaultStickyPosition(workflow),
      parameters: {
        content: '',
        color: STICKY_NOTE_DEFAULT_COLOR,
        width: STICKY_NOTE_DEFAULT_WIDTH,
        height: STICKY_NOTE_DEFAULT_HEIGHT,
      },
    }

    addNodeToWorkflow(workflow, node)
    selectedNodeIds.value = [node.id]
    isDirty.value = true
    return node.id
  }

  /** Updates the content / colour / geometry of a sticky note. */
  function updateStickyNote(
    nodeId: string,
    updates: Partial<StickyNoteParameters>
  ): boolean {
    const workflow = workflowStore.currentWorkflow
    const node = workflow?.nodes.find((n) => n.id === nodeId)
    if (!workflow || !node) {
      return false
    }

    recordHistory('Edit sticky note', { coalesceKey: `sticky:${nodeId}` })
    updateNodeInWorkflow(workflow, nodeId, {
      parameters: { ...node.parameters, ...updates },
    })
    isDirty.value = true
    return true
  }

  // ------------------------------------------------------------------
  // Connections
  // ------------------------------------------------------------------

  function addConnection(sourceNodeId: string, targetNodeId: string, sourceOutput = 0, targetInput = 0) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    recordHistory('Connect nodes')
    if (addConnectionByNodeId(workflow, sourceNodeId, targetNodeId, sourceOutput, targetInput)) {
      isDirty.value = true
    }
  }

  function removeConnection(sourceNodeId: string, targetNodeId: string, sourceOutput = 0, targetInput = 0) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    recordHistory('Disconnect nodes')
    if (removeConnectionByNodeId(workflow, sourceNodeId, targetNodeId, sourceOutput, targetInput)) {
      isDirty.value = true
    }
  }

  // ------------------------------------------------------------------
  // Selection
  // ------------------------------------------------------------------

  function selectNode(nodeId: string, addToSelection = false) {
    if (addToSelection) {
      if (selectedNodeIds.value.includes(nodeId)) {
        selectedNodeIds.value = selectedNodeIds.value.filter((id) => id !== nodeId)
      } else {
        selectedNodeIds.value = [...selectedNodeIds.value, nodeId]
      }
      return
    }

    selectedNodeIds.value = [nodeId]
  }

  function selectNodes(nodeIds: string[]) {
    selectedNodeIds.value = [...nodeIds]
  }

  /** Adds / removes a node from the selection (shift- or ctrl-click). */
  function toggleNodeSelection(nodeId: string) {
    selectNode(nodeId, true)
  }

  /** Selects every node of the workflow (Ctrl+A). */
  function selectAll() {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }
    selectedNodeIds.value = workflow.nodes.map((node) => node.id)
  }

  function clearSelection() {
    selectedNodeIds.value = []
  }

  // ------------------------------------------------------------------
  // Node details view (NDV)
  //
  // n8n parity: the NDV is a layer of its own. A single click on a
  // node only *selects* it (the 6px ring on the card), a double click
  // opens the NDV. The open node therefore lives in its own state
  // instead of being derived from the selection — closing the NDV
  // keeps the node selected, and selecting another node while the
  // NDV is open does not silently swap the panel's content.
  // ------------------------------------------------------------------

  /** Id of the node the NDV is showing (null when the NDV is closed). */
  const ndvNodeId = ref<string | null>(null)

  /** The node the NDV is showing — null while the NDV is closed. */
  const ndvNode = computed(() => {
    const workflow = workflowStore.currentWorkflow
    if (!workflow || !ndvNodeId.value) {
      return null
    }

    return workflow.nodes.find((node) => node.id === ndvNodeId.value) ?? null
  })

  /** True while the NDV is open. */
  const isNodeDetailsOpen = computed(() => ndvNode.value !== null)

  /**
   * Opens the NDV for a node. Unknown ids are ignored so a stale
   * click (e.g. on a node that was just deleted) cannot open the
   * panel.
   */
  function openNodeDetails(nodeId: string) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    if (!workflow.nodes.some((node) => node.id === nodeId)) {
      return
    }

    ndvNodeId.value = nodeId
  }

  /**
   * Closes the NDV. The canvas selection is intentionally left
   * untouched — n8n keeps the node selected after the NDV closes.
   */
  function closeNodeDetails() {
    ndvNodeId.value = null
  }

  // ------------------------------------------------------------------
  // Copy / paste / duplicate
  // ------------------------------------------------------------------

  /**
   * Copies the selected nodes (and the connections between them) to
   * the editor clipboard. Returns the number of copied nodes.
   */
  function copySelected(): number {
    const workflow = workflowStore.currentWorkflow
    if (!workflow || selectedNodeIds.value.length === 0) {
      return 0
    }

    const selected = new Set(selectedNodeIds.value)
    const nodes = workflow.nodes
      .filter((node) => selected.has(node.id))
      .map(cloneWorkflowNode)

    if (nodes.length === 0) {
      return 0
    }

    const idByName = new Map(workflow.nodes.map((node) => [node.name, node.id]))
    const links: EditorClipboard['links'] = []

    Object.entries(workflow.connections ?? {}).forEach(([sourceName, nodeConnections]) => {
      const sourceId = idByName.get(sourceName)
      if (!sourceId || !selected.has(sourceId)) {
        return
      }
      nodeConnections.main?.forEach((outputs, sourceOutput) => {
        outputs.forEach((connection) => {
          const targetId = idByName.get(connection.node)
          if (!targetId || !selected.has(targetId)) {
            return
          }
          links.push({
            sourceId,
            targetId,
            sourceOutput,
            targetInput: connection.index,
          })
        })
      })
    })

    editorClipboard.value = { nodes, links }
    return nodes.length
  }

  /**
   * Pastes the clipboard. The copies get fresh ids, unique names and
   * an offset position; only the connections that existed *between the
   * copied nodes* are re-created.
   */
  function paste(offset: { x: number; y: number } = { x: 40, y: 40 }): string[] {
    const workflow = workflowStore.currentWorkflow
    const clipboard = editorClipboard.value
    if (!workflow || !clipboard?.nodes.length) {
      return []
    }

    recordHistory('Paste nodes')

    const taken = new Set(workflow.nodes.map((node) => node.name))
    const idMap = new Map<string, string>()
    const newNodes: WorkflowNode[] = []

    clipboard.nodes.forEach((source) => {
      const id = generateNodeId()
      idMap.set(source.id, id)
      const name = uniqueNodeName(source.name, taken)
      taken.add(name)
      newNodes.push({
        ...cloneWorkflowNode(source),
        id,
        name,
        position: [source.position[0] + offset.x, source.position[1] + offset.y],
      })
    })

    newNodes.forEach((node) => addNodeToWorkflow(workflow, node))

    const nameById = new Map(newNodes.map((node) => [node.id, node.name]))
    clipboard.links.forEach((link) => {
      const sourceName = nameById.get(idMap.get(link.sourceId) ?? '')
      const targetName = nameById.get(idMap.get(link.targetId) ?? '')
      if (!sourceName || !targetName) {
        return
      }
      addConnectionToWorkflow(
        workflow,
        sourceName,
        targetName,
        link.sourceOutput,
        link.targetInput
      )
    })

    selectedNodeIds.value = newNodes.map((node) => node.id)
    isDirty.value = true
    return newNodes.map((node) => node.id)
  }

  /** Duplicates the current selection (Ctrl+D). */
  function duplicateSelected(): string[] {
    if (copySelected() === 0) {
      return []
    }
    return paste({ x: 40, y: 40 })
  }

  // ------------------------------------------------------------------
  // Layout
  // ------------------------------------------------------------------

  /** Runs the dagre auto-layout ("tidy up") over the whole workflow. */
  function autoLayout(): boolean {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return false
    }

    const positions = computeAutoLayout(workflow.nodes, workflow.connections)
    if (positions.size === 0) {
      return false
    }

    recordHistory('Tidy up')
    positions.forEach((position, nodeId) => {
      updateNodeInWorkflow(workflow, nodeId, { position: [position.x, position.y] })
    })
    isDirty.value = true
    return true
  }

  function setWorkflowName(name: string) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    recordHistory('Rename workflow')
    workflow.name = name
    isDirty.value = true
  }

  return {
    selectedNodeIds,
    selectedNodes,
    selectedNode,
    selectedNodeCount,
    ndvNodeId,
    ndvNode,
    isNodeDetailsOpen,
    openNodeDetails,
    closeNodeDetails,
    hasClipboard,
    canUndo,
    canRedo,
    isDirty,
    resetEditorState,
    markDirty,
    markClean,
    createNewWorkflow,
    addNode,
    updateNode,
    removeNode,
    removeSelectedNodes,
    nudgeSelected,
    bindCredential,
    addConnection,
    removeConnection,
    selectNode,
    selectNodes,
    toggleNodeSelection,
    selectAll,
    clearSelection,
    copySelected,
    paste,
    duplicateSelected,
    addStickyNote,
    updateStickyNote,
    autoLayout,
    undo,
    redo,
    beginNodeDrag,
    setWorkflowName,
  }
})
