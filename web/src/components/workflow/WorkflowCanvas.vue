<script setup lang="ts">
import { computed, markRaw, onMounted, onUnmounted, ref, watch } from 'vue'
import { VueFlow, useVueFlow, SelectionMode } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import type { Node, Edge, Connection, NodeMouseEvent } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'

import BaseNode from '@/components/nodes/BaseNode.vue'
import { useWorkflowEditorStore, useWorkflowStore, useNodesStore } from '@/stores'
import { useThemeStore } from '@/stores/theme'
import { buildFlowEdges, buildFlowNodes, parseEdgeId } from '@/lib/workflowGraph'
import { createDoubleClickDetector } from '@/lib/doubleClick'
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
const themeStore = useThemeStore()

/**
 * Canvas chrome colours.
 *
 * The canvas reads from the semantic token layer when it is published
 * (`--canvas--color--background`, `--canvas--dot--color`) and falls
 * back to the n8n values otherwise. The theme is part of the key so
 * the fallback flips with the light / dark switch.
 */
const canvasBackground = computed(() => {
  const fallback = themeStore.isDark ? 'oklch(20.46% 0 89.88)' : 'hsl(0, 0%, 96%)'
  return `var(--canvas--color--background, ${fallback})`
})

const canvasDotColor = computed(() => {
  // Re-read on every theme change so a published token layer wins.
  void themeStore.isDark
  if (typeof window === 'undefined') {
    return 'oklch(68.3% 0 89.88)'
  }
  const published = getComputedStyle(document.documentElement)
    .getPropertyValue('--canvas--dot--color')
    .trim()
  if (published) {
    return published
  }
  return themeStore.isDark ? 'oklch(38.67% 0 89.88)' : 'oklch(68.3% 0 89.88)'
})

const {
  onNodesChange,
  onEdgesChange,
  onConnect,
  onNodeDragStart,
  onNodeDragStop,
  onSelectionDragStart,
  onSelectionDragStop,
  screenToFlowCoordinate,
  zoomIn,
  zoomOut,
  fitView,
  // The canvas mode is written straight into the store: Vue Flow
  // copies the props into its state once, at setup time, so a
  // reactive prop would not switch the mode at runtime.
  panOnDrag,
  selectionKeyCode,
  panActivationKeyCode,
  zoomActivationKeyCode,
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
// n8n's canvas (n8n/packages/frontend/editor-ui/src/features/
// workflows/canvas/components/Canvas.vue) splits the pointer like this:
//
//   * middle mouse button            -> pan  (`panningMouseButton = [1]`)
//   * space / ctrl (⌘ on macOS)      -> panning mode (`panningKeyCode`)
//   * left drag on the empty canvas  -> the marquee rectangle on
//                                        desktop (`selectionKeyCode =
//                                        true`), the pan gesture on
//                                        mobile (`selectionKeyCode =
//                                        'Shift'`)
//
// While the panning key is held, n8n swaps the modes
// (`switchToPanningMode` / `switchToSelectionMode`): the marquee is
// disarmed and the left button pans.
//
// m9m follows n8n's *mobile* split, because the requested journey is
// "drag the empty canvas and the whole workflow moves with the
// pointer":
//
//   * left drag on the empty canvas  -> pan
//   * shift + left drag              -> marquee rectangle
//   * middle drag                     -> pan
//   * space / ctrl / ⌘ + drag        -> pan (marquee disarmed)
//
// Vue Flow expresses this with two store values: `panOnDrag` (the
// mouse buttons that pan the canvas) and `selectionKeyCode` (the key
// that arms the marquee). Both have to be written into the store —
// Vue Flow copies its props into the state once, at setup time.
// --------------------------------------------------------------------

/** The modifier n8n uses for the panning mode (⌘ on macOS). */
const PANNING_MODIFIER =
  typeof navigator !== 'undefined' && /mac|iphone|ipad/i.test(navigator.userAgent)
    ? 'Meta'
    : 'Control'

const panningMode = ref(false)

const isPanningKeyDown = (event: KeyboardEvent) =>
  event.code === 'Space' ||
  event.key === PANNING_MODIFIER ||
  event.code === PANNING_MODIFIER

/**
 * Writes the current canvas mode into the Vue Flow store.
 *
 * `panOnDrag` lists the mouse buttons that pan the canvas and
 * `selectionKeyCode` arms the marquee:
 *
 *   * default: `panOnDrag = [0, 1]` (left + middle drag pan),
 *              `selectionKeyCode = 'Shift'` (shift + left drag draws
 *              the marquee rectangle).
 *   * panning mode (space / ctrl held): `panOnDrag = [0, 1]`,
 *              `selectionKeyCode = null` (no marquee at all).
 */
const applyCanvasMode = () => {
  panOnDrag.value = [0, 1]
  selectionKeyCode.value = panningMode.value ? null : 'Shift'
}

watch(panningMode, applyCanvasMode)

const onKeyDown = (event: KeyboardEvent) => {
  if (isPanningKeyDown(event) && !isEditableTarget(event.target)) {
    panningMode.value = true
    // Stop the browser from scrolling the page while space is held.
    if (event.code === 'Space') {
      event.preventDefault()
    }
  }
}

const onKeyUp = (event: KeyboardEvent) => {
  if (isPanningKeyDown(event)) {
    panningMode.value = false
  }
}

/**
 * A key release can be missed when the window loses the focus while
 * the modifier is held. n8n resets the canvas mode on blur
 * (`onWindowBlur` -> `switchToSelectionMode`), m9m does the same.
 */
const onWindowBlur = () => {
  panningMode.value = false
}

/** True when the event target is a text input (space must type there). */
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName.toLowerCase()
  return tag === 'input' || tag === 'textarea' || tag === 'select' || target.isContentEditable
}

