<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue'
import {
  XMarkIcon,
  TrashIcon,
  KeyIcon,
  PlusIcon,
} from '@heroicons/vue/24/outline'
import { useRouter } from 'vue-router'
import {
  useWorkflowEditorStore,
  useCredentialsStore,
  useWorkflowStore,
} from '@/stores'
import { useExecutionStore } from '@/stores/execution'
import { useNodeTypesStore } from '@/stores/nodeTypes'
import { getExecution, getExecutions } from '@/api/executions'
import { getNodeCategory } from '@/types/node'
import type { Credential } from '@/types/api'
import type { DataItem, WorkflowExecution } from '@/types'
import type { NodeProperty } from '@/types/node'
import { resolveNodeCredentialTypes } from '@/lib/credentialSchema'
import { EXPRESSION_CONTEXT_KEY } from '@/lib/expressionContext'
import { buildExpressionFieldSources } from '@/lib/expressionFields'
import { isSuggestionListOpen } from '@/lib/suggestionList'
import {
  deleteParameterValue,
  getParameterValue,
  setParameterValue,
} from '@/components/workflow/parameters/parameterPath'
import DisplayOptionsGuard from '@/components/workflow/parameters/DisplayOptionsGuard.vue'
import ParameterField from '@/components/workflow/parameters/ParameterField.vue'

/**
 * Node details view (NDV).
 *
 * n8n parity: a large modal overlay with three resizable columns —
 * Input | Parameters | Output. The parameters column holds the
 * node's parameter fields (credentials + node parameters); the input
 * and output columns are separate panels that show the data of the
 * workflow's last execution and fall back to an honest empty state
 * when there is none.
 *
 * Everything rendered in the parameters column comes from the node-type
 * metadata: the property list, the visibility rules (`displayOptions`)
 * and the credential slots. There is no hard-coded per-node field list.
 */
const workflowEditorStore = useWorkflowEditorStore()
const credentialsStore = useCredentialsStore()
const workflowStore = useWorkflowStore()
const executionStore = useExecutionStore()
const nodeTypesStore = useNodeTypesStore()
const router = useRouter()

const emit = defineEmits<{ close: [] }>()

/**
 * The node the NDV is showing. It is *not* the canvas selection: the
 * panel is opened by a double click and closed without touching the
 * selection (n8n parity — the node stays selected afterwards).
 */
const node = computed(() => workflowEditorStore.ndvNode)

/** The node-type metadata (properties, credentials, display name). */
const nodeType = computed(() => {
  if (!node.value) return null
  return nodeTypesStore.byName(node.value.type) ?? null
})

/** Every property the metadata declares — visibility is resolved per field. */
const properties = computed<NodeProperty[]>(() => nodeType.value?.properties ?? [])

const category = computed(() => {
  if (!node.value) return 'action'
  return getNodeCategory(node.value.type)
})

const localName = ref('')
const localParameters = ref<Record<string, unknown>>({})

// Mirror of `WorkflowNode.credentials` — id+name are required, type is
// implicit from the map key.
const localCredentials = ref<Record<string, BoundCredential>>({})

type BoundCredential = { id: string; name: string }
type LocalCredentialsMap = Record<string, BoundCredential>

// Load the catalog once; the detail endpoint carries the authoritative
// property list for the selected node type.
onMounted(() => {
  void nodeTypesStore.ensureLoaded()
})

watch(
  node,
  (newNode) => {
    if (newNode) {
      localName.value = newNode.name
      localParameters.value = { ...newNode.parameters }
      void nodeTypesStore.fetchNodeTypeDetail(newNode.type)
    }
    // The WorkflowNode.credentials map values are NodeCredential
    // objects with an optional id; coerce to BoundCredential which
    // requires id+name. n8n guarantees id+name are always present.
    const source = (newNode?.credentials ?? {}) as Record<
      string,
      { id?: string; name: string }
    >
    const next: LocalCredentialsMap = {}
    for (const [k, v] of Object.entries(source)) {
      if (v.id) next[k] = { id: v.id, name: v.name }
    }
    localCredentials.value = next
  },
  { immediate: true },
)

