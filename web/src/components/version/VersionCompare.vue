<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useVersionStore } from '@/stores'
import { tagErrorMessage } from '@/api/tags'
import type { WorkflowVersion, VersionComparison } from '@/types/version'
import {
  XMarkIcon,
  ArrowsRightLeftIcon,
  PlusCircleIcon,
  MinusCircleIcon,
  PencilSquareIcon,
  Cog6ToothIcon,
  ArrowPathRoundedSquareIcon,
} from '@heroicons/vue/24/outline'

const props = defineProps<{
  workflowId: string
  fromVersion: WorkflowVersion
  toVersion: WorkflowVersion
}>()

const emit = defineEmits<{
  close: []
  restore: [version: WorkflowVersion]
}>()

const versionStore = useVersionStore()
const comparison = ref<VersionComparison | null>(null)
const loading = ref(true)
const errorMessage = ref<string | null>(null)

onMounted(async () => {
  loading.value = true
  errorMessage.value = null
  try {
    // GET /workflows/{id}/versions/compare?from=&to=
    // → {fromVersion, toVersion, changes}
    comparison.value = await versionStore.compareVersions(
      props.workflowId,
      props.fromVersion.id,
      props.toVersion.id
    )
  } catch (e) {
    errorMessage.value = tagErrorMessage(e, 'Failed to compare versions.')
  } finally {
    loading.value = false
  }
})

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString()
}

function label(version: WorkflowVersion | undefined): string {
  if (!version) return 'Unknown version'
  return version.versionTag || `v${version.versionNum}`
}

// The API echoes both versions back — prefer them over the props so
// the dialog always shows exactly what the server compared.
const shownFrom = computed(() => comparison.value?.fromVersion ?? props.fromVersion)
const shownTo = computed(() => comparison.value?.toVersion ?? props.toVersion)

const changes = computed(() => comparison.value?.changes ?? null)
const nodesAdded = computed(() => changes.value?.nodesAdded ?? [])
const nodesRemoved = computed(() => changes.value?.nodesRemoved ?? [])
const nodesModified = computed(() => changes.value?.nodesModified ?? [])
const connectionsChanged = computed(() => changes.value?.connectionsChanged ?? false)
const settingsChanged = computed(() => changes.value?.settingsChanged ?? false)
const summary = computed(() => changes.value?.summary ?? '')

const fromNodes = computed(() => {
  if (!props.fromVersion.workflow?.nodes) return []
  return props.fromVersion.workflow.nodes.map((n) => n.name)
})

const toNodes = computed(() => {
  if (!props.toVersion.workflow?.nodes) return []
  return props.toVersion.workflow.nodes.map((n) => n.name)
})

function getNodeStatus(nodeName: string): 'added' | 'removed' | 'modified' | 'unchanged' {
  if (nodesAdded.value.includes(nodeName)) return 'added'
  if (nodesRemoved.value.includes(nodeName)) return 'removed'
  if (nodesModified.value.includes(nodeName)) return 'modified'
  return 'unchanged'
}
</script>

