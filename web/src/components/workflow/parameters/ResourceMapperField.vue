<script setup lang="ts">
import { computed } from 'vue'
import { PlusIcon, TrashIcon } from '@heroicons/vue/24/outline'
import type { NodeProperty } from '@/types/node'

/**
 * Resource-mapper parameter renderer (`resourceMapper`).
 *
 * n8n stores a resource mapper as its `ResourceMapperValue`:
 *
 * ```json
 * {
 *   "mappingMode": "defineBelow",
 *   "value": { "<destination field>": "<source expression>" },
 *   "matchingColumns": [],
 *   "schema": [
 *     { "id": "name", "displayName": "name", "defaultMatch": false,
 *       "required": false, "display": true }
 *   ],
 *   "attemptToConvertTypes": false,
 *   "convertFieldsToString": false
 * }
 * ```
 *
 * The rows are derived from the schema (which carries the order) and
 * the value map (which carries the expressions). Renaming a
 * destination field rewrites the schema entry and moves the value
 * entry along with it.
 */
interface ResourceMapperSchemaEntry {
  id: string
  displayName: string
  defaultMatch: boolean
  required: boolean
  display: boolean
}

interface ResourceMapperValue {
  mappingMode: string
  value: Record<string, string>
  matchingColumns: string[]
  schema: ResourceMapperSchemaEntry[]
  attemptToConvertTypes: boolean
  convertFieldsToString: boolean
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
  (event: 'update:modelValue', value: ResourceMapperValue): void
}>()

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** The value the property declares as its default. */
function declaredDefault(): ResourceMapperValue {
  const raw = props.property.default
  if (!isPlainObject(raw)) {
    return {
      mappingMode: 'defineBelow',
      value: {},
      matchingColumns: [],
      schema: [],
      attemptToConvertTypes: false,
      convertFieldsToString: false,
    }
  }
  return {
    mappingMode: typeof raw.mappingMode === 'string' ? raw.mappingMode : 'defineBelow',
    value: {},
    matchingColumns: [],
    schema: [],
    attemptToConvertTypes: raw.attemptToConvertTypes === true,
    convertFieldsToString: raw.convertFieldsToString === true,
  }
}

/** Normalise whatever is stored (or missing) into the n8n shape. */
function normalize(raw: unknown): ResourceMapperValue {
  const fallback = declaredDefault()
  if (!isPlainObject(raw)) return fallback

  const value: Record<string, string> = {}
  if (isPlainObject(raw.value)) {
    for (const [key, entry] of Object.entries(raw.value)) {
      value[key] = typeof entry === 'string' ? entry : String(entry ?? '')
    }
  }

  const schema: ResourceMapperSchemaEntry[] = []
  const known = new Set<string>()
  if (Array.isArray(raw.schema)) {
    for (const entry of raw.schema) {
      if (!isPlainObject(entry)) continue
      const id = typeof entry.id === 'string' ? entry.id : ''
      if (!id || known.has(id)) continue
      known.add(id)
      schema.push({
        id,
        displayName: typeof entry.displayName === 'string' ? entry.displayName : id,
        defaultMatch: entry.defaultMatch === true,
        required: entry.required === true,
        display: entry.display !== false,
      })
    }
  }

  // A value entry without a schema entry still deserves a row — the
  // schema entry is synthesised so both sides stay in sync.
  for (const key of Object.keys(value)) {
    if (known.has(key)) continue
    known.add(key)
    schema.push({
      id: key,
      displayName: key,
      defaultMatch: false,
      required: false,
      display: true,
    })
  }

  const matchingColumns = Array.isArray(raw.matchingColumns)
    ? raw.matchingColumns.map((entry) => String(entry))
    : []

  return {
    mappingMode:
      typeof raw.mappingMode === 'string' && raw.mappingMode
        ? raw.mappingMode
        : fallback.mappingMode,
    value,
    matchingColumns,
    schema,
    attemptToConvertTypes: raw.attemptToConvertTypes === true,
    convertFieldsToString: raw.convertFieldsToString === true,
  }
}

