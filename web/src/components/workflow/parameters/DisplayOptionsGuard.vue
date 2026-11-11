<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { NodeProperty } from '@/types/node'
import { evaluateDisplayOptions } from './displayOptions'

/**
 * Renders its slot only while the guarded property is visible.
 *
 * The guard re-evaluates live: whenever the controlling parameter
 * changes, the visibility flips and the slot is shown/hidden
 * immediately. When a field that was visible becomes hidden the guard
 * emits `hidden` so the panel can drop the value — a hidden field must
 * not be submitted with the workflow.
 */
const props = defineProps<{
  property: NodeProperty
  /** Current parameter values of the node (live, reactive). */
  parameters: Record<string, unknown>
  /** All properties of the node — used to resolve controlling defaults. */
  properties: NodeProperty[]
}>()

const emit = defineEmits<{
  (event: 'hidden', property: NodeProperty): void
}>()

const visible = computed(() =>
  evaluateDisplayOptions(props.property.displayOptions, props.parameters, props.properties),
)

const initialized = ref(false)
const wasVisible = ref(true)

watch(
  visible,
  (now) => {
    if (!initialized.value) {
      initialized.value = true
      wasVisible.value = now
      return
    }
    if (wasVisible.value && !now) {
      emit('hidden', props.property)
    }
    wasVisible.value = now
  },
  { immediate: true },
)
</script>

<template>
  <slot v-if="visible" />
</template>
