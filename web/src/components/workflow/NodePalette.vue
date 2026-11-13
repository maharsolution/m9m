<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { MagnifyingGlassIcon, XMarkIcon } from '@heroicons/vue/24/outline'
import { useNodesStore, useWorkflowEditorStore } from '@/stores'
import { NODE_CATEGORIES, getNodeCategory } from '@/types/node'
import { nodeIconFor } from '@/components/nodes/BaseNode.vue'
import { shouldCloseCreator } from '@/lib/nodeCreator'
import type { NodeType, NodeCategory } from '@/types'

const emit = defineEmits<{
  /** The scrim was clicked or the panel was dismissed. */
  close: []
  /**
   * A node type was clicked in the creator. n8n adds the node to the
   * canvas on a plain click (not only on a drag) — the parent decides
   * where the node lands.
   */
  'add-node': [nodeType: string]
}>()

const nodesStore = useNodesStore()
const workflowEditorStore = useWorkflowEditorStore()

/**
 * The node creator floats over the canvas. Its scrim dims the canvas
 * but stays *visual only* — it never takes pointer events, so the
 * canvas keeps receiving the drag-and-drop that adds a node (and the
 * canvas stays pannable / selectable while the creator is open, n8n
 * parity).
 *
 * Because the scrim cannot take the click, the creator is closed by a
 * document-level click guard: a click outside the panel closes it.
 */
const panelRef = ref<HTMLElement | null>(null)

/**
 * When the panel was mounted. The click that OPENED the creator is
 * older than that — it must not close the creator again.
 */
let openedAt = 0

/**
 * A drag that just finished must not also count as a click —
 * otherwise dropping a node onto the canvas would add it twice.
 */
let lastDragEndAt = 0

/** Clicks right after a drag ended belong to that drag. */
const DRAG_END_GRACE_MS = 300

const onDocumentClick = (event: MouseEvent) => {
  if (!shouldCloseCreator(event.target, panelRef.value, openedAt, event.timeStamp)) {
    return
  }
  emit('close')
}

let listenerTimer: ReturnType<typeof setTimeout> | null = null

onMounted(() => {
  openedAt = performance.now()
  // The guard is attached one tick late: the click that opened the
  // creator is still bubbling towards the document while the panel
  // mounts. A listener attached right away would see that very click,
  // find a target outside the panel and close the creator again —
  // the panel would flash and vanish before a node could be dragged
  // out of it.
  listenerTimer = setTimeout(() => {
    listenerTimer = null
    document.addEventListener('click', onDocumentClick)
  }, 0)
})

onUnmounted(() => {
  if (listenerTimer !== null) {
    clearTimeout(listenerTimer)
    listenerTimer = null
  }
  document.removeEventListener('click', onDocumentClick)
})

const searchQuery = ref('')
const expandedCategories = ref<Set<NodeCategory>>(new Set(['trigger', 'action', 'transform']))

const filteredNodes = computed(() => {
  if (!searchQuery.value) {
    return nodesStore.nodeTypes
  }

  const query = searchQuery.value.toLowerCase()
  return nodesStore.nodeTypes.filter(
    (node) =>
      node.displayName.toLowerCase().includes(query) ||
      node.description.toLowerCase().includes(query)
  )
})

const nodesByCategory = computed(() => {
  const grouped: Record<NodeCategory, NodeType[]> = {} as Record<NodeCategory, NodeType[]>

  NODE_CATEGORIES.forEach((cat) => {
    grouped[cat.name] = []
  })

  filteredNodes.value.forEach((node) => {
    const category = getNodeCategory(node.name)
    if (grouped[category]) {
      grouped[category].push(node)
    } else {
      grouped.action.push(node)
    }
  })

  return grouped
})

/**
 * While the operator is typing, every matching category is open —
 * a collapsed category would hide the very result they are looking
 * for.
 */
const isSearching = computed(() => searchQuery.value.trim().length > 0)

const isExpanded = (category: NodeCategory) =>
  isSearching.value || expandedCategories.value.has(category)

const toggleCategory = (category: NodeCategory) => {
  if (expandedCategories.value.has(category)) {
    expandedCategories.value.delete(category)
  } else {
    expandedCategories.value.add(category)
  }
}

const onDragStart = (event: DragEvent, nodeType: NodeType) => {
  lastDragEndAt = 0
  if (event.dataTransfer) {
    event.dataTransfer.setData('application/vueflow', nodeType.name)
    event.dataTransfer.effectAllowed = 'move'
  }
}

/**
 * A drag that just finished must not also count as a click — otherwise
 * dropping a node onto the canvas would add it twice.
 */
const onDragEnd = () => {
  lastDragEndAt = performance.now()
}

/**
 * Clicking a node type adds it to the canvas.
 *
 * n8n parity: the node creator adds a node when you CLICK it, not only
 * when you drag it onto the canvas. m9m only supported the drag, so a
 * user who clicked a node in the panel saw nothing happen and assumed
 * the feature was broken.
 */
const onItemClick = (nodeType: NodeType) => {
  // The click that ends a drag belongs to that drag.
  if (lastDragEndAt && performance.now() - lastDragEndAt < DRAG_END_GRACE_MS) {
    lastDragEndAt = 0
    return
  }
  emit('add-node', nodeType.name)
}

/**
 * Adds an n8n sticky note to the canvas. Sticky notes are not part of
 * the node-type feed (n8n does not offer them in the node creator
 * either) — they are added through this quick action.
 */
const addStickyNote = () => {
  workflowEditorStore.addStickyNote()
}

