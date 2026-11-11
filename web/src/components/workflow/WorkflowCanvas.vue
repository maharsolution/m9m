<script setup lang="ts">
import { computed, markRaw, onMounted, onUnmounted, ref, watch } from 'vue'
import { VueFlow, useVueFlow, SelectionMode } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import type { Node, Edge, Connection } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'

import BaseNode from '@/components/nodes/BaseNode.vue'
import { useWorkflowEditorStore, useWorkflowStore, useNodesStore } from '@/stores'
import { buildFlowEdges, buildFlowNodes, parseEdgeId } from '@/lib/workflowGraph'
import {
  STICKY_NOTE_DEFAULT_HEIGHT,
  STICKY_NOTE_DEFAULT_WIDTH,
  STICKY_NOTE_TYPE,
} from '@/types'
import type { WorkflowNode } from '@/types'

/**
 * Vue Flow's public `Node` input type does not declare `selected`, but
 * the library copies the whole input object onto the graph node — so
 * the flag is honoured at runtime. The local type makes that explicit.
 */
type FlowNode = Node & { selected?: boolean }

const workflowStore = useWorkflowStore()
const workflowEditorStore = useWorkflowEditorStore()
const nodesStore = useNodesStore()

const {
  onNodesChange,
  onEdgesChange,
  onConnect,
  onNodeDragStart,
  onNodeDragStop,
  onSelectionDragStart,
  onSelectionDragStop,
  project,
  zoomIn,
  zoomOut,
  fitView,
  // The canvas mode is written straight into the store: Vue Flow
  // copies the props into its state once, at setup time, so a
  // reactive prop would not switch the mode at runtime.
  panOnDrag,
  selectionKeyCode,
} = useVueFlow()

const flowNodes = computed<FlowNode[]>(() => {
  const nodes = buildFlowNodes(workflowStore.currentWorkflow)
  const workflowNodes = workflowStore.currentWorkflow?.nodes ?? []
  const selected = new Set(workflowEditorStore.selectedNodeIds)

  return nodes.map((node) => {
    const workflowNode = workflowNodes.find((candidate) => candidate.id === node.id)
    const isSticky = workflowNode?.type === STICKY_NOTE_TYPE

    const next: FlowNode = { ...node, selected: selected.has(node.id) }

    if (isSticky) {
      // Sticky notes carry their geometry in `node.parameters` (n8n's
      // wire format). Vue Flow needs it as an explicit size so the
      // card — and the resize handle — match the document.
      const width = Number(workflowNode?.parameters?.width)
      const height = Number(workflowNode?.parameters?.height)
      next.width = Number.isFinite(width) && width > 0 ? width : STICKY_NOTE_DEFAULT_WIDTH
      next.height = Number.isFinite(height) && height > 0 ? height : STICKY_NOTE_DEFAULT_HEIGHT
    }

    return next
  })
})

const flowEdges = computed<Edge[]>(() => buildFlowEdges(workflowStore.currentWorkflow))

// --------------------------------------------------------------------
// Panning vs. marquee selection
//
// n8n's canvas uses the *left* mouse button for the drag-box marquee
// and reserves the middle button (plus the space bar) for panning.
// Vue Flow supports the same split, but the pan mode has to be
// toggled explicitly, because `pan-on-drag` is a static list of mouse
// buttons.
//
// While panning mode is off:
//   * left drag on the empty canvas  -> marquee (box) selection
//   * middle drag                     -> pan
// While panning mode is on (space held):
//   * left drag                       -> pan
// --------------------------------------------------------------------

const panningMode = ref(false)
const isPanningKeyDown = (event: KeyboardEvent) => event.code === 'Space'

/**
 * Writes the current canvas mode into the Vue Flow store.
 *
 * `panOnDrag` lists the mouse buttons that pan the canvas and
 * `selectionKeyCode` arms the marquee:
 *
 *   * marquee mode (default): `panOnDrag = [1]` (middle button pans),
 *     `selectionKeyCode = true` (left drag on the pane draws the
 *     selection rectangle).
 *   * panning mode (space held): `panOnDrag = [0, 1]` (left button
 *     pans as well), `selectionKeyCode = null` (no marquee).
 */
const applyCanvasMode = () => {
  panOnDrag.value = panningMode.value ? [0, 1] : [1]
  selectionKeyCode.value = panningMode.value ? null : true
}

watch(panningMode, applyCanvasMode)

const onKeyDown = (event: KeyboardEvent) => {
  if (isPanningKeyDown(event) && !isEditableTarget(event.target)) {
    panningMode.value = true
    // Stop the browser from scrolling the page while space is held.
    event.preventDefault()
  }
}

const onKeyUp = (event: KeyboardEvent) => {
  if (isPanningKeyDown(event)) {
    panningMode.value = false
  }
}

/** True when the event target is a text input (space must type there). */
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName.toLowerCase()
  return tag === 'input' || tag === 'textarea' || tag === 'select' || target.isContentEditable
}

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('keyup', onKeyUp)
  // Vue Flow applies its props once, when the <VueFlow> component is
  // set up, so the initial mode has to be pushed into the store
  // afterwards.
  applyCanvasMode()
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('keyup', onKeyUp)
  panningMode.value = false
})

