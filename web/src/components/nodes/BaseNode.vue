<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import '@vue-flow/node-resizer/dist/style.css'
import { XMarkIcon, KeyIcon } from '@heroicons/vue/24/outline'
import { useWorkflowEditorStore } from '@/stores'
import {
  STICKY_NOTE_COLORS,
  STICKY_NOTE_DEFAULT_HEIGHT,
  STICKY_NOTE_DEFAULT_WIDTH,
  STICKY_NOTE_TYPE,
  stickyNoteColor,
} from '@/types'
import type { NodeCategory } from '@/types'

interface Props {
  id: string
  data: {
    label: string
    nodeType: string
    category: NodeCategory
    parameters?: Record<string, unknown>
    credentials?: Record<string, { id: string; name: string }>
    // 'compact' (default) is the editor-friendly 200/260px wide
    // card. 'comfortable' widens the card and grows the icon /
    // label for screens where the user reads the canvas (e.g.
    // the execution detail view), making each node easier to
    // click. The Canvas (editor / execution) sets this when it
    // builds the Vue Flow nodes — see `buildFlowNodes` /
    // ExecutionDetail.vue.
    density?: 'compact' | 'comfortable'
  }
  selected?: boolean
}

const props = defineProps<Props>()
const editorStore = useWorkflowEditorStore()

const isSticky = computed(() => props.data.nodeType === STICKY_NOTE_TYPE)

const categoryStyles = computed(() => {
  switch (props.data.category) {
    case 'trigger':
      return {
        border: 'border-l-green-500',
        icon: 'bg-green-500',
        iconText: '⚡',
      }
    case 'action':
      return {
        border: 'border-l-indigo-500',
        icon: 'bg-indigo-500',
        iconText: '▶',
      }
    case 'transform':
      return {
        border: 'border-l-amber-500',
        icon: 'bg-amber-500',
        iconText: '🔄',
      }
    case 'flow':
      return {
        border: 'border-l-purple-500',
        icon: 'bg-purple-500',
        iconText: '🔀',
      }
    default:
      return {
        border: 'border-l-slate-500',
        icon: 'bg-slate-500',
        iconText: '📦',
      }
  }
})

const nodeTypeName = computed(() => {
  const type = props.data.nodeType
  // Extract display name from type like "n8n-nodes-base.httpRequest"
  const parts = type.split('.')
  return parts[parts.length - 1]
    .replace(/([A-Z])/g, ' $1')
    .replace(/^./, (str) => str.toUpperCase())
    .trim()
})

const credentialCount = computed(() => {
  return props.data.credentials
    ? Object.keys(props.data.credentials).length
    : 0
})

const deleteNode = (event: MouseEvent) => {
  event.stopPropagation()
  event.preventDefault()
  editorStore.removeNode(props.id)
}

// --------------------------------------------------------------------
// Sticky note
// --------------------------------------------------------------------

const stickyContent = computed(() => {
  const content = props.data.parameters?.content
  return typeof content === 'string' ? content : ''
})

const stickyBackground = computed(() =>
  stickyNoteColor(props.data.parameters?.color)
)

const stickyWidth = computed(() => {
  const width = Number(props.data.parameters?.width)
  return Number.isFinite(width) && width > 0 ? width : STICKY_NOTE_DEFAULT_WIDTH
})

const stickyHeight = computed(() => {
  const height = Number(props.data.parameters?.height)
  return Number.isFinite(height) && height > 0 ? height : STICKY_NOTE_DEFAULT_HEIGHT
})

const editing = ref(false)
const draft = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)

const startEditing = () => {
  draft.value = stickyContent.value
  editing.value = true
  nextTick(() => {
    textareaRef.value?.focus()
    textareaRef.value?.select()
  })
}

const commitEditing = () => {
  if (!editing.value) {
    return
  }
  editing.value = false
  if (draft.value !== stickyContent.value) {
    editorStore.updateStickyNote(props.id, { content: draft.value })
  }
}

const cancelEditing = () => {
  editing.value = false
}

const setStickyColor = (index: number) => {
  editorStore.updateStickyNote(props.id, { color: index })
}

