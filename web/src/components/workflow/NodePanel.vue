<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
import { getNodeCategory } from '@/types/node'
import type { Credential } from '@/types/api'
import type { DataItem } from '@/types'
import type { NodeProperty } from '@/types/node'
import { resolveNodeCredentialTypes } from '@/lib/credentialSchema'
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
 * n8n parity: a full-screen modal with three resizable columns —
 * Input | Settings | Output. The settings column holds everything
 * that was in the old right-hand drawer; the input / output columns
 * show the node's last execution data when the editor knows about
 * one.
 *
 * Everything rendered in the settings column comes from the node-type
 * metadata: the property list, the visibility rules
 * (`displayOptions`) and the credential slots. There is no
 * hard-coded per-node field list.
 */
const workflowEditorStore = useWorkflowEditorStore()
const credentialsStore = useCredentialsStore()
const workflowStore = useWorkflowStore()
const executionStore = useExecutionStore()
const nodeTypesStore = useNodeTypesStore()
const router = useRouter()

const node = computed(() => workflowEditorStore.selectedNode)

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
      // The WorkflowNode.credentials map values are NodeCredential
      // objects with an optional id; coerce to BoundCredential which
      // requires id+name. n8n guarantees id+name are always present.
      const source = (newNode.credentials ?? {}) as Record<
        string,
        { id?: string; name: string }
      >
      const next: LocalCredentialsMap = {}
      for (const [k, v] of Object.entries(source)) {
        if (v.id) next[k] = { id: v.id, name: v.name }
      }
      localCredentials.value = next
    }
  },
  { immediate: true },
)

// --------------------------------------------------------------------
// Column geometry
//
// Three columns: Input | Settings | Output. The widths are stored as
// percentages of the modal width; the settings column never drops
// below 260px, and every drag lands on the 8px resize grid.
// --------------------------------------------------------------------

const columnsRef = ref<HTMLElement | null>(null)

/** Minimum width of the settings column (n8n parity). */
const SETTINGS_MIN_WIDTH = 260
/** Resize grid — dragged widths snap to this step (n8n parity). */
const RESIZE_GRID = 8

const inputWidth = ref(26)
const settingsWidth = ref(48)

const outputWidth = computed(() => Math.max(0, 100 - inputWidth.value - settingsWidth.value))

const columnStyle = (percent: number) => ({ width: `${percent}%` })

const clamp = (value: number, min: number, max: number) =>
  Math.min(Math.max(value, min), max)

const snap = (value: number) => Math.round(value / RESIZE_GRID) * RESIZE_GRID

/**
 * Starts a column resize. The pointer is tracked on the window so
 * the drag survives the pointer leaving the divider.
 */