/**
 * A drag is one undo step: the pre-drag document is captured once,
 * when the drag starts, instead of on every mouse-move.
 */
let dragSnapshotTaken = false

const captureDragSnapshot = () => {
  if (dragSnapshotTaken) {
    return
  }
  dragSnapshotTaken = true
  workflowEditorStore.beginNodeDrag()
}

const releaseDragSnapshot = () => {
  dragSnapshotTaken = false
}

onNodeDragStart(captureDragSnapshot)
onNodeDragStop(releaseDragSnapshot)
onSelectionDragStart(captureDragSnapshot)
onSelectionDragStop(releaseDragSnapshot)

onNodesChange((changes) => {
  changes.forEach((change) => {
    if (change.type === 'position' && change.position) {
      const node = workflowStore.currentWorkflow?.nodes.find((n) => n.id === change.id)
      if (node) {
        workflowEditorStore.updateNode(change.id, {
          position: [change.position.x, change.position.y],
        })
      }
    }
    if (change.type === 'select') {
      // Vue Flow owns the click / marquee behaviour (shift- or
      // ctrl-click toggles, drag-box selects). The editor store keeps
      // the resulting set of selected ids so the rest of the app
      // (toolbar, node panel, clipboard) can work with it.
      const next = new Set(workflowEditorStore.selectedNodeIds)
      if (change.selected) {
        next.add(change.id)
      } else {
        next.delete(change.id)
      }
      workflowEditorStore.selectNodes([...next])
    }
    if (change.type === 'remove') {
      workflowEditorStore.removeNode(change.id)
    }
  })
})

onEdgesChange((changes) => {
  changes.forEach((change) => {
    if (change.type === 'remove') {
      const edge = parseEdgeId(change.id)
      if (edge) {
        workflowEditorStore.removeConnection(
          edge.sourceNodeId,
          edge.targetNodeId,
          edge.sourceOutput,
          edge.targetInput
        )
      }
    }
  })
})

onConnect((params: Connection) => {
  if (!params.source || !params.target) return
  // Vue Flow's Connection params use sourceHandle / targetHandle as the
  // handle IDs we assigned in BaseNode.vue (`output-0` / `input-0`).
  // When the user drops on the default handle (no specific handle ID),
  // sourceHandle / targetHandle can be null — fall back to the first
  // port of each side.
  const parseHandle = (
    raw: string | null | undefined,
    prefix: 'input' | 'output'
  ): number => {
    if (!raw) return 0
    const stripped = raw.replace(`${prefix}-`, '')
    const n = parseInt(stripped, 10)
    return Number.isNaN(n) ? 0 : n
  }
  const sourceOutput = parseHandle(params.sourceHandle, 'output')
  const targetInput = parseHandle(params.targetHandle, 'input')

  // Don't allow self-connections
  if (params.source === params.target) return

  workflowEditorStore.addConnection(
    params.source,
    params.target,
    sourceOutput,
    targetInput
  )
})

const onDrop = (event: DragEvent) => {
  const nodeType = event.dataTransfer?.getData('application/vueflow')
  if (!nodeType) return

  const nodeTypeInfo = nodesStore.getNodeType(nodeType)
  if (!nodeTypeInfo) return

  const { left, top } = (event.target as HTMLElement).getBoundingClientRect()
  const position = project({
    x: event.clientX - left,
    y: event.clientY - top,
  })

  // Ensure a unique node `name` (used as the connections map key).
  // If two nodes of the same type end up with the same name, the
  // connections map collapses them into one entry and Vue Flow
  // can't draw the right edge.
  //
  // Naming convention follows n8n's UI: the first instance keeps
  // the bare display name ("HTTP Request"), and subsequent
  // duplicates get a numeric suffix with no space
  // ("HTTP Request1", "HTTP Request2"). This matches what the
  // sync-service/sync.py bridge pushes in for workflows imported
  // from n8n — i.e. what n8n's API returns for nodes duplicated
  // inside its own editor — so a workflow built in the m9m UI
  // and a workflow imported from n8n produce the same connection
  // keys, and round-tripping through sync.py doesn't break
  // anything.
  const baseName = nodeTypeInfo.displayName
  const existingNames = new Set(
    (workflowStore.currentWorkflow?.nodes ?? []).map((n) => n.name)
  )
  let name = baseName
  let counter = 1
  while (existingNames.has(name)) {
    name = `${baseName}${counter++}`
  }

  const newNode: WorkflowNode = {
    id: `node_${Date.now()}`,
    name,
    type: nodeType,
    typeVersion: nodeTypeInfo.version,
    position: [position.x, position.y],
    parameters: {},
  }

  workflowEditorStore.addNode(newNode)
}

const onDragOver = (event: DragEvent) => {
  event.preventDefault()
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'move'
  }
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const nodeTypes: Record<string, any> = {
  custom: markRaw(BaseNode),
}