/** The 20px glyph of a node row — the same one the canvas card uses. */
const iconFor = (node: NodeType) => nodeIconFor(node.name, getNodeCategory(node.name))
</script>

<template>
  <!--
    n8n's node creator: a right-anchored overlay (385px) with a
    scrim over the canvas. The panel is anchored to the editor
    shell, so it starts below the editor header and floats above
    the canvas.

    The scrim dims the canvas but never takes pointer events: it must
    not swallow the drag-and-drop that adds a node, nor the clicks
    that pan / select on the canvas. The creator is closed by the
    document-level click guard in the script block instead.
  -->
  <div class="absolute inset-0 z-30 pointer-events-none">
    <!-- Scrim — visual only -->
    <div
      class="m9m-scrim absolute inset-0 top-[65px] pointer-events-none"
    />

    <!-- Panel -->
    <div
      ref="panelRef"
      class="m9m-panel absolute right-0 top-[65px] bottom-0 w-[385px] pointer-events-auto flex flex-col border-l border-default"
    >
      <!-- Header -->
      <div class="flex items-center justify-between px-4 h-[42px] shrink-0 border-b border-default">
        <h3 class="text-sm font-semibold text-primary">
          Nodes
        </h3>
        <button
          type="button"
          class="h-8 w-8 inline-flex items-center justify-center rounded-md text-tertiary hover:bg-hover"
          title="Close the node panel"
          aria-label="Close the node panel"
          @click="emit('close')"
        >
          <XMarkIcon class="w-4 h-4" />
        </button>
      </div>

      <!-- Search — pinned to the top of the panel -->
      <div class="px-4 pt-3 pb-2 shrink-0 border-b border-default">
        <div class="relative">
          <MagnifyingGlassIcon class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-tertiary" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search nodes..."
            class="input h-10 pl-9 pr-3"
          />
        </div>

        <!-- Quick action: sticky note. n8n offers the note as a canvas
             action rather than as a node type, so it lives here and not
             in the categorised list. -->
        <button
          type="button"
          @click="addStickyNote"
          class="mt-2 w-full flex items-center gap-2 px-3 py-2 text-sm font-medium rounded-md text-secondary hover:bg-hover transition-colors"
          title="Add a sticky note to the canvas"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 4h14v10l-4 4H5V4z" />
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 18v-4h4" />
          </svg>
          <span class="flex-1 text-left">Sticky Note</span>
        </button>
      </div>

      <!-- Node List -->
      <div class="flex-1 overflow-y-auto px-2 py-2">
        <div
          v-for="category in NODE_CATEGORIES"
          :key="category.name"
          class="mb-1"
        >
          <!-- Category Header -->
          <button
            v-if="nodesByCategory[category.name]?.length > 0"
            @click="toggleCategory(category.name)"
            class="w-full flex items-center gap-2 px-2 py-2 rounded-md text-sm font-medium text-secondary hover:bg-hover transition-colors"
          >
            <span class="flex-1 text-left">{{ category.displayName }}</span>
            <span class="text-xs font-normal tabular-nums text-tertiary">
              {{ nodesByCategory[category.name]?.length || 0 }}
            </span>
            <svg
              :class="[
                'w-4 h-4 text-tertiary transition-transform',
                isExpanded(category.name) ? 'rotate-90' : ''
              ]"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
            </svg>
          </button>

          <!-- Category Nodes -->
          <div
            v-if="isExpanded(category.name) && nodesByCategory[category.name]?.length > 0"
            class="mt-1 space-y-0.5"
          >
            <div
              v-for="node in nodesByCategory[category.name]"
              :key="node.name"
              :draggable="true"
              @dragstart="onDragStart($event, node)"
              @dragend="onDragEnd"
              @click="onItemClick(node)"
              class="flex items-start gap-3 px-2 py-2 rounded-md cursor-grab active:cursor-grabbing hover:bg-hover transition-colors group"
            >
              <component
                :is="iconFor(node)"
                class="w-5 h-5 mt-0.5 flex-shrink-0 text-tertiary"
              />
              <div class="min-w-0 flex-1">
                <p class="text-sm font-medium text-primary truncate">
                  {{ node.displayName }}
                </p>
                <p class="text-xs text-tertiary truncate">
                  {{ node.description }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Empty State -->
        <div
          v-if="filteredNodes.length === 0"
          class="text-center py-8"
        >
          <p class="text-sm text-tertiary">
            No nodes found
          </p>
        </div>
      </div>

      <!-- Help Text -->
      <div class="px-4 py-3 shrink-0 border-t border-default">
        <p class="text-xs text-tertiary">
          Click a node to add it, or drag it onto the canvas
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
/*
 * The node creator floats over the canvas, so it gets a surface fill
 * and a shadow (an overlay gets a shadow; an in-layout surface gets a
 * border). The values come from the published token layer.
 */
.m9m-panel {
  background: var(--background--surface, #ffffff);
  box-shadow: var(--shadow--xl, 0 20px 25px -5px oklch(0% 0 0 / 0.1), 0 8px 10px -6px oklch(0% 0 0 / 0.1));
}

:global(.dark) .m9m-panel {
  background: var(--background--surface, oklch(26.86% 0 89.88));
}

/*
 * The scrim sits between the canvas and the node creator. It uses the
 * canvas surface at 70% opacity, so the canvas is dimmed in both
 * themes without introducing a second brand colour.
 */
.m9m-scrim {
  background: oklch(100% 0 89.88 / 0.7);
}

:global(.dark) .m9m-scrim {
  background: oklch(20.46% 0 89.88 / 0.7);
}
</style>
