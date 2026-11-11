<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { NodeProperty, NodePropertyOption } from '@/types/node'
import { useNodeTypesStore } from '@/stores/nodeTypes'

/**
 * Resource-locator parameter renderer.
 *
 * n8n stores a resource locator as `{ __rl: true, value, mode }`: the
 * mode selects *how* the value is interpreted (list / url / id …) and
 * the value is the raw locator string.
 *
 * When the property declares an option with an `action`, the option
 * list is fetched from the backend
 * (`GET /api/v1/node-types/{name}/options`). That endpoint is optional:
 * when it is missing the field degrades to a plain text input and says
 * so — the panel never fails because of it.
 */
const props = withDefaults(
  defineProps<{
    property: NodeProperty
    modelValue?: unknown
    /** Name of the node type — used to probe the dynamic options endpoint. */
    nodeTypeName?: string
    /** Current parameter values — sent along with the options probe. */
    parameters?: Record<string, unknown>
    disabled?: boolean
  }>(),
  {
    nodeTypeName: '',
    parameters: () => ({}),
    disabled: false,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: Record<string, unknown>): void
}>()

const nodeTypesStore = useNodeTypesStore()

const dynamicOptions = ref<NodePropertyOption[] | null>(null)
const dynamicUnavailable = ref(false)
const loadingOptions = ref(false)

/** The n8n resource-locator envelope, normalised from whatever we got. */
const locator = computed<{ value: string; mode: string }>(() => {
  const raw = props.modelValue
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    const record = raw as Record<string, unknown>
    return {
      value: typeof record.value === 'string' ? record.value : '',
      mode: typeof record.mode === 'string' ? record.mode : '',
    }
  }
  if (typeof raw === 'string') return { value: raw, mode: '' }
  return { value: '', mode: '' }
})

/** Available modes — `typeOptions.modes` first, options list as fallback. */
const modes = computed<NodePropertyOption[]>(() => {
  const declared = props.property.typeOptions?.modes
  if (declared && declared.length) return declared
  const options = props.property.options ?? []
  if (options.length) {
    return options.map((option) => ({
      name: option.name,
      value: option.value,
      displayName: option.displayName ?? option.name,
    }))
  }
  return []
})

const currentMode = computed(() => {
  if (locator.value.mode) return locator.value.mode
  if (modes.value.length) return String(modes.value[0].value)
  const fallback = props.property.default
  return typeof fallback === 'string' ? fallback : ''
})

/** Does this property want a server-provided option list? */
const wantsDynamicOptions = computed(() =>
  (props.property.options ?? []).some((option) => Boolean(option.action)),
)

/** Options rendered in the picker: dynamic when available, else static. */
const availableOptions = computed<NodePropertyOption[]>(() => {
  if (dynamicOptions.value) return dynamicOptions.value
  return props.property.options ?? []
})

async function loadDynamicOptions() {
  if (!wantsDynamicOptions.value || !props.nodeTypeName) return
  loadingOptions.value = true
  const options = await nodeTypesStore.fetchOptions(
    props.nodeTypeName,
    props.property.name,
    props.parameters,
  )
  loadingOptions.value = false
  if (options && options.length) {
    dynamicOptions.value = options.map((option) => ({
      name: option.name,
      value: option.value,
    }))
    dynamicUnavailable.value = false
    return
  }
  // No endpoint (404) or an empty answer — degrade to a plain text field.
  dynamicUnavailable.value = true
}

onMounted(loadDynamicOptions)
watch(
  () => [props.property.name, props.nodeTypeName],
  () => {
    dynamicOptions.value = null
    dynamicUnavailable.value = false
    void loadDynamicOptions()
  },
)

function emitLocator(value: string, mode: string) {
  emit('update:modelValue', { __rl: true, value, mode })
}

function onValueInput(event: Event) {
  emitLocator((event.target as HTMLInputElement).value, currentMode.value)
}

function onModeChange(event: Event) {
  const mode = (event.target as HTMLSelectElement).value
  emitLocator(locator.value.value, mode)
}
</script>

<template>
  <div class="space-y-1">
    <div class="flex items-center gap-2">
      <select
        v-if="modes.length > 1"
        :value="currentMode"
        class="input w-auto shrink-0"
        :disabled="disabled"
        @change="onModeChange"
      >
        <option
          v-for="mode in modes"
          :key="String(mode.value)"
          :value="String(mode.value)"
        >
          {{ mode.displayName ?? mode.name }}
        </option>
      </select>
      <input
        :value="locator.value"
        type="text"
        class="input flex-1"
        :placeholder="property.placeholder"
        :disabled="disabled"
        @input="onValueInput"
      />
    </div>

    <select
      v-if="!modes.length && availableOptions.length"
      :value="locator.value"
      class="input"
      :disabled="disabled"
      @change="onValueInput"
    >
      <option value="">— select —</option>
      <option
        v-for="option in availableOptions"
        :key="String(option.value)"
        :value="String(option.value)"
      >
        {{ option.name }}
      </option>
    </select>

    <p
      v-if="loadingOptions"
      class="text-xs text-slate-500 dark:text-slate-400"
    >
      Loading options…
    </p>
    <p
      v-else-if="dynamicUnavailable"
      class="text-xs text-amber-600 dark:text-amber-400"
    >
      Dynamic options are unavailable on this server — enter the value manually.
    </p>
  </div>
</template>