/**
 * The selection ring is drawn inside the node, and the node is scaled
 * by the canvas zoom. Without a compensation factor the 6px ring turns
 * into a 12px ring at 200% zoom and a 1.2px hairline at 20% - n8n
 * keeps the ring at a constant 6px on screen. The factor is the
 * inverse of the zoom, published as a CSS variable on the canvas
 * root; `--canvas-zoom-compensation-factor` is consumed by the node
 * styles (`0 0 0 calc(6px * var(--canvas-zoom-compensation-factor, 1))`).
 */
const canvasRoot = ref<HTMLElement | null>(null)

const publishZoomCompensation = (zoom: number) => {
  if (!canvasRoot.value || !Number.isFinite(zoom) || zoom <= 0) return
  canvasRoot.value.style.setProperty(
    '--canvas-zoom-compensation-factor',
    String(1 / zoom),
  )
}

const onViewportChange = (viewport: { zoom: number }) => {
  publishZoomCompensation(viewport.zoom)
}

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('keyup', onKeyUp)
  window.addEventListener('blur', onWindowBlur)
  // Vue Flow applies its props once, when the <VueFlow> component is
  // set up, so the initial mode has to be pushed into the store
  // afterwards.
  applyCanvasMode()
  // n8n arms the same keys for panning *and* for zooming
  // (`:pan-activation-key-code="panningKeyCode"` and
  //  `:zoom-activation-key-code="panningKeyCode"` in Canvas.vue), so
  // space / ctrl + drag pans and space / ctrl + wheel zooms.
  panActivationKeyCode.value = ['Space', PANNING_MODIFIER]
  zoomActivationKeyCode.value = ['Space', PANNING_MODIFIER]
  window.addEventListener('pointerdown', onPointerDown, true)
  window.addEventListener('pointermove', onPointerMove, true)
  window.addEventListener('pointerup', onPointerUp, true)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('keyup', onKeyUp)
  window.removeEventListener('blur', onWindowBlur)
  window.removeEventListener('pointerdown', onPointerDown, true)
  window.removeEventListener('pointermove', onPointerMove, true)
  window.removeEventListener('pointerup', onPointerUp, true)
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

// --------------------------------------------------------------------
// Node interaction — click selects, double click opens the NDV
//
// n8n parity: a single click on a node card only *selects* it (the
// 6px ring on the card); the node details view opens on a double
// click.
//
// The browser's `dblclick` event is deliberately not used: a node
// drag that ends on the same card can produce a `dblclick`, which
// would open the NDV in the middle of a drag. Every pointer press is
// measured instead — a press that travels more than a few pixels is
// a drag and never counts as a click.
// --------------------------------------------------------------------

const doubleClick = createDoubleClickDetector()

const onPointerDown = (event: PointerEvent) => {
  doubleClick.press(event.clientX, event.clientY)
}

const onPointerMove = (event: PointerEvent) => {
  doubleClick.move(event.clientX, event.clientY)
}

