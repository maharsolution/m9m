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
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'

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
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[oklch(100%_0_89.88/0.8)] p-4 backdrop-blur-[2px] dark:bg-black/60">
    <div class="flex max-h-[90vh] w-full max-w-6xl flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900">
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <div class="flex items-center gap-3">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-50 text-blue-600 dark:bg-blue-950/50 dark:text-blue-400">
            <ArrowsRightLeftIcon class="h-4 w-4" />
          </div>
          <div>
            <h2 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
              Compare Versions
            </h2>
            <p class="text-xs text-neutral-500 dark:text-neutral-400">
              {{ label(shownFrom) }} vs {{ label(shownTo) }}
            </p>
          </div>
        </div>
        <IconButton
          label="Close"
          @click="emit('close')"
        >
          <XMarkIcon class="h-4 w-4" />
        </IconButton>
      </div>

      <!-- Loading -->
      <div
        v-if="loading"
        class="flex flex-1 items-center justify-center p-8"
      >
        <span
          class="inline-block h-8 w-8 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
          aria-hidden="true"
        />
      </div>

      <!-- Error -->
      <div
        v-else-if="errorMessage"
        class="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center"
      >
        <p class="text-sm text-red-700 dark:text-red-400">{{ errorMessage }}</p>
        <Button
          variant="secondary"
          size="sm"
          @click="emit('close')"
        >
          Close
        </Button>
      </div>

      <!-- Content -->
      <div
        v-else-if="comparison"
        class="flex flex-1 flex-col overflow-hidden"
      >
        <!-- Summary -->
        <div class="border-b border-neutral-200 bg-neutral-50 px-6 py-4 dark:border-neutral-800 dark:bg-neutral-950">
          <div class="flex flex-wrap items-center justify-between gap-4">
            <div class="flex flex-wrap items-center gap-6">
              <div class="flex items-center gap-2">
                <PlusCircleIcon class="h-4 w-4 text-green-600 dark:text-green-400" />
                <span class="text-sm text-neutral-700 dark:text-neutral-300">
                  {{ nodesAdded.length }} added
                </span>
              </div>
              <div class="flex items-center gap-2">
                <MinusCircleIcon class="h-4 w-4 text-red-600 dark:text-red-400" />
                <span class="text-sm text-neutral-700 dark:text-neutral-300">
                  {{ nodesRemoved.length }} removed
                </span>
              </div>
              <div class="flex items-center gap-2">
                <PencilSquareIcon class="h-4 w-4 text-yellow-700 dark:text-yellow-400" />
                <span class="text-sm text-neutral-700 dark:text-neutral-300">
                  {{ nodesModified.length }} modified
                </span>
              </div>
              <div
                v-if="connectionsChanged"
                class="flex items-center gap-2"
              >
                <ArrowPathRoundedSquareIcon class="h-4 w-4 text-blue-600 dark:text-blue-400" />
                <span class="text-sm text-neutral-700 dark:text-neutral-300">Connections changed</span>
              </div>
              <div
                v-if="settingsChanged"
                class="flex items-center gap-2"
              >
                <Cog6ToothIcon class="h-4 w-4 text-purple-600 dark:text-purple-400" />
                <span class="text-sm text-neutral-700 dark:text-neutral-300">Settings changed</span>
              </div>
            </div>
            <p
              v-if="summary"
              class="text-sm text-neutral-500 dark:text-neutral-400"
            >
              {{ summary }}
            </p>
          </div>
        </div>

        <!-- Side by Side Comparison -->
        <div class="grid flex-1 grid-cols-2 overflow-hidden">
          <!-- From Version -->
          <div class="flex flex-col border-r border-neutral-200 dark:border-neutral-800">
            <div class="border-b border-neutral-200 bg-red-50 px-4 py-3 dark:border-neutral-800 dark:bg-red-950/40">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <h3 class="font-medium text-neutral-950 dark:text-neutral-50">
                    {{ label(shownFrom) }}
                  </h3>
                  <p class="text-xs text-neutral-500 dark:text-neutral-400">
                    {{ formatDate(shownFrom.createdAt) }} by {{ shownFrom.author }}
                  </p>
                </div>
                <button
                  type="button"
                  class="rounded-lg bg-red-100 px-3 py-1.5 text-xs text-red-800 transition-colors duration-200 hover:bg-red-200 dark:bg-red-950/50 dark:text-red-300 dark:hover:bg-red-900/50"
                  @click="emit('restore', shownFrom)"
                >
                  Restore this version
                </button>
              </div>
            </div>
            <div class="flex-1 overflow-y-auto p-4">
              <h4 class="mb-3 text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
                Nodes ({{ fromNodes.length }})
              </h4>
              <div class="space-y-2">
                <div
                  v-for="nodeName in fromNodes"
                  :key="nodeName"
                  class="rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm dark:border-neutral-800 dark:bg-neutral-900"
                  :class="{
                    'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300': getNodeStatus(nodeName) === 'removed',
                    'border-yellow-200 bg-yellow-50 text-yellow-800 dark:border-yellow-900 dark:bg-yellow-950/50 dark:text-yellow-300': getNodeStatus(nodeName) === 'modified',
                  }"
                >
                  <div class="flex items-center gap-2">
                    <MinusCircleIcon
                      v-if="getNodeStatus(nodeName) === 'removed'"
                      class="h-4 w-4"
                    />
                    <PencilSquareIcon
                      v-else-if="getNodeStatus(nodeName) === 'modified'"
                      class="h-4 w-4"
                    />
                    <span>{{ nodeName }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- To Version -->
          <div class="flex flex-col">
            <div class="border-b border-neutral-200 bg-green-50 px-4 py-3 dark:border-neutral-800 dark:bg-green-950/40">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <h3 class="font-medium text-neutral-950 dark:text-neutral-50">
                    {{ label(shownTo) }}
                  </h3>
                  <p class="text-xs text-neutral-500 dark:text-neutral-400">
                    {{ formatDate(shownTo.createdAt) }} by {{ shownTo.author }}
                  </p>
                </div>
                <button
                  type="button"
                  class="rounded-lg bg-green-100 px-3 py-1.5 text-xs text-green-800 transition-colors duration-200 hover:bg-green-200 dark:bg-green-950/50 dark:text-green-300 dark:hover:bg-green-900/50"
                  @click="emit('restore', shownTo)"
                >
                  Restore this version
                </button>
              </div>
            </div>
            <div class="flex-1 overflow-y-auto p-4">
              <h4 class="mb-3 text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
                Nodes ({{ toNodes.length }})
              </h4>
              <div class="space-y-2">
                <div
                  v-for="nodeName in toNodes"
                  :key="nodeName"
                  class="rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm dark:border-neutral-800 dark:bg-neutral-900"
                  :class="{
                    'border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/50 dark:text-green-300': getNodeStatus(nodeName) === 'added',
                    'border-yellow-200 bg-yellow-50 text-yellow-800 dark:border-yellow-900 dark:bg-yellow-950/50 dark:text-yellow-300': getNodeStatus(nodeName) === 'modified',
                  }"
                >
                  <div class="flex items-center gap-2">
                    <PlusCircleIcon
                      v-if="getNodeStatus(nodeName) === 'added'"
                      class="h-4 w-4"
                    />
                    <PencilSquareIcon
                      v-else-if="getNodeStatus(nodeName) === 'modified'"
                      class="h-4 w-4"
                    />
                    <span>{{ nodeName }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Error State -->
      <div
        v-else
        class="flex flex-1 items-center justify-center p-8 text-neutral-500 dark:text-neutral-400"
      >
        <p class="text-sm">Failed to compare versions</p>
      </div>
    </div>
  </div>
</template>