const state = computed<ResourceMapperValue>(() => normalize(props.modelValue))

/** The mapping modes offered by the metadata. */
const modes = computed(() => props.property.values ?? [])

const currentMode = computed(() => state.value.mappingMode)

interface MapperRow {
  id: string
  displayName: string
  expression: string
}

/** One row per schema entry, in schema order. */
const rows = computed<MapperRow[]>(() =>
  state.value.schema.map((entry) => ({
    id: entry.id,
    displayName: entry.displayName || entry.id,
    expression: state.value.value[entry.id] ?? '',
  })),
)

function emitValue(next: ResourceMapperValue) {
  emit('update:modelValue', next)
}

function onModeChange(event: Event) {
  const mode = (event.target as HTMLSelectElement).value
  emitValue({ ...state.value, mappingMode: mode })
}

function renameRow(index: number, nextName: string) {
  const current = state.value
  const schema = current.schema.map((entry) => ({ ...entry }))
  const values = { ...current.value }
  const entry = schema[index]
  if (!entry) return

  const trimmed = nextName.trim()
  const previousId = entry.id
  const nextId = trimmed || previousId
  const expression = values[previousId] ?? ''

  delete values[previousId]
  entry.id = nextId
  entry.displayName = trimmed
  values[nextId] = expression

  emitValue({ ...current, schema, value: values })
}

function setExpression(index: number, expression: string) {
  const current = state.value
  const schema = current.schema.map((entry) => ({ ...entry }))
  const values = { ...current.value }
  const entry = schema[index]
  if (!entry) return

  values[entry.id] = expression
  emitValue({ ...current, schema, value: values })
}

function addRow() {
  const current = state.value
  const schema = current.schema.map((entry) => ({ ...entry }))
  const values = { ...current.value }

  let counter = schema.length + 1
  let id = `field_${counter}`
  while (id in values) {
    counter += 1
    id = `field_${counter}`
  }

  schema.push({
    id,
    displayName: '',
    defaultMatch: false,
    required: false,
    display: true,
  })
  values[id] = ''

  emitValue({ ...current, schema, value: values })
}

function removeRow(index: number) {
  const current = state.value
  const removed = current.schema[index]
  if (!removed) return

  const schema = current.schema
    .filter((_, i) => i !== index)
    .map((entry) => ({ ...entry }))
  const values = { ...current.value }
  delete values[removed.id]

  emitValue({ ...current, schema, value: values })
}
</script>

<template>
  <div class="space-y-2">
    <select
      v-if="modes.length"
      :value="currentMode"
      class="input"
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

    <div
      v-for="(row, index) in rows"
      :key="row.id + '-' + index"
      class="rounded border border-slate-200 dark:border-slate-600 p-2 space-y-1"
    >
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-slate-500 dark:text-slate-400">
          Field {{ index + 1 }}
        </span>
        <button
          type="button"
          class="p-1 text-red-500 hover:text-red-600 disabled:opacity-30"
          title="Remove field"
          :disabled="disabled"
          @click="removeRow(index)"
        >
          <TrashIcon class="w-3.5 h-3.5" />
        </button>
      </div>

      <input
        :value="row.displayName"
        type="text"
        class="input text-sm"
        placeholder="Destination field name"
        :disabled="disabled"
        @input="renameRow(index, ($event.target as HTMLInputElement).value)"
      />

      <input
        :value="row.expression"
        type="text"
        class="input text-sm font-mono"
        placeholder="Source expression (e.g. {{ $json.name }})"
        :disabled="disabled"
        @input="setExpression(index, ($event.target as HTMLInputElement).value)"
      />
    </div>

    <button
      type="button"
      class="flex items-center gap-1 text-xs text-primary-600 dark:text-primary-400 hover:underline disabled:opacity-40"
      :disabled="disabled"
      @click="addRow"
    >
      <PlusIcon class="w-3.5 h-3.5" />
      Add field
    </button>

    <p
      v-if="!rows.length"
      class="text-xs text-slate-500 dark:text-slate-400"
    >
      No fields mapped yet.
    </p>
  </div>
</template>
