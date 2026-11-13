<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  Position,
  type EdgeProps,
} from '@vue-flow/core'
import { XMarkIcon } from '@heroicons/vue/24/outline'
import { parseEdgeId } from '@/lib/workflowGraph'
import { useWorkflowEditorStore } from '@/stores/workflowEditor'

/**
 * The connection of the workflow canvas.
 *
 * n8n parity: a connection can be selected (click) and removed — with
 * the Delete key or with the button that appears while the pointer
 * hovers the connection.
 *
 * The edge id carries the connection it stands for
 * (`workflow-edge:<source>:<output>:<target>:<input>`), so the button
 * removes exactly the connection that was drawn.
 */
const props = defineProps<EdgeProps>()

const editorStore = useWorkflowEditorStore()

/** True while the pointer hovers the connection. */
const hovered = ref(false)

/** Timer that hides the button after the pointer left the connection. */
let leaveTimer: ReturnType<typeof setTimeout> | null = null

const pathData = computed(() =>
  getBezierPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    targetX: props.targetX,
    targetY: props.targetY,
    sourcePosition: props.sourcePosition ?? Position.Right,
    targetPosition: props.targetPosition ?? Position.Left,
  }),
)

const path = computed(() => pathData.value[0])
const labelX = computed(() => pathData.value[1])
const labelY = computed(() => pathData.value[2])

/** The connection this edge stands for. */
const connection = computed(() => parseEdgeId(props.id))

/** The remove button is shown while the connection is hovered or selected. */
const showRemove = computed(() => hovered.value || Boolean(props.selected))

const onEnter = () => {
  if (leaveTimer) {
    clearTimeout(leaveTimer)
    leaveTimer = null
  }
  hovered.value = true
}

const onLeave = () => {
  // The button sits on top of the connection: the pointer needs a
  // moment to travel from the line to the button.
  leaveTimer = setTimeout(() => {
    hovered.value = false
    leaveTimer = null
  }, 250)
}

onBeforeUnmount(() => {
  if (leaveTimer) {
    clearTimeout(leaveTimer)
    leaveTimer = null
  }
})

/** Removes the connection behind the edge. */
const removeConnection = (event: MouseEvent) => {
  event.stopPropagation()
  const edge = connection.value
  if (!edge) return

  editorStore.removeConnection(
    edge.sourceNodeId,
    edge.targetNodeId,
    edge.sourceOutput,
    edge.targetInput,
  )
}
</script>

<template>
  <g @mouseenter="onEnter" @mouseleave="onLeave">
    <BaseEdge
      :id="props.id"
      :path="path"
      :marker-end="props.markerEnd"
      :style="props.style"
      :interaction-width="props.interactionWidth"
    />
  </g>

  <EdgeLabelRenderer>
    <button
      v-if="showRemove"
      type="button"
      class="m9m-edge-remove nodrag nopan"
      title="Remove connection"
      aria-label="Remove connection"
      :style="{
        transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
      }"
      @mouseenter="onEnter"
      @mouseleave="onLeave"
      @mousedown.stop
      @click.stop="removeConnection"
    >
      <XMarkIcon class="w-3.5 h-3.5" aria-hidden="true" />
    </button>
  </EdgeLabelRenderer>
</template>

<style scoped>
/*
 * The remove button of a connection — n8n parity: a small round
 * button on the connection, in the brand colour of the editor.
 */
.m9m-edge-remove {
  position: absolute;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 9999px;
  border: 1px solid var(--border-color, oklch(0% 0 0 / 0.1));
  background: var(--background--surface, #ffffff);
  color: var(--text-color--danger, oklch(55.55% 0.2373 25.44));
  box-shadow: var(--shadow--sm, 0 1px 3px 0 oklch(0% 0 0 / 0.1));
  cursor: pointer;
  /* The label layer of Vue Flow ignores the pointer (`pointer-events:
     none`) — the button has to take the events back. */
  pointer-events: auto;
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1),
    color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.m9m-edge-remove:hover {
  background: var(--background--danger, oklch(97.05% 0.0129 17.38));
  color: var(--text-color--danger, oklch(55.55% 0.2373 25.44));
}

@media (prefers-reduced-motion: reduce) {
  .m9m-edge-remove {
    transition-duration: 1ms;
  }
}
</style>
