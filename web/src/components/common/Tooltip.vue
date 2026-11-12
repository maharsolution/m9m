<script setup lang="ts">
/**
 * Tooltip — m9m shared primitive.
 *
 * Dark pill, 12px text, ~300ms show delay, appears on hover AND
 * keyboard focus (DESIGN.md §7.8).
 */
import { onBeforeUnmount, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    /** Tooltip copy. */
    text: string
    /** Show delay in milliseconds. */
    delay?: number
    /** Tooltip placement relative to the trigger. */
    placement?: 'top' | 'bottom' | 'left' | 'right'
  }>(),
  {
    delay: 300,
    placement: 'top',
  }
)

const visible = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

function show() {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => {
    visible.value = true
  }, props.delay)
}

function hide() {
  if (timer) clearTimeout(timer)
  timer = null
  visible.value = false
}

onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
})

const placementClass: Record<string, string> = {
  top: 'bottom-full left-1/2 -translate-x-1/2 mb-1.5',
  bottom: 'top-full left-1/2 -translate-x-1/2 mt-1.5',
  left: 'right-full top-1/2 -translate-y-1/2 mr-1.5',
  right: 'left-full top-1/2 -translate-y-1/2 ml-1.5',
}
</script>

<template>
  <span
    class="relative inline-flex"
    @mouseenter="show"
    @mouseleave="hide"
    @focusin="show"
    @focusout="hide"
  >
    <slot />
    <span
      v-if="visible && text"
      role="tooltip"
      class="pointer-events-none absolute z-50 whitespace-nowrap rounded-lg bg-neutral-950 px-2 py-1 text-2xs font-medium text-white shadow-sm dark:bg-neutral-100 dark:text-neutral-950"
      :class="placementClass[placement]"
    >
      {{ text }}
    </span>
  </span>
</template>
