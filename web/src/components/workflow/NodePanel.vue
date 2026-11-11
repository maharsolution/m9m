<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
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
} from '@/stores'
import { useNodeTypesStore } from '@/stores/nodeTypes'
import { getNodeCategory } from '@/types/node'
import type { Credential } from '@/types/api'
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
 * Node detail panel.
 *
 * Everything rendered here comes from the node-type metadata: the
 * property list, the visibility rules (`displayOptions`) and the
 * credential slots. There is no hard-coded per-node field list.
 */
const workflowEditorStore = useWorkflowEditorStore()
const credentialsStore = useCredentialsStore()
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

const getCategoryColor = () => {
  switch (category.value) {
    case 'trigger':
      return 'border-green-500'
    case 'action':
      return 'border-indigo-500'
    case 'transform':
      return 'border-amber-500'
    case 'flow':
      return 'border-purple-500'
    default:
      return 'border-slate-500'
  }
}
</script>

<template>
  <div class="h-full bg-white dark:bg-slate-800 border-l border-slate-200 dark:border-slate-700 flex flex-col">
    <!-- Header -->
    <div :class="['p-4 border-b border-slate-200 dark:border-slate-700 border-l-4', getCategoryColor()]">
      <div class="flex items-center justify-between">
        <div class="flex-1 min-w-0">
          <input
            v-model="localName"
            @blur="updateName"
            @keyup.enter="updateName"
            class="w-full text-lg font-semibold bg-transparent border-none text-slate-900 dark:text-white focus:outline-none focus:ring-0"
          />
          <p class="text-sm text-slate-500 dark:text-slate-400 truncate">
            {{ nodeType?.displayName || node?.type }}
          </p>
        </div>
        <button
          @click="close"
          class="p-1 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700"
        >
          <XMarkIcon class="w-5 h-5" />
        </button>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-4 space-y-4">
      <!-- Node Description -->
      <div v-if="nodeType?.description" class="text-sm text-slate-600 dark:text-slate-300">
        {{ nodeType.description }}
      </div>

      <!-- Credentials Section -->
      <div v-if="credentialSlots.length > 0" class="space-y-3">
        <div class="flex items-center gap-2">
          <KeyIcon class="w-4 h-4 text-slate-500 dark:text-slate-400" />
          <h4 class="font-medium text-slate-900 dark:text-white">
            Credentials
          </h4>
        </div>
        <div
          v-for="slot in credentialSlots"
          :key="slot.type"
          class="space-y-1"
        >
          <label class="block text-sm font-medium text-slate-700 dark:text-slate-300">
            {{ slot.displayName }}
            <span class="text-xs text-slate-400 ml-1">({{ slot.type }})</span>
            <span v-if="slot.required" class="text-red-500">*</span>
          </label>
          <select
            :value="localCredentials[slot.type]?.id ?? ''"
            @change="setCredential(slot.type, ($event.target as HTMLSelectElement).value)"
            class="input"
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
            class="text-xs text-primary-600 dark:text-primary-400 hover:underline flex items-center gap-1"
          >
            <PlusIcon class="w-3 h-3" />
            Create new {{ slot.displayName }}
          </button>
        </div>
      </div>

      <!-- Parameters Section -->
      <div class="space-y-4">
        <h4 class="font-medium text-slate-900 dark:text-white">Parameters</h4>

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
              <label class="block text-sm font-medium text-slate-700 dark:text-slate-300">
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
                class="text-xs text-slate-500 dark:text-slate-400"
              >
                {{ prop.description }}
              </p>
            </DisplayOptionsGuard>
          </div>
        </div>

        <!-- No properties message -->
        <div v-else class="text-sm text-slate-500 dark:text-slate-400">
          <template v-if="nodeTypesStore.loading">
            Loading node metadata…
          </template>
          <template v-else>
            No configurable parameters for this node.
          </template>
        </div>

        <!-- Raw Parameters Editor -->
        <div class="pt-4 border-t border-slate-200 dark:border-slate-700">
          <details class="group">
            <summary class="cursor-pointer text-sm font-medium text-slate-700 dark:text-slate-300 hover:text-slate-900 hover:dark:text-white">
              Advanced: Raw Parameters
            </summary>
            <div class="mt-2">
              <textarea
                :value="JSON.stringify(localParameters, null, 2)"
                @input="localParameters = JSON.parse(($event.target as HTMLTextAreaElement).value || '{}')"
                @blur="node && workflowEditorStore.updateNode(node.id, { parameters: localParameters })"
                rows="6"
                class="input font-mono text-xs"
              />
            </div>
          </details>
        </div>
      </div>
    </div>

    <!-- Footer Actions -->
    <div class="p-4 border-t border-slate-200 dark:border-slate-700 flex items-center gap-2">
      <button
        @click="deleteNode"
        class="btn-ghost text-red-600 dark:text-red-400 flex items-center gap-2"
      >
        <TrashIcon class="w-4 h-4" />
        <span>Delete</span>
      </button>
    </div>
  </div>
</template>