const onPointerUp = () => {
  doubleClick.release()
}

/**
 * Vue Flow reports every click on a node card. Two of them, in quick
 * succession on the same node, open the node details view.
 *
 * Sticky notes are excluded: a double click on a sticky note edits
 * its content (n8n parity), it never opens the NDV.
 */
const onNodeClick = ({ event, node }: NodeMouseEvent) => {
  const workflowNode = workflowStore.currentWorkflow?.nodes.find((n) => n.id === node.id)
  if (workflowNode?.type === STICKY_NOTE_TYPE) {
    doubleClick.reset()
    return
  }

  // Touch events carry no pointer coordinates — they never form a
  // double click.
  if (!('clientX' in event)) return

  if (!doubleClick.click(node.id, event.timeStamp, event.clientX, event.clientY)) {
    return
  }

  workflowEditorStore.openNodeDetails(node.id)
}

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

  // The node lands where the pointer is. `screenToFlowCoordinate`
  // projects the screen position onto the canvas: it takes the
  // container bounds and the viewport transform (pan + zoom) into
  // account, so a drop is correct after the canvas was panned or
  // zoomed. The result is snapped to the canvas grid, like n8n.
  const position = screenToFlowCoordinate({
    x: event.clientX,
    y: event.clientY,
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
    ref="canvasRoot"
    class="w-full h-full"
    @drop="onDrop"
    @dragover="onDragOver"
  >
    <!--
      Arrow marker for the connections. SVG markers are resolved
      document-wide by id, so one hidden <defs> serves every edge.
    -->
    <svg class="m9m-marker-defs" aria-hidden="true" focusable="false">
      <defs>
        <marker
          id="m9m-arrow"
          viewBox="0 0 10 10"
          refX="9"
          refY="5"
          markerWidth="10"
          markerHeight="10"
          markerUnits="userSpaceOnUse"
          orient="auto"
        >
          <path d="M 0 0 L 10 5 L 0 10 z" style="fill: var(--background--brand, oklch(78.07% 0.136 5.02))" />
        </marker>
        <marker
          id="m9m-arrow-selected"
          viewBox="0 0 10 10"
          refX="9"
          refY="5"
          markerWidth="10"
          markerHeight="10"
          markerUnits="userSpaceOnUse"
          orient="auto"
        >
          <path d="M 0 0 L 10 5 L 0 10 z" style="fill: var(--background--brand--active, oklch(59.09% 0.1611 5.09))" />
        </marker>
      </defs>
    </svg>

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
      :snap-grid="[16, 16]"
      :connection-radius="120"
      :elevate-edges-on-select="true"
      :connection-line-style="{ stroke: 'var(--background--brand, oklch(78.07% 0.136 5.02))', strokeWidth: 3 }"
      :default-edge-options="{ style: { stroke: 'var(--background--brand, oklch(78.07% 0.136 5.02))', strokeWidth: 2 } }"
      :fit-view-on-init-options="{ padding: 0.2, minZoom: 0.6, maxZoom: 1, includeHiddenNodes: false }"
      fit-view-on-init
      :style="{ backgroundColor: canvasBackground }"
      class="vue-flow--m9m"
      :selection-mode="SelectionMode.Partial"
      multi-selection-key-code="['Shift', 'Control', 'Meta']"
      :delete-key-code="null"
      :disable-keyboard-a11y="true"
      @viewport-change="onViewportChange"
      @node-click="onNodeClick"
    >
      <Background
        :color="canvasDotColor"
        :gap="16"
        :size="1"
      />
      <Controls
        class="m9m-controls"
      />
      <MiniMap
        :node-color="getMinimapNodeColor"
        :height="120"
        :node-border-radius="4"
        class="m9m-minimap"
      />
    </VueFlow>

    <!-- Selection badge -->
    <div
      v-if="workflowEditorStore.selectedNodeCount > 1"
      class="absolute top-3 left-3 z-10 pointer-events-none px-2.5 py-1 rounded-full text-xs font-medium bg-surface text-secondary border border-default shadow-sm"
    >
      {{ workflowEditorStore.selectedNodeCount }} nodes selected
    </div>

    <!-- Empty state overlay -->
    <div
      v-if="!workflowStore.currentWorkflow?.nodes.length"
      class="absolute inset-0 flex items-center justify-center pointer-events-none"
    >
      <div class="text-center">
        <div class="w-16 h-16 mx-auto mb-4 rounded-full bg-subtle flex items-center justify-center">
          <svg class="w-8 h-8 text-tertiary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
          </svg>
        </div>
        <h3 class="text-lg font-semibold tracking-tight text-primary">
          Drag nodes here to build your workflow
        </h3>
        <p class="mt-2 text-sm text-tertiary">
          Open the node creator to pick a node
        </p>
      </div>
    </div>
  </div>
</template>

<style>
/*
 * Vue Flow chrome — n8n parity.
 *
 * The canvas is tonal: a dotted background, hairline borders, and
 * shadows only on the pieces that float above the canvas (the
 * controls pill and the minimap).
 */

/* The hidden <defs> block that carries the arrow markers. */
.m9m-marker-defs {
  position: absolute;
  width: 0;
  height: 0;
  pointer-events: none;
}

/*
 * Node selection is a ring painted on the card itself (see
 * BaseNode.vue). Nothing is added here — one cue per state.
 */
.vue-flow__node {
  transition: none;
}

/*
 * The dotted background is decoration, not a surface. It must not
 * swallow the pointer: Vue Flow's pane needs the press to draw the
 * marquee rectangle, the click to clear the selection and the drag
 * to pan the canvas.
 */
.vue-flow__background {
  pointer-events: none;
}

/*
 * The pointer follows the canvas mode (n8n parity):
 *   * `grab` / `grabbing` while the canvas can be panned
 *   * `crosshair` while the marquee rectangle is armed (shift held)
 */
.vue-flow--m9m.vue-flow .vue-flow__pane.draggable {
  cursor: grab;
}

.vue-flow--m9m.vue-flow .vue-flow__pane.dragging {
  cursor: grabbing;
}

.vue-flow--m9m.vue-flow .vue-flow__pane.selection {
  cursor: crosshair;
}

/*
 * Connections use the primary (brand) family, with an arrow marker
 * at the target end. The stroke is set with `!important` because
 * Vue Flow writes the edge colour as an inline style.
 */
.vue-flow__edge-path {
  stroke: var(--background--brand, oklch(78.07% 0.136 5.02)) !important;
  stroke-width: 2;
  marker-end: url(#m9m-arrow);
}

.vue-flow__edge.selected .vue-flow__edge-path {
  stroke: var(--background--brand--active, oklch(59.09% 0.1611 5.09)) !important;
  stroke-width: 3;
  marker-end: url(#m9m-arrow-selected);
}

/*
 * Controls — n8n's horizontal pill: surface fill, 1px border,
 * hairline shadow, 16px icons.
 */
.vue-flow__controls {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 2px;
  padding: 4px;
  border-radius: 9999px;
  border: 1px solid var(--border-color, oklch(0% 0 0 / 0.1));
  background: var(--background--surface, #ffffff);
  box-shadow: var(--shadow--xs, 0 1px 2px -1px oklch(0% 0 0 / 0.1));
  overflow: hidden;
}

.vue-flow__controls-button {
  width: 24px;
  height: 24px;
  min-width: 24px;
  min-height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: 9999px;
  background: transparent;
  color: var(--text-color, oklch(38.67% 0 89.88));
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.vue-flow__controls-button svg {
  width: 16px;
  height: 16px;
  fill: currentColor;
}

.vue-flow__controls-button:hover {
  background: var(--background--hover, oklch(0% 0 0 / 0.05));
}

.vue-flow__controls-button + .vue-flow__controls-button {
  border-left: none;
}

/*
 * Minimap — 120px tall, 8px radius, 1px border, subtle fill.
 */
.vue-flow__minimap {
  height: 120px !important;
  border-radius: 8px;
  border: 1px solid var(--border-color, oklch(0% 0 0 / 0.1));
  background: var(--background--subtle, oklch(97.02% 0 89.88));
  box-shadow: none;
  overflow: hidden;
}

.vue-flow__minimap-mask {
  fill: oklch(20.46% 0 89.88 / 0.08);
}

/*
 * Handles — the visible indicator is styled by BaseNode.vue; these
 * are the fallbacks for any handle that isn't a BaseNode.
 */
.vue-flow__handle {
  transition: none;
}

@media (max-width: 767px) {
  .vue-flow__minimap {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .vue-flow__controls-button {
    transition-duration: 1ms;
  }
}
</style>
