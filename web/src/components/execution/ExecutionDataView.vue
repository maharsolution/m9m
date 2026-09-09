<script setup lang="ts">
/**
 * ExecutionDataView — renders a DataItem[] in one of three modes:
 *
 *   - `schema` : a single-column table showing every key found
 *                 across all items and its inferred type.
 *   - `table`  : a multi-column table where each top-level key
 *                 becomes a column; rows are items.
 *   - `json`   : pretty-printed JSON.
 *
 * This is the n8n-style Input/Output panel. It deliberately avoids
 * any per-key filtering so the user always sees the full payload
 * (n8n does the same — switching between Schema/Table/JSON does
 * not drop fields).
 */
import { computed } from 'vue'
import type { DataItem } from '@/types'

interface Props {
  data: DataItem[] | undefined
  mode: 'schema' | 'table' | 'json'
  emptyLabel?: string
  /** Cap on rendered rows for very large payloads. */
  maxRows?: number
}

const props = withDefaults(defineProps<Props>(), {
  emptyLabel: 'No data',
  maxRows: 200,
})

const safeData = computed(() => Array.isArray(props.data) ? props.data : [])

// Schema view: union of all keys across all items, with their
// inferred JSON types. Mirrors n8n's "Schema" tab.
const schemaEntries = computed(() => {
  const typeMap = new Map<string, Set<string>>()
  for (const item of safeData.value) {
    if (!item || !item.json) continue
    for (const [k, v] of Object.entries(item.json)) {
      if (!typeMap.has(k)) typeMap.set(k, new Set())
      typeMap.get(k)!.add(inferType(v))
    }
  }
  return Array.from(typeMap.entries()).map(([name, types]) => ({
    name,
    type: Array.from(types).join(' | '),
  })).sort((a, b) => a.name.localeCompare(b.name))
})

function inferType(v: unknown): string {
  if (v === null) return 'null'
  if (Array.isArray(v)) return 'array'
  return typeof v
}

// Table view: each top-level key becomes a column, each item is a
// row. Mirrors n8n's "Table" tab.
const tableColumns = computed(() => schemaEntries.value.map((e) => e.name))
const tableRows = computed(() => safeData.value.slice(0, props.maxRows).map((item) => {
  if (!item?.json) return {} as Record<string, unknown>
  // Only project top-level keys so the column set stays stable.
  const out: Record<string, unknown> = {}
  for (const col of tableColumns.value) {
    out[col] = item.json[col]
  }
  return out
}))

// JSON view: pretty-print with a hard size cap so a runaway payload
// can't lock up the browser. Truncation suffix mirrors
// ExecutionDetail.vue's existing behaviour.
function truncate(s: string, max = 20_000): string {
  return s.length > max ? s.slice(0, max) + '\n…(truncated)' : s
}
const jsonText = computed(() => truncate(JSON.stringify(safeData.value, null, 2)))

// Cell renderer: string/object/array → truncated string for the
// table view. Avoids a per-cell JSON.stringify on every render.
function renderCell(value: unknown): string {
  if (value === undefined) return ''
  if (value === null) return 'null'
  if (typeof value === 'object') {
    const s = JSON.stringify(value)
    return s.length > 80 ? s.slice(0, 80) + '…' : s
  }
  const s = String(value)
  return s.length > 80 ? s.slice(0, 80) + '…' : s
}
</script>

<template>
  <div class="execution-data-view">
    <div v-if="safeData.length === 0" class="px-4 py-3 text-xs italic text-slate-500 dark:text-slate-400">
      {{ emptyLabel }}
    </div>

    <div v-else-if="mode === 'schema'" class="overflow-x-auto">
      <table class="min-w-full text-xs">
        <thead>
          <tr class="bg-slate-100 dark:bg-slate-900">
            <th class="text-left px-3 py-2 font-semibold text-slate-700 dark:text-slate-300 w-1/2">
              Name
            </th>
            <th class="text-left px-3 py-2 font-semibold text-slate-700 dark:text-slate-300">
              Type
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="entry in schemaEntries"
            :key="entry.name"
            class="border-b border-slate-100 dark:border-slate-700"
          >
            <td class="px-3 py-1.5 font-mono text-slate-700 dark:text-slate-300">{{ entry.name }}</td>
            <td class="px-3 py-1.5 text-slate-500 dark:text-slate-400">{{ entry.type }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-else-if="mode === 'table'" class="overflow-x-auto">
      <table class="min-w-full text-xs">
        <thead>
          <tr class="bg-slate-100 dark:bg-slate-900">
            <th
              v-for="col in tableColumns"
              :key="col"
              class="text-left px-3 py-2 font-semibold text-slate-700 dark:text-slate-300 whitespace-nowrap"
            >
              {{ col }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(row, idx) in tableRows"
            :key="idx"
            class="border-b border-slate-100 dark:border-slate-700"
          >
            <td
              v-for="col in tableColumns"
              :key="col"
              class="px-3 py-1.5 font-mono text-slate-600 dark:text-slate-300 whitespace-nowrap"
            >
              {{ renderCell(row[col]) }}
            </td>
          </tr>
        </tbody>
      </table>
      <div v-if="safeData.length > maxRows" class="px-3 py-2 text-[10px] italic text-slate-500 dark:text-slate-400">
        Showing {{ maxRows }} of {{ safeData.length }} items.
      </div>
    </div>

    <div v-else>
      <pre class="text-xs text-slate-600 dark:text-slate-400 whitespace-pre-wrap font-mono bg-slate-50 dark:bg-slate-900 p-3 rounded-lg overflow-auto max-h-[420px]">{{ jsonText }}</pre>
    </div>
  </div>
</template>
