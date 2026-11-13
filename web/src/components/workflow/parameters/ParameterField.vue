<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue'
import type { NodeProperty } from '@/types/node'
import { useWorkflowStore } from '@/stores/workflow'
import { EXPRESSION_CONTEXT_KEY } from '@/lib/expressionContext'
import CollectionField from './CollectionField.vue'
import ExpressionField from './ExpressionField.vue'
import ExpressionInput from './ExpressionInput.vue'
import FileField from './FileField.vue'
import FilterField from './FilterField.vue'
import IconField from './IconField.vue'
import MultiSelectField from './MultiSelectField.vue'
import ResourceLocatorField from './ResourceLocatorField.vue'
import ResourceMapperField from './ResourceMapperField.vue'
import WebhookUrlField from './WebhookUrlField.vue'
import { isWebhookNodeType } from './webhookUrls'

/**
 * Metadata-driven parameter renderer.
 *
 * One component per n8n property type. The renderer never assumes a
 * specific node: everything it needs comes from the property
 * descriptor (type, default, required, placeholder, options,
 * typeOptions) plus the node's current parameter values.
 *
 * Unknown property types degrade to a plain text input — the panel
 * never crashes on a property it does not understand.
 */
const props = withDefaults(
  defineProps<{
    property: NodeProperty
    modelValue?: unknown
    /** Current parameter values of the node (context for nested fields). */
    parameters?: Record<string, unknown>
    /** Name of the node type (used to probe dynamic option endpoints). */
    nodeTypeName?: string
    disabled?: boolean
  }>(),
  {
    parameters: () => ({}),
    nodeTypeName: '',
    disabled: false,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: unknown): void
}>()

const typeOptions = computed(() => props.property.typeOptions ?? {})

/** The effective value: the stored one, or the declared default. */
const value = computed(() =>
  props.modelValue === undefined ? props.property.default : props.modelValue,
)

/** True for property types the renderer has no dedicated widget for. */
const unsupportedType = computed(() => {
  const known = [
    'string',
    'number',
    'boolean',
    'options',
    'multiOptions',
    'json',
    'code',
    'collection',
    'fixedCollection',
    'resourceLocator',
    'resourceMapper',
    'filter',
    'file',
    'icon',
    'dateTime',
    'color',
  ]
  return !known.includes(props.property.type)
})

// ---------------------------------------------------------------------
// webhook URLs
// ---------------------------------------------------------------------

/**
 * True for the Webhook node's `path` parameter. n8n shows BOTH the
 * production URL (`/webhook/<path>`) and the test URL
 * (`/webhook-test/<path>`) next to the path field — the test URL is the
 * one that works while the workflow is still inactive, which is how the
 * input / output of a flow is captured during design.
 */
const isWebhookPathField = computed(
  () => props.property.name === 'path' && isWebhookNodeType(props.nodeTypeName),
)

// ---------------------------------------------------------------------
// string
// ---------------------------------------------------------------------

const stringRows = computed(() => {
  const rows = typeOptions.value.rows
  return typeof rows === 'number' && rows > 1 ? rows : 0
})

const isPassword = computed(() => typeOptions.value.password === true)

function onStringInput(next: string) {
  emit('update:modelValue', next)
}

// ---------------------------------------------------------------------
// expression editor
// ---------------------------------------------------------------------

const workflowStore = useWorkflowStore()

/**
 * The completion data of the open node (the fields of the current item
 * and the fields every other node produced). The node details view
 * provides it; a field rendered outside of it simply has no data.
 */
const expressionContext = inject(EXPRESSION_CONTEXT_KEY, null)

/**
 * Nodes of the current workflow, offered to the expression editor so
 * `$node["Set"]` can be autocompleted.
 */
const availableNodes = computed(() =>
  (workflowStore.currentWorkflow?.nodes ?? []).map((node) => ({
    name: node.name,
    type: node.type,
    fields: expressionContext?.value.nodeFields[node.name] ?? [],
  })),
)

