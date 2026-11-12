<script setup lang="ts">
/**
 * Button — m9m shared primitive.
 *
 * Variants: primary | secondary | ghost | danger | icon
 * Sizes:    sm | md
 *
 * Geometry follows DESIGN.md §7.1: 8px radius, 14px/500 label,
 * 32px minimum icon hit target, visible `:focus-visible` ring and a
 * 50% opacity disabled state.
 */
import { computed } from 'vue'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'icon'
export type ButtonSize = 'sm' | 'md'

const props = withDefaults(
  defineProps<{
    /** Visual variant. */
    variant?: ButtonVariant
    /** Control size. */
    size?: ButtonSize
    /** Shows a spinner and blocks interaction. */
    loading?: boolean
    /** Disables the button. */
    disabled?: boolean
    /** Native button type. */
    type?: 'button' | 'submit' | 'reset'
    /** Accessible label — required for the icon variant. */
    title?: string
  }>(),
  {
    variant: 'secondary',
    size: 'md',
    loading: false,
    disabled: false,
    type: 'button',
    title: undefined,
  }
)

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

const VARIANT_CLASSES: Record<ButtonVariant, string> = {
  primary:
    'bg-brand-500 text-white hover:bg-brand-600 active:bg-brand-700 border border-transparent',
  secondary:
    'bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 border border-neutral-300 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 active:bg-neutral-100 dark:active:bg-neutral-800',
  ghost:
    'bg-transparent text-neutral-700 dark:text-neutral-300 border border-transparent hover:bg-neutral-100 dark:hover:bg-neutral-800 active:bg-neutral-200 dark:active:bg-neutral-700',
  danger:
    'bg-red-600 text-white hover:bg-red-700 active:bg-red-800 border border-transparent',
  icon:
    'bg-transparent text-neutral-600 dark:text-neutral-300 border border-transparent hover:bg-neutral-100 dark:hover:bg-neutral-800',
}

const SIZE_CLASSES: Record<ButtonSize, string> = {
  sm: 'h-8 px-3 text-xs gap-1.5',
  md: 'h-10 px-4 text-sm gap-2',
}

const ICON_SIZE_CLASSES: Record<ButtonSize, string> = {
  sm: 'h-8 w-8 p-0',
  md: 'h-8 w-8 p-0',
}

const rootClass = computed(() => [
  'inline-flex items-center justify-center rounded-lg font-medium',
  'transition-colors duration-200 ease-out select-none',
  'focus-visible:outline-none focus-visible:outline-2 focus-visible:outline-offset-2',
  'focus-visible:outline-neutral-950 dark:focus-visible:outline-neutral-100',
  'disabled:opacity-50 disabled:cursor-not-allowed',
  VARIANT_CLASSES[props.variant],
  props.variant === 'icon' ? ICON_SIZE_CLASSES[props.size] : SIZE_CLASSES[props.size],
])

function handleClick(event: MouseEvent) {
  if (props.disabled || props.loading) {
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
    :disabled="disabled || loading"
    :title="title"
    :aria-busy="loading || undefined"
    @click="handleClick"
  >
    <span
      v-if="loading"
      class="inline-block h-3.5 w-3.5 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent"
      aria-hidden="true"
    />
    <span
      v-if="$slots.icon"
      class="inline-flex shrink-0 items-center"
      :class="loading ? 'opacity-60' : ''"
    >
      <slot name="icon" />
    </span>
    <span v-if="$slots.default" :class="loading ? 'opacity-80' : ''">
      <slot />
    </span>
  </button>
</template>