// --------------------------------------------------------------------
// Column geometry
//
// Three columns: Input | Parameters | Output. The widths are stored as
// percentages of the modal width; the parameters column never drops
// below 260px, and every drag lands on the 8px resize grid.
// --------------------------------------------------------------------

const columnsRef = ref<HTMLElement | null>(null)

/** Minimum width of the parameters column (n8n parity). */
const PARAMETERS_MIN_WIDTH = 260
/** Resize grid — dragged widths snap to this step (n8n parity). */
const RESIZE_GRID = 8
/** Smallest share a data column may keep while it is dragged. */
const DATA_MIN_PERCENT = 8

const inputWidth = ref(26)
const parametersWidth = ref(48)

const outputWidth = computed(() =>
  Math.max(0, 100 - inputWidth.value - parametersWidth.value)
)

const columnStyle = (percent: number) => ({ width: `${percent}%` })

const clamp = (value: number, min: number, max: number) =>
  Math.min(Math.max(value, min), max)

const snap = (value: number) => Math.round(value / RESIZE_GRID) * RESIZE_GRID

/**
 * Starts a column resize. The pointer is tracked on the window so
 * the drag survives the pointer leaving the divider.
 */
const startResize = (event: PointerEvent, column: 'input' | 'parameters') => {
  const container = columnsRef.value
  if (!container) return

  event.preventDefault()

  const total = container.getBoundingClientRect().width
  if (total <= 0) return

  const startX = event.clientX
  const startInput = inputWidth.value
  const startParameters = parametersWidth.value
  const minParametersPercent = (PARAMETERS_MIN_WIDTH / total) * 100

  const onMove = (moveEvent: PointerEvent) => {
    const delta = moveEvent.clientX - startX

    if (column === 'input') {
      const pixels = snap((startInput / 100) * total + delta)
      inputWidth.value = clamp(
        (pixels / total) * 100,
        DATA_MIN_PERCENT,
        100 - parametersWidth.value - minParametersPercent
      )
      return
    }

    const pixels = snap((startParameters / 100) * total + delta)
    parametersWidth.value = clamp(
      (pixels / total) * 100,
      minParametersPercent,
      100 - inputWidth.value - DATA_MIN_PERCENT
    )
  }

  const onUp = () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  }

  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

// --------------------------------------------------------------------
// Actions
// --------------------------------------------------------------------

/**
 * Closes the NDV. The canvas selection is left alone — n8n keeps the
 * node selected after the panel closes.
 */
const close = () => {
  workflowEditorStore.closeNodeDetails()
  emit('close')
}

const deleteNode = () => {
  if (node.value && confirm('Are you sure you want to delete this node?')) {
    workflowEditorStore.removeNode(node.value.id)
  }
}

const updateName = () => {
  if (node.value && localName.value.trim()) {
    workflowEditorStore.updateNode(node.value.id, { name: localName.value.trim() })
  }
}

/** Current value of a property (nested paths are supported). */
const parameterValue = (property: NodeProperty): unknown =>
  getParameterValue(localParameters.value, property.name)

/** Write a property value back to the node (nested paths are supported). */
const updateParameter = (name: string, value: unknown) => {
  if (!node.value) return
  // An emptied field is removed from the parameters instead of being
  // stored as `undefined`.
  const newParams =
    value === undefined
      ? deleteParameterValue(localParameters.value, name)
      : setParameterValue(localParameters.value, name, value)
  localParameters.value = newParams
  workflowEditorStore.updateNode(node.value.id, { parameters: newParams })
}

/**
 * A hidden property must not be submitted — drop its value when the
 * visibility rule flips to hidden.
 */
const clearParameter = (property: NodeProperty) => {
  if (!node.value) return
  if (getParameterValue(localParameters.value, property.name) === undefined) return
  const newParams = deleteParameterValue(localParameters.value, property.name)
  localParameters.value = newParams
  workflowEditorStore.updateNode(node.value.id, { parameters: newParams })
}

// Credentials — the slots come from the node-type metadata
// (`credentials[]`: type + displayName + required).
const credentialSlots = computed(() => resolveNodeCredentialTypes(nodeType.value))