<template>
  <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
    <div class="bg-white dark:bg-slate-800 rounded-xl shadow-2xl w-full max-w-6xl max-h-[90vh] overflow-hidden flex flex-col">
      <!-- Header -->
      <div class="px-6 py-4 border-b border-slate-200 dark:border-slate-700 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-lg">
            <ArrowsRightLeftIcon class="w-5 h-5 text-blue-600 dark:text-blue-400" />
          </div>
          <div>
            <h2 class="text-lg font-semibold text-slate-900 dark:text-white">
              Compare Versions
            </h2>
            <p class="text-sm text-slate-500 dark:text-slate-400">
              {{ label(shownFrom) }} vs {{ label(shownTo) }}
            </p>
          </div>
        </div>
        <button
          @click="emit('close')"
          class="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-700"
        >
          <XMarkIcon class="w-5 h-5" />
        </button>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex-1 flex items-center justify-center p-8">
        <div class="animate-spin rounded-full h-8 w-8 border-4 border-blue-500 border-t-transparent"></div>
      </div>

      <!-- Error -->
      <div
        v-else-if="errorMessage"
        class="flex-1 flex flex-col items-center justify-center p-8 text-center gap-3"
      >
        <p class="text-red-600 dark:text-red-400">{{ errorMessage }}</p>
        <button @click="emit('close')" class="btn-secondary">Close</button>
      </div>

      <!-- Content -->
      <div v-else-if="comparison" class="flex-1 overflow-hidden flex flex-col">
        <!-- Summary -->
        <div class="px-6 py-4 bg-slate-50 dark:bg-slate-900/50 border-b border-slate-200 dark:border-slate-700">
          <div class="flex items-center justify-between gap-4 flex-wrap">
            <div class="flex items-center gap-6">
              <div class="flex items-center gap-2">
                <PlusCircleIcon class="w-5 h-5 text-green-500" />
                <span class="text-sm text-slate-700 dark:text-slate-300">
                  {{ nodesAdded.length }} added
                </span>
              </div>
              <div class="flex items-center gap-2">
                <MinusCircleIcon class="w-5 h-5 text-red-500" />
                <span class="text-sm text-slate-700 dark:text-slate-300">
                  {{ nodesRemoved.length }} removed
                </span>
              </div>
              <div class="flex items-center gap-2">
                <PencilSquareIcon class="w-5 h-5 text-amber-500" />
                <span class="text-sm text-slate-700 dark:text-slate-300">
                  {{ nodesModified.length }} modified
                </span>
              </div>
              <div v-if="connectionsChanged" class="flex items-center gap-2">
                <ArrowPathRoundedSquareIcon class="w-5 h-5 text-blue-500" />
                <span class="text-sm text-slate-700 dark:text-slate-300">Connections changed</span>
              </div>
              <div v-if="settingsChanged" class="flex items-center gap-2">
                <Cog6ToothIcon class="w-5 h-5 text-purple-500" />
                <span class="text-sm text-slate-700 dark:text-slate-300">Settings changed</span>
              </div>
            </div>
            <p v-if="summary" class="text-sm text-slate-500 dark:text-slate-400">
              {{ summary }}
            </p>
          </div>
        </div>

        <!-- Side by Side Comparison -->
        <div class="flex-1 overflow-hidden grid grid-cols-2">
          <!-- From Version -->
          <div class="border-r border-slate-200 dark:border-slate-700 flex flex-col">
            <div class="px-4 py-3 bg-red-50 dark:bg-red-900/20 border-b border-slate-200 dark:border-slate-700">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="font-medium text-slate-900 dark:text-white">
                    {{ label(shownFrom) }}
                  </h3>
                  <p class="text-xs text-slate-500 dark:text-slate-400">
                    {{ formatDate(shownFrom.createdAt) }} by {{ shownFrom.author }}
                  </p>
                </div>
                <button
                  @click="emit('restore', shownFrom)"
                  class="px-3 py-1.5 text-xs bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 hover:bg-red-200 dark:hover:bg-red-900/50 rounded-lg transition-colors"
                >
                  Restore this version
                </button>
              </div>
            </div>
            <div class="flex-1 overflow-y-auto p-4">
              <h4 class="text-xs font-medium text-slate-500 dark:text-slate-400 uppercase mb-3">
                Nodes ({{ fromNodes.length }})
              </h4>
              <div class="space-y-2">
                <div
                  v-for="nodeName in fromNodes"
                  :key="nodeName"
                  class="px-3 py-2 rounded-lg text-sm"
                  :class="{
                    'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300': getNodeStatus(nodeName) === 'removed',
                    'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300': getNodeStatus(nodeName) === 'modified',
                    'bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300': getNodeStatus(nodeName) === 'unchanged',
                  }"
                >
                  <div class="flex items-center gap-2">
                    <MinusCircleIcon v-if="getNodeStatus(nodeName) === 'removed'" class="w-4 h-4" />
                    <PencilSquareIcon v-else-if="getNodeStatus(nodeName) === 'modified'" class="w-4 h-4" />
                    <span>{{ nodeName }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- To Version -->
          <div class="flex flex-col">
            <div class="px-4 py-3 bg-green-50 dark:bg-green-900/20 border-b border-slate-200 dark:border-slate-700">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="font-medium text-slate-900 dark:text-white">
                    {{ label(shownTo) }}
                  </h3>
                  <p class="text-xs text-slate-500 dark:text-slate-400">
                    {{ formatDate(shownTo.createdAt) }} by {{ shownTo.author }}
                  </p>
                </div>
                <button
                  @click="emit('restore', shownTo)"
                  class="px-3 py-1.5 text-xs bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300 hover:bg-green-200 dark:hover:bg-green-900/50 rounded-lg transition-colors"
                >
                  Restore this version
                </button>
              </div>
            </div>
            <div class="flex-1 overflow-y-auto p-4">
              <h4 class="text-xs font-medium text-slate-500 dark:text-slate-400 uppercase mb-3">
                Nodes ({{ toNodes.length }})
              </h4>
              <div class="space-y-2">
                <div
                  v-for="nodeName in toNodes"
                  :key="nodeName"
                  class="px-3 py-2 rounded-lg text-sm"
                  :class="{
                    'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300': getNodeStatus(nodeName) === 'added',
                    'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300': getNodeStatus(nodeName) === 'modified',
                    'bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300': getNodeStatus(nodeName) === 'unchanged',
                  }"
                >
                  <div class="flex items-center gap-2">
                    <PlusCircleIcon v-if="getNodeStatus(nodeName) === 'added'" class="w-4 h-4" />
                    <PencilSquareIcon v-else-if="getNodeStatus(nodeName) === 'modified'" class="w-4 h-4" />
                    <span>{{ nodeName }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Error State -->
      <div v-else class="flex-1 flex items-center justify-center p-8 text-slate-500 dark:text-slate-400">
        Failed to compare versions
      </div>
    </div>
  </div>
</template>