/** The fields of the current item (`$json.<field>`). */
const itemFields = computed(() => expressionContext?.value.itemFields ?? [])

/** The fields per node (`$node["Set"].json.<field>`). */
const nodeFields = computed(() => expressionContext?.value.nodeFields ?? {})

/** The stored value as text — the expression editor works on strings. */
const stringValue = computed(() => {
  const current = value.value
  if (current === undefined || current === null) return ''
  return typeof current === 'string' ? current : String(current)
})

// ---------------------------------------------------------------------
// number
// ---------------------------------------------------------------------

const numberStep = computed(() => {
  const precision = typeOptions.value.numberPrecision
  if (typeof precision === 'number' && precision >= 0) {
    return 10 ** -precision
  }
  return undefined
})

function onNumberInput(event: Event) {
  const raw = (event.target as HTMLInputElement).value
  if (raw === '') {
    emit('update:modelValue', undefined)
    return
  }
  const parsed = Number(raw)
  emit('update:modelValue', Number.isNaN(parsed) ? undefined : parsed)
}

// ---------------------------------------------------------------------
// boolean
// ---------------------------------------------------------------------

function onBooleanChange(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).checked)
}

// ---------------------------------------------------------------------
// options
// ---------------------------------------------------------------------

const optionValues = computed(() => props.property.options ?? [])

const selectedOption = computed(() => {
  const current = value.value
  if (current === undefined || current === null) return ''
  return String(current)
})
function onOptionChange(event: Event) {
  emit('update:modelValue', (event.target as HTMLSelectElement).value)
}

// ---------------------------------------------------------------------
// json
// ---------------------------------------------------------------------

const jsonText = ref('')
const jsonError = ref('')

watch(
  () => props.modelValue,
  (current) => {
    if (current === undefined || current === null) {
      jsonText.value = ''
    } else if (typeof current === 'string') {
      jsonText.value = current
    } else {
      jsonText.value = JSON.stringify(current, null, 2)
    }
  },
  { immediate: true },
)

function onJsonInput(event: Event) {
  const raw = (event.target as HTMLTextAreaElement).value
  jsonText.value = raw
  if (raw.trim() === '') {
    jsonError.value = ''
    emit('update:modelValue', undefined)
    return
  }
  try {
    const parsed: unknown = JSON.parse(raw)
    jsonError.value = ''
    emit('update:modelValue', parsed)
  } catch (error) {
    jsonError.value = error instanceof Error ? error.message : 'Invalid JSON'
  }
}

// ---------------------------------------------------------------------
// code
// ---------------------------------------------------------------------

const codeRows = computed(() => {
  const rows = typeOptions.value.rows
  return typeof rows === 'number' && rows > 0 ? rows : 5
})

function onCodeInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
}

// ---------------------------------------------------------------------
// dateTime / color
// ---------------------------------------------------------------------

function onDateTimeInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="space-y-1">
    <!-- collection / fixedCollection -->
    <CollectionField
      v-if="property.type === 'collection' || property.type === 'fixedCollection'"
      :property="property"
      :model-value="value"
      :parameters="parameters"
      :disabled="disabled"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <!-- resourceLocator -->
    <ResourceLocatorField
      v-else-if="property.type === 'resourceLocator'"
      :property="property"
      :model-value="value"
      :node-type-name="nodeTypeName"
      :parameters="parameters"
      :disabled="disabled"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <!-- resourceMapper -->
    <ResourceMapperField
      v-else-if="property.type === 'resourceMapper'"
      :property="property"
      :model-value="value"
      :disabled="disabled"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <!-- filter -->
    <FilterField
      v-else-if="property.type === 'filter'"
      :property="property"
      :model-value="value"
      :disabled="disabled"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <!-- file -->
    <FileField
      v-else-if="property.type === 'file'"
      :property="property"
      :model-value="value"
      :disabled="disabled"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <!-- icon -->
    <IconField
      v-else-if="property.type === 'icon'"
      :property="property"
      :model-value="value"
      :disabled="disabled"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <!-- multiOptions -->
    <MultiSelectField
      v-else-if="property.type === 'multiOptions'"
      :model-value="value"
      :options="optionValues"
      :placeholder="property.placeholder"
      :disabled="disabled"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <!-- boolean -->
    <label
      v-else-if="property.type === 'boolean'"
      class="flex items-center gap-2"
    >
      <input
        type="checkbox"
        class="w-4 h-4 rounded border-slate-300 dark:border-slate-600 text-primary-600 focus:ring-primary-500"
        :checked="value === true"
        :disabled="disabled"
        @change="onBooleanChange"
      />
      <span class="text-sm text-slate-600 dark:text-slate-300">
        {{ property.description || property.displayName }}
      </span>
    </label>

    <!-- number -->
    <input
      v-else-if="property.type === 'number'"
      type="number"
      class="input"
      :value="value === undefined || value === null ? '' : Number(value)"
      :min="typeOptions.minValue"
      :max="typeOptions.maxValue"
      :step="numberStep"
      :placeholder="property.placeholder"
      :disabled="disabled"
      @input="onNumberInput"
    />

    <!-- options -->
    <select
      v-else-if="property.type === 'options' && optionValues.length"
      class="input"
      :value="selectedOption"
      :disabled="disabled"
      @change="onOptionChange"
    >
      <option value="">— select —</option>
      <option
        v-for="option in optionValues"
        :key="String(option.value)"
        :value="String(option.value)"
      >
        {{ option.name }}
      </option>
    </select>

    <!-- json -->
    <div
      v-else-if="property.type === 'json'"
      class="space-y-1"
    >
      <textarea
        :value="jsonText"
        :rows="typeOptions.rows ?? 4"
        class="input font-mono text-sm"
        :placeholder="property.placeholder"
        :disabled="disabled"
        @input="onJsonInput"
      />
      <p
        v-if="jsonError"
        class="text-xs text-red-500"
      >
        {{ jsonError }}
      </p>
    </div>

    <!-- code -->
    <textarea
      v-else-if="property.type === 'code'"
      :value="typeof value === 'string' ? value : ''"
      :rows="codeRows"
      class="input font-mono text-sm"
      :placeholder="property.placeholder"
      :disabled="disabled"
      @input="onCodeInput"
    />

    <!-- dateTime -->
    <input
      v-else-if="property.type === 'dateTime'"
      type="datetime-local"
      class="input"
      :value="typeof value === 'string' ? value : ''"
      :placeholder="property.placeholder"
      :disabled="disabled"
      @input="onDateTimeInput"
    />

    <!-- color -->
    <input
      v-else-if="property.type === 'color'"
      type="color"
      class="input h-9 p-1"
      :value="typeof value === 'string' ? value : '#000000'"
      :disabled="disabled"
      @input="onDateTimeInput"
    />

    <!-- string + everything unknown -->
    <div v-else class="space-y-1">
      <ExpressionField
        v-if="!isPassword"
        :model-value="stringValue"
        :property="property"
        :available-nodes="availableNodes"
        :item-fields="itemFields"
        :node-fields="nodeFields"
        :disabled="disabled"
        @update:model-value="onStringInput"
      />
      <ExpressionInput
        v-else
        :model-value="value"
        :placeholder="property.placeholder"
        input-type="password"
        :rows="stringRows"
        :disabled="disabled"
        @update:model-value="onStringInput"
      />
      <WebhookUrlField
        v-if="isWebhookPathField"
        :path="stringValue"
      />
      <p
        v-if="unsupportedType"
        class="text-xs text-amber-600 dark:text-amber-400"
      >
        Unsupported parameter type “{{ property.type }}” — rendered as text.
      </p>
    </div>
  </div>
</template>