const getMinimapNodeColor = (node: Node) => {
  const category = node.data?.category
  switch (category) {
    case 'trigger': return '#10b981'
    case 'action': return '#6366f1'
    case 'transform': return '#f59e0b'
    default: return '#94a3b8'
  }
}

// --------------------------------------------------------------------
// Viewport helpers (exposed to the editor view so the keyboard
// shortcuts can drive the zoom).
// --------------------------------------------------------------------

const zoomInCanvas = () => {
  void zoomIn({ duration: 150 })
}

const zoomOutCanvas = () => {
  void zoomOut({ duration: 150 })
}

const resetZoomCanvas = () => {
  void fitView({ padding: 0.2, duration: 200 })
}

defineExpose({ zoomIn: zoomInCanvas, zoomOut: zoomOutCanvas, resetZoom: resetZoomCanvas })
</script>

<template>
  <div
    class="w-full h-full"
    @drop="onDrop"
    @dragover="onDragOver"
  >
    <!--
      Canvas behaviour:
        * `selection-mode="partial"` + `multi-selection-key-code` make
          shift- / ctrl-click toggle a node in / out of the selection
          and drag a box on the pane to marquee-select.
        * `delete-key-code` is disabled because deleting is handled by
          the editor store, so the deletion ends up in the undo history.
        * `disable-keyboard-a11y` is on because the editor implements
          its own keyboard handling (delete, nudge, escape, …).
    -->
    <VueFlow
      :nodes="flowNodes"
      :edges="flowEdges"
      :node-types="nodeTypes"
      :default-viewport="{ x: 100, y: 100, zoom: 1 }"
      :min-zoom="0.2"
      :max-zoom="2"
      :snap-to-grid="true"
      :snap-grid="[20, 20]"
      :connection-radius="120"
      :elevate-edges-on-select="true"
      :connection-line-style="{ stroke: '#6366f1', strokeWidth: 3 }"
      :default-edge-options="{ style: { stroke: '#94a3b8', strokeWidth: 2 } }"
      :fit-view-on-init-options="{ padding: 0.2, minZoom: 0.6, maxZoom: 1, includeHiddenNodes: false }"
      fit-view-on-init
      class="bg-[#f5f5f7] dark:bg-slate-900"
      :selection-mode="SelectionMode.Partial"
      multi-selection-key-code="['Shift', 'Control', 'Meta']"
      :delete-key-code="null"
      :disable-keyboard-a11y="true"
    >
      <Background
        :pattern-color="'#cbd5e1'"
        :gap="20"
      />
      <Controls
        class="!bg-white dark:!bg-slate-800 !border-slate-200 dark:!border-slate-700 !shadow-lg"
      />
      <MiniMap
        :node-color="getMinimapNodeColor"
        class="!bg-white dark:!bg-slate-800 !border-slate-200 dark:!border-slate-700 !shadow-lg"
      />
    </VueFlow>

    <!-- Selection badge -->
    <div
      v-if="workflowEditorStore.selectedNodeCount > 1"
      class="absolute top-3 left-3 z-10 pointer-events-none px-2.5 py-1 rounded-full text-xs font-medium bg-primary-100 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300 shadow-sm"
    >
      {{ workflowEditorStore.selectedNodeCount }} nodes selected
    </div>

    <!-- Empty state overlay -->
    <div
      v-if="!workflowStore.currentWorkflow?.nodes.length"
      class="absolute inset-0 flex items-center justify-center pointer-events-none"
    >
      <div class="text-center">
        <div class="w-16 h-16 mx-auto mb-4 rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center">
          <svg class="w-8 h-8 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
          </svg>
        </div>
        <h3 class="text-lg font-medium text-slate-700 dark:text-slate-300">
          Drag nodes here to build your workflow
        </h3>
        <p class="mt-2 text-sm text-slate-500 dark:text-slate-400">
          Select nodes from the palette on the left
        </p>
      </div>
    </div>
  </div>
</template>

<style>
/* Vue Flow custom styles */
.vue-flow__node {
  @apply transition-shadow duration-150;
}

.vue-flow__node.selected {
  @apply ring-2 ring-primary-500 ring-offset-2 dark:ring-offset-slate-900;
}

.vue-flow__edge-path {
  @apply stroke-slate-400 dark:stroke-slate-500;
  stroke-width: 2;
}

.vue-flow__edge.selected .vue-flow__edge-path {
  @apply stroke-primary-500;
  stroke-width: 3;
}

.vue-flow__handle {
  /* The component-level .connection-handle rule in BaseNode.vue sets
     the actual visual size + the hit-area child element. This global
     rule is kept as a fallback for any handle that isn't a BaseNode. */
  @apply transition-all duration-150;
}

.vue-flow__handle:hover {
  @apply !bg-primary-500 scale-125;
}

.vue-flow__handle-connecting {
  @apply !bg-primary-500;
}

.vue-flow__handle-valid {
  @apply !bg-green-500;
}
</style>