/**
 * Keeps the note's geometry in the workflow document. n8n stores the
 * sticky-note width / height inside `node.parameters`, so the note
 * survives an export / import round-trip unchanged.
 */
const onStickyResize = (payload: { params: { width: number; height: number } }) => {
  editorStore.updateStickyNote(props.id, {
    width: Math.round(payload.params.width),
    height: Math.round(payload.params.height),
  })
}
</script>

<template>
  <!-- Sticky note. n8n's `n8n-nodes-base.stickyNote` — content, colour
       and geometry live in `node.parameters`, exactly like upstream
       n8n, so the workflow document stays n8n-compatible. -->
  <div
    v-if="isSticky"
    class="sticky-note"
    :class="selected ? 'selected' : ''"
    :style="{
      backgroundColor: stickyBackground,
      width: `${stickyWidth}px`,
      height: `${stickyHeight}px`,
    }"
  >
    <NodeResizer
      :is-visible="selected"
      :min-width="120"
      :min-height="80"
      color="#6366f1"
      @resize="onStickyResize"
    />

    <!-- Delete affordance -->
    <button
      @click="deleteNode"
      title="Delete sticky note"
      aria-label="Delete sticky note"
      class="node-delete-btn"
    >
      <XMarkIcon class="w-3.5 h-3.5" />
    </button>

    <!-- Colour swatches (n8n offers seven). -->
    <div
      v-if="selected || editing"
      class="sticky-note__colors"
      @mousedown.stop
    >
      <button
        v-for="color in STICKY_NOTE_COLORS"
        :key="color.index"
        type="button"
        class="sticky-note__swatch"
        :class="props.data.parameters?.color === color.index ? 'selected' : ''"
        :style="{ backgroundColor: color.hex }"
        :title="color.name"
        @click.stop="setStickyColor(color.index)"
      />
    </div>

    <textarea
      v-if="editing"
      ref="textareaRef"
      v-model="draft"
      class="sticky-note__editor"
      placeholder="Write a note..."
      @mousedown.stop
      @click.stop
      @dblclick.stop
      @keydown.esc.stop.prevent="cancelEditing"
      @keydown.ctrl.enter.stop.prevent="commitEditing"
      @keydown.meta.enter.stop.prevent="commitEditing"
      @blur="commitEditing"
    />
    <div
      v-else
      class="sticky-note__content"
      :title="'Double click to edit'"
      @dblclick.stop="startEditing"
    >
      {{ stickyContent }}
    </div>
  </div>

  <!-- Regular node -->
  <div
    v-else
    :class="[
      // Width and minimum height vary with density. The compact
      // (default) size keeps the editor canvas dense; comfortable
      // widens the card and bumps the min-height so the execution
      // canvas reads more like an inspector and is easier to click
      // (user feedback Sep 2026 — execution nodes were too small).
      data.density === 'comfortable'
        ? 'workflow-node workflow-node--comfortable min-w-[280px] max-w-[340px] min-h-[110px]'
        : 'workflow-node min-w-[200px] max-w-[260px] min-h-[80px]',
      categoryStyles.border,
      'border-l-4',
      selected ? 'selected' : ''
    ]"
  >
    <!-- Input Handle. The :id binding (not the static `id`
         attribute) is required because Vue Flow's <Handle> declares
         `id` as a prop. Using `id="input-0"` lets Vue's attribute
         inheritance push it onto the DOM root (which is just an
         empty div, so the prop never reaches the component),
         resulting in Vue Flow rendering edges from the node center
         because both handles have null id. The :id binding makes
         it an explicit prop binding and forces the id through. -->
    <Handle
      v-if="data.category !== 'trigger'"
      :id="'input-0'"
      type="target"
      :position="Position.Left"
      :connectable="true"
      :is-connectable="true"
      class="connection-handle connection-handle--input"
    >
      <!-- Larger transparent hit-area — sits above the visible dot in
           stacking order but does NOT capture pointer events (pointer-
           events: none in CSS below). This lets users grab the handle
           with ~40px of slop on either side without breaking drag. -->
      <span class="handle-hit-area" aria-hidden="true" />
    </Handle>

    <!-- Delete affordance (top-right, hover-reveal) -->
    <button
      @click="deleteNode"
      title="Delete node"
      class="node-delete-btn"
      aria-label="Delete node"
    >
      <XMarkIcon class="w-3.5 h-3.5" />
    </button>

    <!-- Node Content. Padding, icon size, and font size all
         scale with density so the "comfortable" card doesn't end
         up with cramped content. We keep the editor's compact
         defaults by NOT touching the inner classes when density
         is compact. -->
    <div :class="data.density === 'comfortable' ? 'p-4' : 'p-3'">
      <div class="flex items-center gap-2">
        <div
          :class="[
            categoryStyles.icon,
            data.density === 'comfortable'
              ? 'w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0'
              : 'w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0',
          ]"
        >
          <span :class="data.density === 'comfortable' ? 'text-white text-base' : 'text-white text-sm'">{{ categoryStyles.iconText }}</span>
        </div>
        <div class="min-w-0 flex-1">
          <h4 :class="data.density === 'comfortable' ? 'text-base font-semibold text-slate-900 dark:text-white truncate' : 'text-sm font-semibold text-slate-900 dark:text-white truncate'">
            {{ data.label }}
          </h4>
          <p :class="data.density === 'comfortable' ? 'text-sm text-slate-500 dark:text-slate-400 truncate' : 'text-xs text-slate-500 dark:text-slate-400 truncate'">
            {{ nodeTypeName }}
          </p>
        </div>
      </div>

      <!-- Credential badge -->
      <div
        v-if="credentialCount > 0"
        :class="[
          'mt-2 pt-2 border-t border-slate-200 dark:border-slate-600 flex items-center gap-1',
          data.density === 'comfortable'
            ? 'text-sm text-slate-600 dark:text-slate-300'
            : 'text-xs text-slate-600 dark:text-slate-300',
        ]"
      >
        <KeyIcon :class="data.density === 'comfortable' ? 'w-3.5 h-3.5' : 'w-3 h-3'" />
        <span>{{ credentialCount }} credential{{ credentialCount > 1 ? 's' : '' }}</span>
      </div>

      <!-- Parameters preview -->
      <div
        v-if="data.parameters && Object.keys(data.parameters).length > 0"
        class="mt-2 pt-2 border-t border-slate-200 dark:border-slate-600"
      >
        <div
          v-for="(value, key) in data.parameters"
          :key="key"
          :class="[
            data.density === 'comfortable'
              ? 'text-sm text-slate-500 dark:text-slate-400 truncate'
              : 'text-xs text-slate-500 dark:text-slate-400 truncate',
          ]"
        >
          <span class="font-medium">{{ key }}:</span>
          {{ typeof value === 'object' ? '...' : value }}
        </div>
      </div>
    </div>

    <!-- Output Handle. See note on the input handle above re:
         why this uses :id instead of static `id`. -->
    <Handle
      :id="'output-0'"
      type="source"
      :position="Position.Right"
      :connectable="true"
      :is-connectable="true"
      class="connection-handle connection-handle--output"
    >
      <span class="handle-hit-area" aria-hidden="true" />
    </Handle>
  </div>
