<script setup lang="ts">
import { computed } from 'vue'
import type { NodeProperty } from '@/types/node'

/**
 * File parameter renderer (`file`).
 *
 * n8n stores a file reference as `{ "mode": <mode>, "value": <name> }`:
 * the mode says *what* the value means (`binaryPropertyName` points at
 * the binary property carrying the payload, `fileName` names the file
 * itself) and the value is that name.
 *
 * Legacy workflows stored the bare name as a string — those are read
 * as `{ mode: <default mode>, value: <string> }`.
 */
interface FileValue {
  mode: string
  value: string
}

const props = withDefaults(
  defineProps<{
    property: NodeProperty
    modelValue?: unknown
    disabled?: boolean
  }>(),
  {
    disabled: false,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: FileValue): void
}>()

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** The mode the property falls back to. */
function defaultMode(): string {
  const raw = props.property.default
  if (isPlainObject(raw) && typeof raw.mode === 'string' && raw.mode) {
    return raw.mode
  }
  const modes = props.property.values ?? []
  return modes.length ? modes[0].value : 'binaryPropertyName'
}

/** Normalise whatever is stored (or missing) into the n8n shape. */
function normalize(raw: unknown): FileValue {
  if (typeof raw === 'string') {
    return { mode: defaultMode(), value: raw }
  }
  if (!isPlainObject(raw)) {
    return { mode: defaultMode(), value: '' }
  }
  return {
    mode: typeof raw.mode === 'string' && raw.mode ? raw.mode : defaultMode(),
    value: typeof raw.value === 'string' ? raw.value : '',
  }
}

const state = computed<FileValue>(() => normalize(props.modelValue))

/** The modes offered by the metadata. */
const modes = computed(() => props.property.values ?? [])

function onModeChange(event: Event) {
  const mode = (event.target as HTMLSelectElement).value
  emit('update:modelValue', { ...state.value, mode })
}

function onValueInput(event: Event) {
  const value = (event.target as HTMLInputElement).value
  emit('update:modelValue', { ...state.value, value })
}
</script>

<template>
  <div class="space-y-1">
    <div class="flex items-center gap-2">
      <select
        v-if="modes.length"
        :value="state.mode"
        class="input w-auto shrink-0"
        :disabled="disabled"
        @change="onModeChange"
      >
        <option
          v-for="mode in modes"
          :key="mode.value"
          :value="mode.value"
        >
          {{ mode.name }}
        </option>
      </select>

      <input
        :value="state.value"
        type="text"
        class="input flex-1"
        placeholder="Property name"
        :disabled="disabled"
        @input="onValueInput"
      />
    </div>
  </div>
</template>