const credentialsAvailable = computed(() => credentialsStore.credentials)

const setCredential = (credType: string, credId: string) => {
  if (!node.value) return
  if (!credId) {
    workflowEditorStore.bindCredential(node.value.id, credType, null)
    const next: LocalCredentialsMap = { ...localCredentials.value }
    delete next[credType]
    localCredentials.value = next
    return
  }
  const found = credentialsAvailable.value.find((c) => c.id === credId)
  if (!found) return
  const next: LocalCredentialsMap = {
    ...localCredentials.value,
    [credType]: { id: found.id, name: found.name },
  }
  localCredentials.value = next
  workflowEditorStore.bindCredential(node.value.id, credType, {
    id: found.id,
    name: found.name,
  })
}

const goToCreateCredential = (credType: string) => {
  router.push({ path: '/credentials', query: { newType: credType } })
}

// --------------------------------------------------------------------
// Execution data — Input / Output columns
//
// The columns show the data of the workflow's *last* execution. The
// editor loads it when the panel opens; when the workflow has never
// run — or when the node did not take part in that run — the column
// shows an empty state instead of invented data.
// --------------------------------------------------------------------

/** The last execution of the open workflow. */
const lastExecution = ref<WorkflowExecution | null>(null)
const loadingExecution = ref(false)

const loadLastExecution = async () => {
  const workflowId = workflowStore.currentWorkflow?.id
  if (!workflowId) {
    lastExecution.value = null
    return
  }

  loadingExecution.value = true
  try {
    const response = await getExecutions({ workflowId, limit: 1 })
    const latest = response.data?.[0]
    if (!latest) {
      lastExecution.value = null
      return
    }
    // The list carries the run metadata; the per-node payloads live
    // on the execution detail.
    lastExecution.value = latest.nodeData ? latest : await getExecution(latest.id)
  } catch {
    // "No execution data" is not an error — the columns fall back to
    // their empty state.
    lastExecution.value = null
  } finally {
    loadingExecution.value = false
  }
}

// A different workflow invalidates the loaded data.
watch(
  () => workflowStore.currentWorkflow?.id,
  () => {
    void loadLastExecution()
  }
)

// The panel usually mounts *after* the workflow was loaded, so the
// watcher above never fires for the workflow it is showing. Load the
// last run on mount, otherwise the Input / Output columns would always
// render their empty state.
onMounted(() => {
  void loadLastExecution()
})

// A run that finishes while the panel is open refreshes the columns.
watch(
  () => executionStore.runningExecutions.size,
  (size, previous) => {
    if (previous > 0 && size === 0) {
      void loadLastExecution()
    }
  }
)

/** The execution the Input / Output columns are rendered from. */
const latestExecution = computed<WorkflowExecution | null>(() => {
  const workflowId = workflowStore.currentWorkflow?.id
  if (!workflowId) return null

  const loaded = lastExecution.value
  if (loaded && loaded.workflowId === workflowId) return loaded

  // The execution the app already has open (e.g. the user came from
  // the execution detail page).
  const current = executionStore.currentExecution
  if (current && current.workflowId === workflowId) return current

  return null
})

/** The data this node produced during the last execution. */
const nodeOutput = computed<DataItem[] | undefined>(() => {
  const name = node.value?.name
  if (!name || !latestExecution.value) return undefined
  return latestExecution.value.nodeData?.[name]
})

/**
 * The data that flowed INTO this node. For a node with upstream
 * connections that is the output of those upstream nodes; for a
 * trigger (no upstream) it is its own payload.
 */
const nodeInput = computed<DataItem[] | undefined>(() => {
  const workflow = workflowStore.currentWorkflow
  const execution = latestExecution.value
  const current = node.value
  if (!workflow || !execution || !current) return undefined

  const nodeData = execution.nodeData ?? {}
  const incoming: DataItem[] = []
  let hasUpstream = false

  // The workflow keys its connections by the *source* node name.
  for (const [sourceName, connections] of Object.entries(workflow.connections ?? {})) {
    for (const outputs of connections.main ?? []) {
      for (const connection of outputs) {
        if (connection.node !== current.name) continue
        hasUpstream = true
        const data = nodeData[sourceName]
        if (data && data.length > 0) {
          incoming.push(...data)
        }
      }
    }
  }

  // A trigger has no upstream — its own payload is its input.
  if (!hasUpstream) {
    const own = nodeData[current.name]
    return own && own.length > 0 ? own : undefined
  }

  return incoming.length > 0 ? incoming : undefined
})

