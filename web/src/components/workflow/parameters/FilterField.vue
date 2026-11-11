<script setup lang="ts">
import { computed } from 'vue'
import { PlusIcon, TrashIcon } from '@heroicons/vue/24/outline'
import type { NodeProperty } from '@/types/node'

/**
 * Filter parameter renderer (`filter`).
 *
 * n8n stores a filter as its `FilterValue`:
 *
 * ```json
 * {
 *   "options": { "caseSensitive": true, "leftValue": "",
 *                "typeValidation": "strict", "version": 2 },
 *   "conditions": [
 *     { "id": "…", "leftValue": "{{ $json.name }}",
 *       "operator": { "type": "string", "operation": "equals" },
 *       "rightValue": "n8n" }
 *   ],
 *   "combinator": "and"
 * }
 * ```
 *
 * The metadata lists the operators as `"<type>:<operation>"` ids
 * (e.g. `string:equals`); the renderer splits them into the two
 * halves of the stored `operator` object.
 */
interface FilterOperator {
  type: string
  operation: string
}

interface FilterCondition {
  id: string
  leftValue: string
  operator: FilterOperator
  rightValue: string
}

interface FilterValue {
  options: {
    caseSensitive: boolean
    leftValue: string
    typeValidation: string
    version: number
  }
  conditions: FilterCondition[]
  combinator: string
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
  (event: 'update:modelValue', value: FilterValue): void
}>()

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** The operators that take no right-hand value. */
const SINGLE_VALUE_OPERATIONS = new Set([
  'empty',
  'notEmpty',
  'exists',
  'notExists',
  'true',
  'false',
])

let idCounter = 0

function generateId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  idCounter += 1
  return `condition-${idCounter}`
}

function splitOperatorId(id: string): FilterOperator {
  const separator = id.indexOf(':')
  if (separator === -1) return { type: 'string', operation: id }
  return {
    type: id.slice(0, separator),
    operation: id.slice(separator + 1),
  }
}

function joinOperatorId(operator: FilterOperator): string {
  return `${operator.type}:${operator.operation}`
}

function defaultFilter(): FilterValue {
  return {
    options: {
      caseSensitive: true,
      leftValue: '',
      typeValidation: 'strict',
      version: 2,
    },
    conditions: [],
    combinator: 'and',
  }
}

/** Normalise whatever is stored (or missing) into the n8n shape. */
function normalize(raw: unknown): FilterValue {
  const fallback = defaultFilter()
  if (!isPlainObject(raw)) return fallback

  const rawOptions = isPlainObject(raw.options) ? raw.options : {}
  const options = {
    caseSensitive:
      typeof rawOptions.caseSensitive === 'boolean'
        ? rawOptions.caseSensitive
        : fallback.options.caseSensitive,
    leftValue:
      typeof rawOptions.leftValue === 'string' ? rawOptions.leftValue : '',
    typeValidation:
      typeof rawOptions.typeValidation === 'string'
        ? rawOptions.typeValidation
        : fallback.options.typeValidation,
    version:
      typeof rawOptions.version === 'number'
        ? rawOptions.version
        : fallback.options.version,
  }

  const conditions: FilterCondition[] = []
  if (Array.isArray(raw.conditions)) {
    for (const entry of raw.conditions) {
      if (!isPlainObject(entry)) continue
      const rawOperator = entry.operator
      let operator: FilterOperator = { type: 'string', operation: 'equals' }
      if (typeof rawOperator === 'string') {
        operator = splitOperatorId(rawOperator)
      } else if (isPlainObject(rawOperator)) {
        operator = {
          type: typeof rawOperator.type === 'string' ? rawOperator.type : 'string',
          operation:
            typeof rawOperator.operation === 'string'
              ? rawOperator.operation
              : 'equals',
        }
      }
      conditions.push({
        id: typeof entry.id === 'string' && entry.id ? entry.id : generateId(),
        leftValue: typeof entry.leftValue === 'string' ? entry.leftValue : '',
        operator,
        rightValue: typeof entry.rightValue === 'string' ? entry.rightValue : '',
      })
    }
  }

  return {
    options,
    conditions,
    combinator:
      typeof raw.combinator === 'string' && raw.combinator
        ? raw.combinator
        : fallback.combinator,
  }
}

const state = computed<FilterValue>(() => normalize(props.modelValue))

/** The operators offered by the metadata. */
const operators = computed(() => props.property.values ?? [])