</template>

<style scoped>
.workflow-node {
  @apply bg-white dark:bg-slate-800 rounded-lg;
  @apply border-2 border-slate-200 dark:border-slate-600;
  @apply shadow-md;
  @apply transition-all duration-150;
  /* Make sure handle hit areas are not clipped by rounded corners. */
  overflow: visible;
}

.workflow-node:hover {
  @apply border-primary-400 dark:border-primary-500;
  @apply shadow-lg;
}

.workflow-node.selected {
  @apply border-primary-500 dark:border-primary-400;
}

/*
 * Sticky note. The card fills the Vue Flow node wrapper — the wrapper
 * is sized from `parameters.width` / `parameters.height` (n8n keeps the
 * geometry in the node parameters) — so dragging the resize handle
 * updates the document and the card follows.
 */
.sticky-note {
  width: 100%;
  height: 100%;
  min-width: 120px;
  min-height: 80px;
  border-radius: 8px;
  border: 1px solid rgba(15, 23, 42, 0.12);
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.12);
  padding: 12px;
  overflow: hidden;
  position: relative;
  cursor: default;
}

.sticky-note.selected {
  border-color: #6366f1;
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.35);
}

.sticky-note__content {
  width: 100%;
  height: 100%;
  overflow: hidden;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  line-height: 1.45;
  color: #1e293b;
  cursor: text;
}

