<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import type { NodeCategory } from '@/types'

interface Props {
  id: string
  data: {
    label: string
    nodeType: string
    category: NodeCategory
    parameters?: Record<string, unknown>
  }
  selected?: boolean
}

const props = defineProps<Props>()

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
</script>

<template>
  <div
    :class="[
      'workflow-node min-w-[180px] max-w-[220px]',
      categoryStyles.border,
      'border-l-4',
      selected ? 'selected' : ''
    ]"
  >
    <!-- Input Handle -->
    <Handle
      v-if="data.category !== 'trigger'"
      id="input-0"
      type="target"
      :position="Position.Left"
      :connectable="true"
      :is-connectable="true"
      class="connection-handle connection-handle--input"
    />

    <!-- Node Content -->
    <div class="p-3">
      <div class="flex items-center gap-2">
        <div :class="[categoryStyles.icon, 'w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0']">
          <span class="text-white text-sm">{{ categoryStyles.iconText }}</span>
        </div>
        <div class="min-w-0 flex-1">
          <h4 class="text-sm font-semibold text-slate-900 dark:text-white truncate">
            {{ data.label }}
          </h4>
          <p class="text-xs text-slate-500 dark:text-slate-400 truncate">
            {{ nodeTypeName }}
          </p>
        </div>
      </div>

      <!-- Parameters preview (optional) -->
      <div
        v-if="data.parameters && Object.keys(data.parameters).length > 0"
        class="mt-2 pt-2 border-t border-slate-200 dark:border-slate-600"
      >
        <div
          v-for="(value, key) in data.parameters"
          :key="key"
          class="text-xs text-slate-500 dark:text-slate-400 truncate"
        >
          <span class="font-medium">{{ key }}:</span>
          {{ typeof value === 'object' ? '...' : value }}
        </div>
      </div>
    </div>

    <!-- Output Handle -->
    <Handle
      id="output-0"
      type="source"
      :position="Position.Right"
      :connectable="true"
      :is-connectable="true"
      class="connection-handle connection-handle--output"
    />
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
 * Connection handles sit on the left/right edges of the node. The default
 * 12x12 visual is enlarged to a 14x14 dot with a 28x28 transparent hit
 * area (via ::before) so users can grab the handle without pixel-perfect
 * aim. The hit area does not shift the visual dot.
 */
.connection-handle {
  @apply !w-3.5 !h-3.5 rounded-full;
  @apply !bg-slate-400 dark:!bg-slate-500;
  @apply !border-2 !border-white dark:!border-slate-800;
  @apply transition-all duration-150;
  z-index: 2;
  position: absolute;
}

.connection-handle::before {
  content: '';
  position: absolute;
  top: 50%;
  left: 50%;
  width: 28px;
  height: 28px;
  transform: translate(-50%, -50%);
  border-radius: 9999px;
  background: transparent;
  z-index: 1;
  /*
   * CRITICAL: without `pointer-events: none`, this enlarged hit target
   * swallows the pointerdown event before it reaches the Vue Flow Handle,
   * so drag-to-connect never starts. The pseudo-element must remain
   * visually present for the larger hit area but transparent to pointer
   * events — the parent Handle (which is 14x14) receives them instead.
   */
  pointer-events: none;
}

.connection-handle--input {
  left: -7px !important;
}

.connection-handle--output {
  right: -7px !important;
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
