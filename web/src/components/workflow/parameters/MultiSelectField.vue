<script setup lang="ts">
import { computed, ref } from 'vue'
import type { NodePropertyOption } from '@/types/node'

/**
 * Multi-select parameter renderer (`multiOptions`).
 *
 * n8n renders a searchable checkbox list. m9m keeps the same shape:
 * every declared option is a toggle, the selected values are stored as
 * an array of option values.
 */
const props = withDefaults(
  defineProps<{
    modelValue?: unknown
    options?: NodePropertyOption[]
    placeholder?: string
    disabled?: boolean
  }>(),
  {
    options: () => [],
    placeholder: '',
    disabled: false,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: string[]): void
}>()

const search = ref('')

/** Currently selected option values. */
const selected = computed<string[]>(() => {
  const value = props.modelValue
  if (Array.isArray(value)) return value.map((entry) => String(entry))
  if (typeof value === 'string' && value.length > 0) return [value]
  return []
})

const filteredOptions = computed(() => {
  const term = search.value.trim().toLowerCase()
  if (!term) return props.options
  return props.options.filter((option) =>
    option.name.toLowerCase().includes(term),
  )
})

function isSelected(value: string): boolean {
  return selected.value.includes(value)
}

function toggle(value: string) {
  const next = isSelected(value)
    ? selected.value.filter((entry) => entry !== value)
    : [...selected.value, value]
  emit('update:modelValue', next)
}
</script>

<template>
  <div class="space-y-1">
    <input
      v-if="options.length > 8"
      v-model="search"
      type="search"
      :placeholder="placeholder || 'Search options…'"
      class="input text-sm"
    />
    <div
      v-if="options.length"
      class="max-h-48 overflow-y-auto rounded border border-slate-200 dark:border-slate-600 divide-y divide-slate-100 dark:divide-slate-700"
    >
      <label
        v-for="option in filteredOptions"
        :key="String(option.value)"
        class="flex items-center gap-2 px-2 py-1 text-sm text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-700 cursor-pointer"
      >
        <input
          type="checkbox"
          class="w-4 h-4 rounded border-slate-300 dark:border-slate-600 text-primary-600 focus:ring-primary-500"
          :checked="isSelected(String(option.value))"
          :disabled="disabled"
          @change="toggle(String(option.value))"
        />
        <span class="truncate">{{ option.name }}</span>
      </label>
      <p
        v-if="!filteredOptions.length"
        class="px-2 py-1 text-xs text-slate-500 dark:text-slate-400"
      >
        No matching options.
      </p>
    </div>
    <p v-else class="text-xs text-slate-500 dark:text-slate-400">
      This parameter declares no options.
    </p>
  </div>
</template>
