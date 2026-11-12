<script setup lang="ts">
/**
 * IconButton — m9m shared primitive.
 *
 * 32px hit target, 16px icon (the slot decides the icon size, this
 * component guarantees the box). The accessible name comes from the
 * `label` prop, which is also rendered as the native tooltip.
 */
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    /** Accessible name — also used as the native tooltip. */
    label: string
    /** Disables the button. */
    disabled?: boolean
    /** Visual treatment. */
    tone?: 'default' | 'danger'
    /** Native button type. */
    type?: 'button' | 'submit' | 'reset'
  }>(),
  {
    disabled: false,
    tone: 'default',
    type: 'button',
  }
)

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

const rootClass = computed(() => [
  'inline-flex h-8 w-8 items-center justify-center rounded-lg',
  'transition-colors duration-200 ease-out',
  'focus-visible:outline-none focus-visible:outline-2 focus-visible:outline-offset-2',
  'focus-visible:outline-neutral-950 dark:focus-visible:outline-neutral-100',
  'disabled:opacity-50 disabled:cursor-not-allowed',
  props.tone === 'danger'
    ? 'text-neutral-500 dark:text-neutral-400 hover:bg-red-50 hover:text-red-700 dark:hover:bg-red-950/40 dark:hover:text-red-300'
    : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 hover:text-neutral-900 dark:hover:bg-neutral-800 dark:hover:text-neutral-100',
])

function handleClick(event: MouseEvent) {
  if (props.disabled) {
    event.preventDefault()
    event.stopPropagation()
    return
  }
  emit('click', event)
}
</script>

<template>
  <button
    :type="type"
    :class="rootClass"
    :disabled="disabled"
    :title="label"
    :aria-label="label"
    @click="handleClick"
  >
    <slot />
  </button>
</template>
