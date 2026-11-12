<script setup lang="ts" generic="T extends object">
/**
 * DataTable — m9m shared primitive.
 *
 * Header sits on the subtle surface, rows alternate between the
 * surface and the subtle surface, and the hovered row gets a tint
 * (DESIGN.md §7.8). Cells can be overridden per column with a
 * scoped slot named after the column key.
 */
export interface DataTableColumn {
  /** Column key — also the name of the cell slot. */
  key: string
  /** Header label. */
  label: string
  /** Column width, any CSS width value. */
  width?: string
  /** Horizontal alignment. */
  align?: 'left' | 'center' | 'right'
}

const props = withDefaults(
  defineProps<{
    /** Column descriptors. */
    columns: DataTableColumn[]
    /** Row data. */
    rows: T[]
    /** Property used as the row key. */
    rowKey?: string
    /** Shows the loading state instead of the rows. */
    loading?: boolean
    /** Copy shown when there are no rows. */
    emptyText?: string
    /** Makes rows clickable. */
    clickable?: boolean
  }>(),
  {
    rowKey: 'id',
    loading: false,
    emptyText: 'No records found',
    clickable: false,
  }
)

const emit = defineEmits<{
  rowClick: [row: T]
}>()

function alignClass(align?: DataTableColumn['align']) {
  if (align === 'center') return 'text-center'
  if (align === 'right') return 'text-right'
  return 'text-left'
}

function rowId(row: T, index: number): string {
  const record = row as Record<string, unknown>
  const value = record[props.rowKey]
  return value === undefined || value === null ? String(index) : String(value)
}

function cellValue(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key]
}

function handleRowClick(row: T) {
  if (!props.clickable) return
  emit('rowClick', row)
}
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
    <div v-if="loading" class="flex items-center justify-center py-12">
      <span
        class="inline-block h-6 w-6 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
        aria-hidden="true"
      />
      <span class="ml-3 text-sm text-neutral-600 dark:text-neutral-400">Loading…</span>
    </div>

    <div v-else-if="rows.length === 0" class="px-4 py-12 text-center">
      <slot name="empty">
        <p class="text-sm text-neutral-600 dark:text-neutral-400">{{ emptyText }}</p>
      </slot>
    </div>

    <div v-else class="overflow-x-auto">
      <table class="w-full border-collapse text-sm">
        <thead>
          <tr class="bg-neutral-50 dark:bg-neutral-950">
            <th
              v-for="column in columns"
              :key="column.key"
              scope="col"
              class="px-4 py-2.5 text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400"
              :class="alignClass(column.align)"
              :style="column.width ? { width: column.width } : undefined"
            >
              <!-- `header-<key>` slot lets a column own its header cell
                   (selection checkboxes, filter controls, …). -->
              <slot :name="`header-${column.key}`" :column="column">
                {{ column.label }}
              </slot>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(row, index) in rows"
            :key="rowId(row, index)"
            class="border-t border-neutral-100 transition-colors duration-200 dark:border-neutral-800"
            :class="[
              index % 2 === 1
                ? 'bg-neutral-50/60 dark:bg-neutral-950/40'
                : 'bg-white dark:bg-neutral-900',
              clickable
                ? 'cursor-pointer hover:bg-neutral-100 dark:hover:bg-neutral-800/60'
                : '',
            ]"
            @click="handleRowClick(row)"
          >
            <td
              v-for="column in columns"
              :key="column.key"
              class="px-4 py-3 text-neutral-800 dark:text-neutral-200"
              :class="alignClass(column.align)"
            >
              <slot :name="column.key" :row="row" :value="cellValue(row, column.key)" :index="index">
                {{ cellValue(row, column.key) }}
              </slot>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
