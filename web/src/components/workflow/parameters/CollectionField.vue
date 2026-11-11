<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  PlusIcon,
  TrashIcon,
} from '@heroicons/vue/24/outline'
import type { NodeProperty } from '@/types/node'
import {
  collectionFieldDefs,
  defaultRow,
  isListCollection,
  isPlainObject,
  toRows,
} from './collectionModel'

/**
 * Collection renderer for `collection` / `fixedCollection` properties.
 *
 * The renderer is metadata-driven: the row fields come from the
 * property's declared `options[].values` when the backend provides
 * them, otherwise they are inferred from the stored rows so existing
 * workflows stay editable. Rows can be added, removed and reordered.
 *
 * `ParameterField` is imported lazily — the two components reference
 * each other (a collection may contain nested collections).
 */
const ParameterField = defineAsyncComponent(() => import('./ParameterField.vue'))

const props = withDefaults(
  defineProps<{
    property: NodeProperty
    modelValue?: unknown
    /** Current parameter values of the node (used by nested fields). */
    parameters?: Record<string, unknown>
    disabled?: boolean
  }>(),
  {
    parameters: () => ({}),
    disabled: false,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: unknown): void
}>()

/** True when the collection stores its rows as a list. */
const listMode = computed(() => isListCollection(props.property, props.modelValue))

/** The stored rows. */
const rows = computed<Record<string, unknown>[]>(() => {
  const value = props.modelValue
  if (isPlainObject(value)) {
    const bucket = props.property.options?.find((option) => option.name in value)
    if (bucket) return toRows(value[bucket.name])
  }
  return toRows(value)
})

/** The field definitions every row is rendered with. */
const defs = computed(() => collectionFieldDefs(props.property, rows.value[0]))

/**
 * n8n stores a single collection bucket wrapped in an object keyed by
 * the option name (`{ values: { …fields } }`). The key is preserved so
 * saving keeps the original shape.
 */
const bucketKey = computed<string | null>(() => {
  const value = props.modelValue
  if (!isPlainObject(value)) {
    if (value == null && props.property.type === 'fixedCollection') {
      return props.property.options?.[0]?.name ?? null
    }
    return null
  }
  const declared = props.property.options?.find((option) => option.name in value)
  if (declared) return declared.name
  if (props.property.type === 'fixedCollection') {
    const first = props.property.options?.[0]
    if (first?.values?.length) return first.name
  }
  const keys = Object.keys(value)
  if (keys.length !== 1) return null
  return isPlainObject(value[keys[0]]) ? keys[0] : null
})

function emitRows(next: Record<string, unknown>[]) {
  if (listMode.value) {
    emit('update:modelValue', bucketKey.value ? { ...((isPlainObject(props.modelValue) && props.modelValue) || {}), [bucketKey.value]: next } : next)
    return
  }
  const row = next[0] ?? {}
  if (bucketKey.value) {
    emit('update:modelValue', { [bucketKey.value]: row })
    return
  }
  emit('update:modelValue', row)
}

function updateField(index: number, name: string, value: unknown) {
  const next = rows.value.map((row, i) =>
    i === index ? { ...row, [name]: value } : { ...row },
  )
  emitRows(next)
}

function addRow() {
  const next = [...rows.value.map((row) => ({ ...row })), defaultRow(defs.value)]
  emitRows(next)
}

function removeRow(index: number) {
  const next = rows.value.filter((_, i) => i !== index).map((row) => ({ ...row }))
  emitRows(next)
}

function moveRow(index: number, delta: number) {
  const target = index + delta
  if (target < 0 || target >= rows.value.length) return
  const next = rows.value.map((row) => ({ ...row }))
  const moved = next[index]
  next[index] = next[target]
  next[target] = moved
  emitRows(next)
}
</script>

<template>
  <div class="space-y-2">
    <div
      v-for="(row, index) in rows"
      :key="index"
      class="rounded border border-slate-200 dark:border-slate-600 p-2 space-y-2"
    >
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-slate-500 dark:text-slate-400">
          {{ property.displayName }} {{ rows.length > 1 ? index + 1 : '' }}
        </span>
        <div class="flex items-center gap-1">
          <button
            v-if="listMode"
            type="button"
            class="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 disabled:opacity-30"
            title="Move up"
            :disabled="disabled || index === 0"
            @click="moveRow(index, -1)"
          >
            <ArrowUpIcon class="w-3.5 h-3.5" />
          </button>
          <button
            v-if="listMode"
            type="button"
            class="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 disabled:opacity-30"
            title="Move down"
            :disabled="disabled || index === rows.length - 1"
            @click="moveRow(index, 1)"
          >
            <ArrowDownIcon class="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            class="p-1 text-red-500 hover:text-red-600 disabled:opacity-30"
            title="Remove"
            :disabled="disabled"
            @click="removeRow(index)"
          >
            <TrashIcon class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      <div
        v-for="def in defs"
        :key="def.name"
        class="space-y-1"
      >
        <label class="block text-xs font-medium text-slate-600 dark:text-slate-300">
          {{ def.displayName }}
          <span v-if="def.required" class="text-red-500">*</span>
        </label>
        <component
          :is="ParameterField"
          :property="{
            name: def.name,
            displayName: def.displayName,
            type: def.type,
            default: def.default,
            required: def.required,
            description: def.description,
            placeholder: def.placeholder,
            options: def.options,
            typeOptions: def.typeOptions,
          }"
          :model-value="row[def.name]"
          :parameters="parameters"
          :disabled="disabled"
          @update:model-value="updateField(index, def.name, $event)"
        />
      </div>
    </div>

    <button
      v-if="listMode"
      type="button"
      class="flex items-center gap-1 text-xs text-primary-600 dark:text-primary-400 hover:underline disabled:opacity-40"
      :disabled="disabled"
      @click="addRow"
    >
      <PlusIcon class="w-3.5 h-3.5" />
      Add {{ property.displayName }}
    </button>

    <p
      v-if="!rows.length"
      class="text-xs text-slate-500 dark:text-slate-400"
    >
      No entries yet.
    </p>
  </div>
</template>