const formatItems = (items: DataItem[] | undefined): string => {
  if (!items || !items.length) return ''
  return JSON.stringify(items.map((item) => item.json), null, 2)
}

const inputText = computed(() => formatItems(nodeInput.value))
const outputText = computed(() => formatItems(nodeOutput.value))

/**
 * The completion data of the expression editor.
 *
 * The fields of the current item (`$json.<field>`) and the fields every
 * other node produced (`$node["Name"].json.<field>`) come from the
 * last execution — the same data the Input / Output columns show.
 */
const expressionContext = computed(() =>
  buildExpressionFieldSources(
    nodeInput.value?.[0]?.json,
    Object.fromEntries(
      Object.entries(latestExecution.value?.nodeData ?? {}).map(([name, items]) => [
        name,
        items?.[0]?.json,
      ]),
    ),
  ),
)

provide(EXPRESSION_CONTEXT_KEY, expressionContext)

/**
 * The empty state of a data column. It never invents data: it either
 * says that the workflow has not run yet, or that the node did not
 * take part in the last run.
 */
const emptyState = (kind: 'input' | 'output') => {
  if (loadingExecution.value) {
    return {
      title: 'Loading execution data…',
      body: 'Fetching the last execution of this workflow.',
    }
  }

  if (!latestExecution.value) {
    return {
      title: kind === 'input' ? 'No input data' : 'No output data',
      body:
        kind === 'input'
          ? 'This workflow has no execution yet. Run it to see the data this node received.'
          : 'This workflow has no execution yet. Run it to see the data this node produced.',
    }
  }

  return {
    title: kind === 'input' ? 'No input data' : 'No output data',
    body:
      kind === 'input'
        ? 'This node did not receive any data in the last execution.'
        : 'This node did not produce any data in the last execution.',
  }
}

const inputEmpty = computed(() => emptyState('input'))
const outputEmpty = computed(() => emptyState('output'))

// --------------------------------------------------------------------
// Keyboard
// --------------------------------------------------------------------

/**
 * Escape closes the NDV.
 *
 * The handler runs in the capture phase and stops the propagation so
 * the canvas' "deselect" shortcut (Escape) does not fire: closing the
 * NDV must leave the node selected (n8n parity).
 *
 * The autocomplete of the expression editor owns Escape while its
 * suggestion list is open — the list closes first and the panel stays
 * open (n8n parity). Because the handler runs in the capture phase it
 * cannot be stopped from the field, so the open list is detected
 * here.
 */
const onKeyDown = (event: KeyboardEvent) => {
  if (event.key !== 'Escape') return
  if (isSuggestionListOpen(event.target)) return
  event.stopPropagation()
  event.preventDefault()
  close()
}

// The dialog takes the focus so the keyboard belongs to the NDV while
// it is open (n8n focuses the panel as well).
const dialogRef = ref<HTMLElement | null>(null)

onMounted(() => {
  window.addEventListener('keydown', onKeyDown, true)
  void nextTick(() => dialogRef.value?.focus())
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown, true)
})
</script>

