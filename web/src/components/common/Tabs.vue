<script setup lang="ts">
/**
 * Tabs — m9m shared primitive.
 *
 * Underline style, active state uses the brand accent, inactive tabs
 * use the tertiary text role (DESIGN.md §7.9 / role map §3.3).
 */
import { computed } from 'vue'

export interface TabItem {
  /** Stable identifier — the value carried by `modelValue`. */
  value: string
  /** Visible label. */
  label: string
  /** Disables the tab. */
  disabled?: boolean
  /** Small trailing marker (counts, badges). */
  badge?: string | number
}

const props = withDefaults(
  defineProps<{
    /** Active tab value (use `v-model`). */
    modelValue: string
    /** Tab descriptors. */
    tabs: TabItem[]
  }>(),
  {
    modelValue: '',
    tabs: () => [],
  }
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  change: [value: string]
}>()

const activeValue = computed(() => props.modelValue)

function select(value: string) {
  if (value === props.modelValue) return
  emit('update:modelValue', value)
  emit('change', value)
}
</script>

<template>
  <div
    role="tablist"
    class="flex items-center gap-1 overflow-x-auto border-b border-neutral-200 dark:border-neutral-800"
  >
    <button
      v-for="tab in tabs"
      :key="tab.value"
      type="button"
      role="tab"
      :aria-selected="activeValue === tab.value"
      :disabled="tab.disabled"
      class="-mb-px inline-flex items-center gap-2 whitespace-nowrap border-b-2 px-3 py-2 text-2xs font-medium uppercase tracking-wide transition-colors duration-200 ease-out focus-visible:outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-neutral-950 disabled:cursor-not-allowed disabled:opacity-50 dark:focus-visible:outline-neutral-100"
      :class="
        activeValue === tab.value
          ? 'border-brand-500 text-brand-700 dark:text-brand-300'
          : 'border-transparent text-neutral-600 hover:border-neutral-300 hover:text-neutral-950 dark:text-neutral-400 dark:hover:border-neutral-700 dark:hover:text-neutral-100'
      "
      @click="select(tab.value)"
    >
      <span>{{ tab.label }}</span>
      <span
        v-if="tab.badge !== undefined && tab.badge !== ''"
        class="rounded-full bg-neutral-100 px-1.5 py-0.5 text-3xs font-medium text-neutral-700 dark:bg-neutral-800 dark:text-neutral-200"
      >
        {{ tab.badge }}
      </span>
    </button>
  </div>
</template>