/** The combinators the property allows. */
const allowedCombinators = computed<string[]>(() => {
  const typeOptions = props.property.typeOptions
  if (!typeOptions) return []
  const filterOptions = typeOptions.filter
  if (!isPlainObject(filterOptions)) return []
  const combinators = filterOptions.allowedCombinators
  if (!Array.isArray(combinators)) return []
  return combinators.map((entry) => String(entry))
})

function emitValue(next: FilterValue) {
  emit('update:modelValue', next)
}

/** True when the operator takes no right-hand value. */
function isSingleValue(operator: FilterOperator): boolean {
  return SINGLE_VALUE_OPERATIONS.has(operator.operation)
}

function onCombinatorChange(event: Event) {
  const combinator = (event.target as HTMLSelectElement).value
  emitValue({ ...state.value, combinator })
}

function updateCondition(index: number, patch: Partial<FilterCondition>) {
  const conditions = state.value.conditions.map((condition, i) =>
    i === index ? { ...condition, ...patch } : { ...condition },
  )
  emitValue({ ...state.value, conditions })
}

function onLeftValueChange(index: number, leftValue: string) {
  updateCondition(index, { leftValue })
}

function onOperatorChange(index: number, operatorId: string) {
  const operator = splitOperatorId(operatorId)
  const patch: Partial<FilterCondition> = { operator }
  // The single-value operators ignore the right-hand side — keep the
  // stored value empty so the saved workflow matches n8n.
  if (isSingleValue(operator)) patch.rightValue = ''
  updateCondition(index, patch)
}

function onRightValueChange(index: number, rightValue: string) {
  updateCondition(index, { rightValue })
}

function addCondition() {
  const conditions = [
    ...state.value.conditions.map((condition) => ({ ...condition })),
    {
      id: generateId(),
      leftValue: '',
      operator: { type: 'string', operation: 'equals' },
      rightValue: '',
    },
  ]
  emitValue({ ...state.value, conditions })
}

function removeCondition(index: number) {
  const conditions = state.value.conditions
    .filter((_, i) => i !== index)
    .map((condition) => ({ ...condition }))
  emitValue({ ...state.value, conditions })
}
</script>

<template>
  <div class="space-y-2">
    <select
      v-if="allowedCombinators.length"
      :value="state.combinator"
      class="input w-auto"
      :disabled="disabled"
      @change="onCombinatorChange"
    >
      <option
        v-for="combinator in allowedCombinators"
        :key="combinator"
        :value="combinator"
      >
        {{ combinator === 'and' ? 'All conditions (AND)' : 'Any condition (OR)' }}
      </option>
    </select>

    <div
      v-for="(condition, index) in state.conditions"
      :key="condition.id"
      class="rounded border border-slate-200 dark:border-slate-600 p-2 space-y-1"
    >
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-slate-500 dark:text-slate-400">
          Condition {{ index + 1 }}
        </span>
        <button
          type="button"
          class="p-1 text-red-500 hover:text-red-600 disabled:opacity-30"
          title="Remove condition"
          :disabled="disabled"
          @click="removeCondition(index)"
        >
          <TrashIcon class="w-3.5 h-3.5" />
        </button>
      </div>

      <input
        :value="condition.leftValue"
        type="text"
        class="input text-sm font-mono"
        placeholder="Left value (e.g. {{ $json.name }})"
        :disabled="disabled"
        @input="onLeftValueChange(index, ($event.target as HTMLInputElement).value)"
      />

      <select
        :value="joinOperatorId(condition.operator)"
        class="input text-sm"
        :disabled="disabled"
        @change="onOperatorChange(index, ($event.target as HTMLSelectElement).value)"
      >
        <option
          v-for="operator in operators"
          :key="operator.value"
          :value="operator.value"
        >
          {{ operator.name }}
        </option>
      </select>

      <input
        v-if="!isSingleValue(condition.operator)"
        :value="condition.rightValue"
        type="text"
        class="input text-sm font-mono"
        placeholder="Right value"
        :disabled="disabled"
        @input="onRightValueChange(index, ($event.target as HTMLInputElement).value)"
      />
    </div>

    <button
      type="button"
      class="flex items-center gap-1 text-xs text-primary-600 dark:text-primary-400 hover:underline disabled:opacity-40"
      :disabled="disabled"
      @click="addCondition"
    >
      <PlusIcon class="w-3.5 h-3.5" />
      Add condition
    </button>

    <p
      v-if="!state.conditions.length"
      class="text-xs text-slate-500 dark:text-slate-400"
    >
      No conditions yet.
    </p>
  </div>
</template>