<template>
  <!--
    NDV overlay — n8n parity: a dimmed backdrop with a large dialog
    on top of it. The backdrop is clickable and closes the panel.
  -->
  <div
    class="ndv-backdrop"
    data-test-id="ndv-backdrop"
    @click.self="close"
  >
    <div
      ref="dialogRef"
      class="ndv-dialog"
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      :aria-label="node ? `Node details: ${node.name}` : 'Node details'"
      data-test-id="ndv"
    >
      <!-- Header — 42px, n8n parity -->
      <header class="ndv-header">
        <div class="ndv-header__identity">
          <input
            v-model="localName"
            @blur="updateName"
            @keyup.enter="updateName"
            class="ndv-header__name"
            aria-label="Node name"
          />
          <span class="ndv-header__type">
            {{ category }} · {{ nodeType?.displayName || node?.type }}
          </span>
        </div>

        <div class="ndv-header__actions">
          <button
            type="button"
            @click="deleteNode"
            class="ndv-btn-danger"
          >
            <TrashIcon class="w-4 h-4" />
            <span>Delete</span>
          </button>
          <button
            type="button"
            @click="close"
            class="ndv-btn-primary"
          >
            <span>Done</span>
          </button>
          <button
            type="button"
            @click="close"
            class="ndv-btn-ghost"
            title="Close"
            aria-label="Close"
            data-test-id="ndv-close-button"
          >
            <XMarkIcon class="w-4 h-4" />
          </button>
        </div>
      </header>

      <!-- Three resizable columns: Input | Parameters | Output -->
      <div ref="columnsRef" class="ndv-columns">
        <!-- Input -->
        <section
          class="ndv-column ndv-column--input"
          :style="columnStyle(inputWidth)"
        >
          <h4 class="ndv-column__title">Input</h4>
          <pre v-if="inputText" class="ndv-data">{{ inputText }}</pre>
          <div v-else class="ndv-empty">
            <p class="ndv-empty__title">{{ inputEmpty.title }}</p>
            <p class="ndv-empty__body">{{ inputEmpty.body }}</p>
          </div>
        </section>

        <div
          class="ndv-resizer"
          role="separator"
          aria-orientation="vertical"
          @pointerdown="startResize($event, 'input')"
        />

        <!-- Parameters -->
        <section
          class="ndv-column ndv-column--parameters"
          :style="columnStyle(parametersWidth)"
        >
          <h4 class="ndv-column__title">Parameters</h4>

          <!-- Node Description -->
          <div v-if="nodeType?.description" class="ndv-description">
            {{ nodeType.description }}
          </div>

          <!-- Credentials Section -->
          <div v-if="credentialSlots.length > 0" class="ndv-section">
            <div class="ndv-section__title">
              <KeyIcon class="w-4 h-4" />
              <h5>Credentials</h5>
            </div>
            <div
              v-for="slot in credentialSlots"
              :key="slot.type"
              class="ndv-field"
            >
              <label class="ndv-field__label">
                {{ slot.displayName }}
                <span class="ndv-field__hint">({{ slot.type }})</span>
                <span v-if="slot.required" class="ndv-field__required">*</span>
              </label>
              <select
                :value="localCredentials[slot.type]?.id ?? ''"
                @change="setCredential(slot.type, ($event.target as HTMLSelectElement).value)"
                class="input h-10 px-3"
              >
                <option value="">— None —</option>
                <option
                  v-for="c in credentialsAvailable.filter((c: Credential) => c.type === slot.type)"
                  :key="c.id"
                  :value="c.id"
                >
                  {{ c.name }}
                </option>
              </select>
              <button
                type="button"
                @click="goToCreateCredential(slot.type)"
                class="ndv-field__action"
              >
                <PlusIcon class="w-3 h-3" />
                Create new {{ slot.displayName }}
              </button>
            </div>
          </div>

          <!-- Parameter fields -->
          <div class="ndv-section">
            <!-- Metadata-driven parameters -->
            <div v-if="properties.length" class="ndv-fields">
              <div
                v-for="prop in properties"
                :key="prop.name"
                class="ndv-field"
              >
                <DisplayOptionsGuard
                  :property="prop"
                  :parameters="localParameters"
                  :properties="properties"
                  @hidden="clearParameter(prop)"
                >
                  <label class="ndv-field__label">
                    {{ prop.displayName }}
                    <span v-if="prop.required" class="ndv-field__required">*</span>
                  </label>

                  <ParameterField
                    :property="prop"
                    :model-value="parameterValue(prop)"
                    :parameters="localParameters"
                    :node-type-name="node?.type"
                    @update:model-value="updateParameter(prop.name, $event)"
                  />

                  <p
                    v-if="prop.description && prop.type !== 'boolean'"
                    class="ndv-field__hint"
                  >
                    {{ prop.description }}
                  </p>
                </DisplayOptionsGuard>
              </div>
            </div>

            <!-- No properties message -->
            <div v-else class="ndv-empty__body">
              <template v-if="nodeTypesStore.loading">
                Loading node metadata…
              </template>
              <template v-else>
                No configurable parameters for this node.
              </template>
            </div>

            <!-- Raw Parameters Editor -->
            <div class="ndv-advanced">
              <details class="group">
                <summary class="ndv-advanced__summary">
                  Advanced: Raw Parameters
                </summary>
                <div class="mt-2">
                  <textarea
                    :value="JSON.stringify(localParameters, null, 2)"
                    @input="localParameters = JSON.parse(($event.target as HTMLTextAreaElement).value || '{}')"
                    @blur="node && workflowEditorStore.updateNode(node.id, { parameters: localParameters })"
                    rows="6"
                    class="input font-mono text-xs resize-y"
                  />
                </div>
              </details>
            </div>
          </div>
        </section>

        <div
          class="ndv-resizer"
          role="separator"
          aria-orientation="vertical"
          @pointerdown="startResize($event, 'parameters')"
        />

        <!-- Output -->
        <section
          class="ndv-column ndv-column--output"
          :style="columnStyle(outputWidth)"
        >
          <h4 class="ndv-column__title">Output</h4>
          <pre v-if="outputText" class="ndv-data">{{ outputText }}</pre>
          <div v-else class="ndv-empty">
            <p class="ndv-empty__title">{{ outputEmpty.title }}</p>
            <p class="ndv-empty__body">{{ outputEmpty.body }}</p>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