.sticky-note__editor {
  width: 100%;
  height: 100%;
  resize: none;
  border: none;
  outline: none;
  background: transparent;
  font-size: 13px;
  line-height: 1.45;
  color: #1e293b;
  font-family: inherit;
}

.sticky-note__colors {
  position: absolute;
  top: -34px;
  left: 0;
  display: flex;
  gap: 4px;
  padding: 4px;
  border-radius: 8px;
  background: #ffffff;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.18);
  z-index: 10;
}

.sticky-note__swatch {
  width: 16px;
  height: 16px;
  border-radius: 9999px;
  border: 1px solid rgba(15, 23, 42, 0.2);
  cursor: pointer;
  padding: 0;
}

.sticky-note__swatch.selected {
  box-shadow: 0 0 0 2px #6366f1;
}

/* Delete button — top-right corner, hidden by default, revealed on hover. */
.node-delete-btn {
  position: absolute;
  top: -8px;
  right: -8px;
  width: 20px;
  height: 20px;
  border-radius: 9999px;
  background: #ef4444;
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transform: scale(0.85);
  transition: opacity 120ms ease, transform 120ms ease;
  z-index: 5;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.15);
  cursor: pointer;
  border: 2px solid white;
}

.workflow-node:hover .node-delete-btn,
.workflow-node.selected .node-delete-btn,
.sticky-note:hover .node-delete-btn,
.sticky-note.selected .node-delete-btn {
  opacity: 1;
  transform: scale(1);
}

.node-delete-btn:hover {
  background: #dc2626;
  transform: scale(1.1);
}

/*
 * Connection handles sit on the left/right edges of the node. The default
 * 12x12 visual is enlarged to a 14x14 dot with a 28x28 transparent hit
 * area (via ::before) so users can grab the handle without pixel-perfect
 * aim. The hit area does not shift the visual dot.
 */
.connection-handle {
  /* Logical (CSS) size — what users see at 100% zoom. The visible dot
   * is intentionally larger than n8n's default so it stays grabbable
   * even when fit-view-on-init scales the canvas down (0.6x = 17 px). */
  @apply !w-7 !h-7 rounded-full;
  @apply !bg-slate-400 dark:!bg-slate-500;
  @apply !border-2 !border-white dark:!border-slate-800;
  @apply transition-all duration-150;
  z-index: 2;
  position: absolute;
}

/* Larger hit-area rendered as a child of <Handle>. By default it is
 * invisible; on hover or while another handle is being dragged it shows
 * a faint halo so users see where to drop. pointer-events: none keeps
 * drag initiation working. The 72px box gives ~36px of slop on every
 * side which, even at 0.6x zoom, is still ~22 px — far above the
 * usual ~5 px click target of an unscaled handle. */
.handle-hit-area {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 72px;
  height: 72px;
  transform: translate(-50%, -50%);
  border-radius: 9999px;
  background: transparent;
  pointer-events: none;
  transition: background-color 150ms ease;
  z-index: 1;
}

.connection-handle:hover .handle-hit-area {
  background: rgba(99, 102, 241, 0.15);
}

.vue-flow__handle-connecting .connection-handle .handle-hit-area,
.connection-handle.vue-flow__handle-hot .handle-hit-area {
  background: rgba(99, 102, 241, 0.25);
}

.connection-handle--input {
  /* Offset = half of 28px handle so the dot sits flush on the node edge. */
  left: -14px !important;
}

.connection-handle--output {
  right: -14px !important;
}

.connection-handle:hover {
  @apply !bg-primary-500 scale-125;
  z-index: 3;
}

.vue-flow__handle-connecting .connection-handle,
.connection-handle.vue-flow__handle-hot {
  @apply !bg-primary-500;
  box-shadow: 0 0 0 4px rgba(59, 130, 246, 0.25);
}
</style>
