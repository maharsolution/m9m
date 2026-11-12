<script setup lang="ts">
/**
 * StatusBadge — m9m shared primitive.
 *
 * One family, four voices (DESIGN.md §2.4): the background, border
 * and text of a badge are always the same hue at three lightness
 * steps. Neutral is the "no state" voice.
 */
import { computed } from 'vue'

export type StatusTone = 'success' | 'warning' | 'danger' | 'info' | 'neutral'

const props = withDefaults(
  defineProps<{
    /** Status tone. */
    status: StatusTone
    /** Badge text. */
    label?: string
    /** Badge size. */
    size?: 'sm' | 'md'
    /** Shows a leading status dot. */
    dot?: boolean
  }>(),
  {
    label: undefined,
    size: 'sm',
    dot: false,
  }
)

const TONE_CLASSES: Record<StatusTone, string> = {
  success:
    'bg-green-50 border-green-200 text-green-800 dark:bg-green-950/50 dark:border-green-900 dark:text-green-300',
  warning:
    'bg-yellow-100 border-yellow-200 text-yellow-800 dark:bg-yellow-950/50 dark:border-yellow-900 dark:text-yellow-300',
  danger:
    'bg-red-50 border-red-200 text-red-800 dark:bg-red-950/50 dark:border-red-900 dark:text-red-300',
  info:
    'bg-blue-50 border-blue-200 text-blue-800 dark:bg-blue-950/50 dark:border-blue-900 dark:text-blue-300',
  neutral:
    'bg-neutral-100 border-neutral-200 text-neutral-700 dark:bg-neutral-800 dark:border-neutral-700 dark:text-neutral-200',
}

const DOT_CLASSES: Record<StatusTone, string> = {
  success: 'bg-green-600 dark:bg-green-400',
  warning: 'bg-yellow-600 dark:bg-yellow-400',
  danger: 'bg-red-600 dark:bg-red-400',
  info: 'bg-blue-600 dark:bg-blue-400',
  neutral: 'bg-neutral-500 dark:bg-neutral-400',
}

const rootClass = computed(() => [
  'inline-flex items-center gap-1.5 rounded-lg border font-medium whitespace-nowrap',
  'transition-colors duration-200 ease-out',
  TONE_CLASSES[props.status],
  props.size === 'sm' ? 'h-5 px-2 text-2xs' : 'h-6 px-2.5 text-xs',
])
</script>

<template>
  <span :class="rootClass">
    <span
      v-if="dot"
      class="h-1.5 w-1.5 shrink-0 rounded-full"
      :class="DOT_CLASSES[status]"
      aria-hidden="true"
    />
    <slot>{{ label }}</slot>
  </span>
</template>
