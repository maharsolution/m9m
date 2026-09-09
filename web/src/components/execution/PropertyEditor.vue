<script setup lang="ts">
/**
 * PropertyEditor — renders a single NodeProperty descriptor as a
 * form control. Read-only in the execution detail view (we render
 * it to show the user what was configured, not to edit it).
 *
 * The control type drives the renderer:
 *   - 'string'        → text input (rows from typeOptions.rows)
 *   - 'options'       → dropdown
 *   - 'multiOptions'  → multi-select checkboxes
 *   - 'number'        → number input (min/max from typeOptions)
 *   - 'boolean'       → checkbox
 *   - 'json'          → JSON textarea with syntax styling
 *   - 'collection'    → key/value rows (name/value pairs)
 *   - 'fixedCollection'→ labelled bucket rows
 *   - 'dateTime'      → ISO datetime string
 */
import { computed } from 'vue'
import type { NodeProperty } from '@/types'

interface Props {
  property: NodeProperty
  value: unknown
}

const props = defineProps<Props>()

const isTextarea = computed(() => props.property.type === 'string' && (props.property.typeOptions?.rows ?? 1) > 1)
const rows = computed(() => Number(props.property.typeOptions?.rows ?? 3))

function displayString(v: unknown): string {
  if (v === undefined || v === null) return ''
  if (typeof v === 'string') return v
  return JSON.stringify(v, null, 2)
}

function inferDefaultString(prop: NodeProperty): string {
  if (prop.default === undefined || prop.default === null) return ''
  if (typeof prop.default === 'string') return prop.default
  return JSON.stringify(prop.default)
}

const textValue = computed(() => displayString(props.value ?? inferDefaultString(props.property)))

const boolValue = computed<boolean>(() => {
  if (typeof props.value === 'boolean') return props.value
  if (props.value === undefined && props.property.default !== undefined) {
    return Boolean(props.property.default)
  }
  return Boolean(props.value)
})

const numberValue = computed<number>(() => {
  if (typeof props.value === 'number') return props.value
  if (typeof props.value === 'string') {
    const n = Number(props.value)
    return Number.isFinite(n) ? n : 0
  }
  if (typeof props.property.default === 'number') return props.property.default
  return 0
})

// Collection row helper: turn a `[]map[string]interface{}` value
// into rows the template can render. Unknown / empty defaults to
// a single empty row so the user sees "Name / Value" hints.
interface KVRow {
  name: string
  value: unknown
}

const collectionRows = computed<KVRow[]>(() => {
  const v = props.value
  if (Array.isArray(v)) {
    return v.map((entry) => {
      if (entry && typeof entry === 'object' && !Array.isArray(entry)) {
        const obj = entry as Record<string, unknown>
        // For HTTP Request `name`/`value` collections.
        const nameKey = Object.keys(obj).find((k) => k.toLowerCase() === 'name') ?? Object.keys(obj)[0]
        const valueKey = Object.keys(obj).find((k) => k.toLowerCase() === 'value') ?? Object.keys(obj)[1]
        return {
          name: String(obj[nameKey] ?? ''),
          value: obj[valueKey],
        }
      }
      return { name: '', value: entry }
    })
  }
  return [{ name: '', value: '' }]
})
</script>

<template>
  <div class="property-editor mb-3">
    <label class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
      {{ property.displayName }}
      <span v-if="property.required" class="text-red-500">*</span>
    </label>

    <textarea
      v-if="property.type === 'string' && isTextarea"
      :value="textValue"
      :rows="rows"
      readonly
      class="w-full px-2.5 py-1.5 text-xs font-mono border border-slate-200 dark:border-slate-700 rounded-md bg-slate-50 dark:bg-slate-900 text-slate-700 dark:text-slate-200"
    />

    <select
      v-else-if="property.type === 'options'"
      :value="typeof value === 'string' ? value : (typeof property.default === 'string' ? property.default : '')"
      disabled
      class="w-full px-2.5 py-1.5 text-xs border border-slate-200 dark:border-slate-700 rounded-md bg-slate-50 dark:bg-slate-900 text-slate-700 dark:text-slate-200"
    >
      <option v-for="opt in property.options ?? []" :key="String(opt.value)" :value="opt.value">
        {{ opt.name }}
      </option>
    </select>

    <div v-else-if="property.type === 'multiOptions'" class="space-y-1">
      <label
        v-for="opt in property.options ?? []"
        :key="String(opt.value)"
        class="flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300"
      >
        <input
          type="checkbox"
          :checked="Array.isArray(value) && (value as unknown[]).includes(opt.value)"
          disabled
          class="rounded border-slate-300"
        />
        {{ opt.name }}
      </label>
    </div>

    <input
      v-else-if="property.type === 'number'"
      type="number"
      :value="numberValue"
      readonly
      class="w-full px-2.5 py-1.5 text-xs border border-slate-200 dark:border-slate-700 rounded-md bg-slate-50 dark:bg-slate-900 text-slate-700 dark:text-slate-200"
    />

    <label v-else-if="property.type === 'boolean'" class="inline-flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300">
      <input type="checkbox" :checked="boolValue" disabled class="rounded border-slate-300" />
      {{ boolValue ? 'true' : 'false' }}
    </label>

    <textarea
      v-else-if="property.type === 'json'"
      :value="displayString(value ?? inferDefaultString(property))"
      rows="5"
      readonly
      class="w-full px-2.5 py-1.5 text-xs font-mono border border-slate-200 dark:border-slate-700 rounded-md bg-slate-50 dark:bg-slate-900 text-slate-700 dark:text-slate-200"
    />

    <input
      v-else-if="property.type === 'dateTime'"
      type="datetime-local"
      :value="textValue"
      readonly
      class="w-full px-2.5 py-1.5 text-xs border border-slate-200 dark:border-slate-700 rounded-md bg-slate-50 dark:bg-slate-900 text-slate-700 dark:text-slate-200"
    />

    <div v-else-if="property.type === 'collection'" class="space-y-1.5">
      <div
        v-for="(row, idx) in collectionRows"
        :key="idx"
        class="flex items-center gap-2 text-xs"
      >
        <input
          :value="row.name"
          readonly
          placeholder="Name"
          class="flex-1 px-2 py-1 border border-slate-200 dark:border-slate-700 rounded-md bg-slate-50 dark:bg-slate-900 font-mono"
        />
        <input
          :value="displayString(row.value)"
          readonly
          placeholder="Value"
          class="flex-1 px-2 py-1 border border-slate-200 dark:border-slate-700 rounded-md bg-slate-50 dark:bg-slate-900 font-mono"
        />
      </div>
    </div>

    <div v-else-if="property.type === 'fixedCollection'" class="text-xs italic text-slate-500 dark:text-slate-400">
      Fixed collection (see JSON view).
    </div>

    <input
      v-else
      type="text"
      :value="displayString(value ?? inferDefaultString(property))"
      readonly
      class="w-full px-2.5 py-1.5 text-xs border border-slate-200 dark:border-slate-700 rounded-md bg-slate-50 dark:bg-slate-900 text-slate-700 dark:text-slate-200"
    />

    <p v-if="property.description" class="mt-1 text-[11px] text-slate-500 dark:text-slate-400">
      {{ property.description }}
    </p>
  </div>
</template>