const startResize = (event: PointerEvent, column: 'input' | 'settings') => {
  const container = columnsRef.value
  if (!container) return

  event.preventDefault()

  const total = container.getBoundingClientRect().width
  if (total <= 0) return

  const startX = event.clientX
  const startInput = inputWidth.value
  const startSettings = settingsWidth.value
  const minSettingsPercent = (SETTINGS_MIN_WIDTH / total) * 100

  const onMove = (moveEvent: PointerEvent) => {
    const delta = moveEvent.clientX - startX

    if (column === 'input') {
      const pixels = snap((startInput / 100) * total + delta)
      inputWidth.value = clamp(
        (pixels / total) * 100,
        0,
        100 - settingsWidth.value - minSettingsPercent
      )
      return
    }

    const pixels = snap((startSettings / 100) * total + delta)
    settingsWidth.value = clamp(
      (pixels / total) * 100,
      minSettingsPercent,
      100 - inputWidth.value
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

const close = () => {
  workflowEditorStore.clearSelection()
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
// The editor reads the executions the app already knows about (the
// execution store). When no execution has been loaded for this
// workflow, the columns show an empty state instead of invented
// data.
// --------------------------------------------------------------------

/** The most recent known execution of the open workflow. */
const latestExecution = computed(() => {
  const workflowId = workflowStore.currentWorkflow?.id
  if (!workflowId) return null

  const candidates: typeof executionStore.executions = [
    ...executionStore.executions,
  ]
  if (executionStore.currentExecution) {
    candidates.push(executionStore.currentExecution)
  }

  const matching = candidates.filter((execution) => execution.workflowId === workflowId)
  if (!matching.length) return null

  return (
    [...matching].sort(
      (a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
    )[0] ?? null
  )
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

// --------------------------------------------------------------------
// Keyboard
// --------------------------------------------------------------------

const onKeyDown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    event.stopPropagation()
    close()
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown)
})
</script>

<template>
  <!--
    Full-screen NDV. The panel covers the viewport (n8n parity); the
    backdrop still reacts to a click that lands outside the panel.
  -->
  <div
    class="fixed inset-0 z-50 bg-neutral-950/40"
    @click.self="close"
  >
    <div class="absolute inset-0 flex flex-col bg-surface">
      <!-- Header — 42px, n8n parity -->
      <header class="h-[42px] shrink-0 flex items-center gap-3 px-3 border-b border-default">
        <div class="flex items-center gap-2 min-w-0 flex-1">
          <input
            v-model="localName"
            @blur="updateName"
            @keyup.enter="updateName"
            class="w-64 px-2 py-1 text-sm font-semibold rounded-md bg-transparent border border-transparent hover:border-default focus:outline-none focus:border-brand-500 text-primary"
            aria-label="Node name"
          />
          <span class="text-xs text-tertiary truncate">
            {{ category }} · {{ nodeType?.displayName || node?.type }}
          </span>
        </div>

        <div class="flex items-center gap-1">
          <button
            @click="deleteNode"
            class="h-8 px-3 inline-flex items-center gap-1.5 text-sm font-medium rounded-md text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-500/10"
          >
            <TrashIcon class="w-4 h-4" />
            <span>Delete</span>
          </button>
          <button
            @click="close"
            class="ndv-btn-primary h-8 px-3 inline-flex items-center gap-1.5 text-sm font-medium rounded-md text-white"
          >
            <span>Done</span>
          </button>
          <button
            @click="close"
            class="h-8 w-8 inline-flex items-center justify-center rounded-md text-tertiary hover:bg-hover"
            title="Close"
            aria-label="Close"
          >
            <XMarkIcon class="w-4 h-4" />
          </button>
        </div>
      </header>

      <!-- Three resizable columns: Input | Settings | Output -->
      <div ref="columnsRef" class="flex-1 flex min-h-0">
        <!-- Input -->
        <section
          class="ndv-column ndv-column--input overflow-y-auto"
          :style="columnStyle(inputWidth)"
        >
          <h4 class="ndv-column__title">Input</h4>
          <pre v-if="inputText" class="ndv-data">{{ inputText }}</pre>
          <div v-else class="ndv-empty">
            <p class="ndv-empty__title">No input data</p>
            <p class="ndv-empty__body">
              Run the workflow to see the data this node received. The input
              appears here after the first execution.
            </p>
          </div>
        </section>

        <div
          class="ndv-resizer"
          role="separator"
          aria-orientation="vertical"
          @pointerdown="startResize($event, 'input')"
        />

        <!-- Settings -->
        <section
          class="ndv-column ndv-column--settings overflow-y-auto"
          :style="columnStyle(settingsWidth)"
        >
          <!-- Node Description -->
          <div v-if="nodeType?.description" class="text-sm text-secondary mb-4">
            {{ nodeType.description }}
          </div>

          <!-- Credentials Section -->
          <div v-if="credentialSlots.length > 0" class="space-y-3 mb-6">
            <div class="flex items-center gap-2">
              <KeyIcon class="w-4 h-4 text-tertiary" />
              <h4 class="text-sm font-semibold text-primary">
                Credentials
              </h4>
            </div>
            <div
              v-for="slot in credentialSlots"
              :key="slot.type"
              class="space-y-1"
            >
              <label class="block text-xs font-medium text-secondary">
                {{ slot.displayName }}
                <span class="text-tertiary ml-1">({{ slot.type }})</span>
                <span v-if="slot.required" class="text-red-500">*</span>
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
                class="text-xs text-brand-600 dark:text-brand-400 hover:underline flex items-center gap-1"
              >
                <PlusIcon class="w-3 h-3" />
                Create new {{ slot.displayName }}
              </button>
            </div>
          </div>

          <!-- Parameters Section -->
          <div class="space-y-4">
            <h4 class="text-sm font-semibold text-primary">Parameters</h4>

            <!-- Metadata-driven parameters -->
            <div v-if="properties.length" class="space-y-4">
              <div
                v-for="prop in properties"
                :key="prop.name"
                class="space-y-1"
              >
                <DisplayOptionsGuard
                  :property="prop"
                  :parameters="localParameters"
                  :properties="properties"
                  @hidden="clearParameter(prop)"
                >
                  <label class="block text-xs font-medium text-secondary">
                    {{ prop.displayName }}
                    <span v-if="prop.required" class="text-red-500">*</span>
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
                    class="text-xs text-tertiary"
                  >
                    {{ prop.description }}
                  </p>
                </DisplayOptionsGuard>
              </div>
            </div>

            <!-- No properties message -->
            <div v-else class="text-sm text-tertiary">
              <template v-if="nodeTypesStore.loading">
                Loading node metadata…
              </template>
              <template v-else>
                No configurable parameters for this node.
              </template>
            </div>

            <!-- Raw Parameters Editor -->
            <div class="pt-4 border-t border-default">
              <details class="group">
                <summary class="cursor-pointer text-sm font-medium text-secondary hover:text-primary">
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
          @pointerdown="startResize($event, 'settings')"
        />

        <!-- Output -->
        <section
          class="ndv-column ndv-column--output overflow-y-auto"
          :style="columnStyle(outputWidth)"
        >
          <h4 class="ndv-column__title">Output</h4>
          <pre v-if="outputText" class="ndv-data">{{ outputText }}</pre>
          <div v-else class="ndv-empty">
            <p class="ndv-empty__title">No output data</p>
            <p class="ndv-empty__body">
              Run the workflow to see the data this node produced. The output
              appears here after the first execution.
            </p>
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
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.ndv-btn-primary:hover {
  background: var(--background--brand--hover, oklch(68.08% 0.148 5.02));
}

.ndv-btn-primary:active {
  background: var(--background--brand--active, oklch(59.09% 0.1611 5.09));
}

.ndv-column {
  min-width: 0;
  padding: 16px;
  background: var(--background--surface, #ffffff);
  color: var(--text-color, oklch(20.46% 0 89.88));
}

:global(.dark) .ndv-column {
  background: var(--background--surface, oklch(26.86% 0 89.88));
}

.ndv-column--settings {
  border-left: 1px solid var(--border-color--subtle, oklch(0% 0 0 / 0.05));
  border-right: 1px solid var(--border-color--subtle, oklch(0% 0 0 / 0.05));
}

.ndv-column__title {
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.025em;
  text-transform: uppercase;
  color: var(--text-color--subtle, oklch(55.55% 0 89.88));
  margin-bottom: 12px;
}

.ndv-data {
  margin: 0;
  padding: 12px;
  border-radius: 8px;
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
  .ndv-resizer {
    transition-duration: 1ms;
  }
}
</style>