/*
 * The NDV's primary action. The brand colour comes from the token
 * layer (`--background--brand`); the fallback keeps it visible before
 * the layer lands.
 */
.ndv-btn-primary {
  background: var(--background--brand, oklch(78.07% 0.136 5.02));
  color: #ffffff;
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.ndv-btn-primary:hover {
  background: var(--background--brand--hover, oklch(68.08% 0.148 5.02));
}

.ndv-btn-primary:active {
  background: var(--background--brand--active, oklch(59.09% 0.1611 5.09));
}

.ndv-btn-danger {
  color: var(--text-color--danger, oklch(55.55% 0.2373 25.44));
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.ndv-btn-danger:hover {
  background: var(--background--danger, oklch(97.05% 0.0129 17.38));
}

.ndv-btn-ghost {
  color: var(--text-color--subtler, oklch(55.55% 0 89.88));
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.ndv-btn-ghost:hover {
  background: var(--background--hover, oklch(0% 0 0 / 0.05));
}

/*
 * The overlay: a dimmed backdrop with the dialog floating on top of
 * it (n8n parity). The inset keeps a strip of backdrop reachable, so
 * a click outside the dialog closes the panel.
 */
.ndv-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  background: var(--dialog--overlay, oklch(20.46% 0 89.88 / 0.5));
}

.ndv-dialog {
  position: absolute;
  inset: 24px;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-radius: var(--radius--lg, 8px);
  border: 1px solid var(--border-color, oklch(0% 0 0 / 0.1));
  background: var(--background--surface, #ffffff);
  box-shadow: var(--shadow--xl, 0 20px 25px -5px oklch(0% 0 0 / 0.1));
  overflow: hidden;
  outline: none;
}

:global(.dark) .ndv-dialog {
  background: var(--background--surface, oklch(26.86% 0 89.88));
}

/* Header — 42px, n8n parity */
.ndv-header {
  height: 42px;
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 12px;
  border-bottom: 1px solid var(--border-color, oklch(0% 0 0 / 0.1));
}

.ndv-header__identity {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1 1 auto;
}

.ndv-header__name {
  width: 16rem;
  padding: 4px 8px;
  font-size: 14px;
  font-weight: 600;
  border-radius: var(--radius--md, 6px);
  border: 1px solid transparent;
  background: transparent;
  color: var(--text-color, oklch(20.46% 0 89.88));
}

.ndv-header__name:hover {
  border-color: var(--border-color, oklch(0% 0 0 / 0.1));
}

.ndv-header__name:focus {
  outline: none;
  border-color: var(--background--brand, oklch(78.07% 0.136 5.02));
}

.ndv-header__type {
  font-size: 12px;
  color: var(--text-color--subtler, oklch(55.55% 0 89.88));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ndv-header__actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.ndv-header__actions button {
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 12px;
  font-size: 14px;
  font-weight: 500;
  border-radius: var(--radius--md, 6px);
  border: none;
  cursor: pointer;
}

.ndv-header__actions .ndv-btn-ghost {
  width: 32px;
  padding: 0;
}

/* The three columns */
.ndv-columns {
  flex: 1 1 auto;
  display: flex;
  min-height: 0;
}

.ndv-column {
  min-width: 0;
  overflow-y: auto;
  padding: 16px;
  background: var(--background--surface, #ffffff);
  color: var(--text-color, oklch(20.46% 0 89.88));
}

:global(.dark) .ndv-column {
  background: var(--background--surface, oklch(26.86% 0 89.88));
}

/* 1px dividers between the columns */
.ndv-column--parameters {
  border-left: 1px solid var(--border-color--subtle, oklch(0% 0 0 / 0.05));
  border-right: 1px solid var(--border-color--subtle, oklch(0% 0 0 / 0.05));
}

.ndv-column__title {
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.025em;
  text-transform: uppercase;
  color: var(--text-color--subtler, oklch(55.55% 0 89.88));
  margin-bottom: 12px;
}

.ndv-description {
  font-size: 14px;
  color: var(--text-color--subtle, oklch(38.67% 0 89.88));
  margin-bottom: 16px;
}

.ndv-section + .ndv-section {
  margin-top: 24px;
}

.ndv-section__title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  color: var(--text-color--subtler, oklch(55.55% 0 89.88));
}

.ndv-section__title h5 {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-color, oklch(20.46% 0 89.88));
}

.ndv-fields {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.ndv-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ndv-field__label {
  display: block;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-color--subtle, oklch(38.67% 0 89.88));
}

.ndv-field__hint {
  margin-top: 0;
  font-size: 12px;
  color: var(--text-color--subtler, oklch(55.55% 0 89.88));
}

.ndv-field__required {
  color: var(--text-color--danger, oklch(55.55% 0.2373 25.44));
  margin-left: 4px;
}

.ndv-field__action {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  align-self: flex-start;
  font-size: 12px;
  color: var(--background--brand--active, oklch(59.09% 0.1611 5.09));
}

.ndv-field__action:hover {
  text-decoration: underline;
}

.ndv-advanced {
  padding-top: 16px;
  margin-top: 16px;
  border-top: 1px solid var(--border-color--subtle, oklch(0% 0 0 / 0.05));
}

.ndv-advanced__summary {
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  color: var(--text-color--subtle, oklch(38.67% 0 89.88));
}

.ndv-advanced__summary:hover {
  color: var(--text-color, oklch(20.46% 0 89.88));
}

.ndv-data {
  margin: 0;
  padding: 12px;
  border-radius: var(--radius--md, 6px);
  border: 1px solid var(--border-color--subtle, oklch(0% 0 0 / 0.05));
  background: var(--background--subtle, oklch(97.02% 0 89.88));
  font-family: 'Geist Mono', ui-monospace, Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--text-color, oklch(20.46% 0 89.88));
}

.ndv-empty {
  padding: 24px 12px;
  text-align: center;
}

.ndv-empty__title {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-color, oklch(20.46% 0 89.88));
}

.ndv-empty__body {
  margin-top: 6px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--text-color--subtler, oklch(55.55% 0 89.88));
}

/* The divider between the columns — 8px resize grid. */
.ndv-resizer {
  flex: 0 0 4px;
  cursor: col-resize;
  background: transparent;
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.ndv-resizer:hover,
.ndv-resizer:active {
  background: var(--background--brand, oklch(78.07% 0.136 5.02));
}

@media (prefers-reduced-motion: reduce) {
  .ndv-resizer,
  .ndv-btn-primary,
  .ndv-btn-danger,
  .ndv-btn-ghost {
    transition-duration: 1ms;
  }
}
</style>
