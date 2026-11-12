<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useVersionStore } from '@/stores'
import { tagErrorMessage } from '@/api/tags'
import type { WorkflowVersion, VersionCreateRequest } from '@/types/version'
import {
  ClockIcon,
  PlusIcon,
  ArrowPathIcon,
  TrashIcon,
  DocumentDuplicateIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  TagIcon,
  UserIcon,
  XMarkIcon,
} from '@heroicons/vue/24/outline'
import { CheckCircleIcon } from '@heroicons/vue/24/solid'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'

const props = defineProps<{
  workflowId: string
}>()

const emit = defineEmits<{
  restore: [version: WorkflowVersion]
  compare: [fromVersion: WorkflowVersion, toVersion: WorkflowVersion]
  viewVersion: [version: WorkflowVersion]
}>()

const versionStore = useVersionStore()

// UI State
const showCreateForm = ref(false)
const expandedVersionId = ref<string | null>(null)
const selectedVersions = ref<string[]>([])
// Error of the last action, shown inline in the drawer.
const errorMessage = ref<string | null>(null)

// Create form data
const newVersionTag = ref('')
const newVersionDescription = ref('')

onMounted(() => {
  versionStore.fetchVersions(props.workflowId).catch(() => {
    // The store keeps the message; it is rendered below.
  })
})

watch(() => props.workflowId, (newId) => {
  if (newId) {
    versionStore.fetchVersions(newId).catch(() => {
      // The store keeps the message; it is rendered below.
    })
  }
})

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatRelativeDate(dateString: string): string {
  const date = new Date(dateString)
  const now = new Date()
  const diff = now.getTime() - date.getTime()

  if (diff < 60000) return 'Just now'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
  if (diff < 604800000) return `${Math.floor(diff / 86400000)}d ago`
  return formatDate(dateString)
}

function toggleExpand(versionId: string) {
  expandedVersionId.value = expandedVersionId.value === versionId ? null : versionId
}

function toggleSelect(versionId: string) {
  const index = selectedVersions.value.indexOf(versionId)
  if (index === -1) {
    if (selectedVersions.value.length < 2) {
      selectedVersions.value.push(versionId)
    }
  } else {
    selectedVersions.value.splice(index, 1)
  }
}

async function handleCreateVersion() {
  const data: VersionCreateRequest = {
    versionTag: newVersionTag.value || undefined,
    description: newVersionDescription.value || undefined,
  }

  errorMessage.value = null
  try {
    await versionStore.createVersion(props.workflowId, data)
    showCreateForm.value = false
    newVersionTag.value = ''
    newVersionDescription.value = ''
  } catch (e) {
    errorMessage.value = tagErrorMessage(e, 'Failed to create the version.')
  }
}

async function handleRestore(version: WorkflowVersion) {
  emit('restore', version)
}

async function handleDelete(version: WorkflowVersion) {
  const label = version.versionTag || `v${version.versionNum}`
  if (!confirm(`Delete version ${label}? This cannot be undone.`)) {
    return
  }

  errorMessage.value = null
  try {
    await versionStore.deleteVersion(props.workflowId, version.id)
  } catch (e) {
    errorMessage.value = tagErrorMessage(e, 'Failed to delete the version.')
  }
}

function handleCompare() {
  if (selectedVersions.value.length !== 2) return
  const versions = versionStore.versions
  const fromVersion = versions.find((v) => v.id === selectedVersions.value[0])
  const toVersion = versions.find((v) => v.id === selectedVersions.value[1])
  if (fromVersion && toVersion) {
    emit('compare', fromVersion, toVersion)
  }
}
</script>

<template>
  <div class="flex h-full flex-col border-l border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
    <!-- Header -->
    <div class="border-b border-neutral-200 px-4 py-3 dark:border-neutral-800">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <ClockIcon class="h-4 w-4 text-brand-500" />
          <h3 class="font-semibold text-neutral-950 dark:text-neutral-50">
            Version History
          </h3>
        </div>
        <IconButton
          label="Create new version"
          @click="showCreateForm = !showCreateForm"
        >
          <PlusIcon class="h-4 w-4" />
        </IconButton>
      </div>

      <!-- Create Form -->
      <div
        v-if="showCreateForm"
        class="mt-3 space-y-2 rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-neutral-800 dark:bg-neutral-950"
      >
        <input
          v-model="newVersionTag"
          type="text"
          placeholder="Version tag (e.g., v1.0.0)"
          class="input"
        >
        <textarea
          v-model="newVersionDescription"
          placeholder="Description (optional)"
          rows="2"
          class="input"
        />
        <div class="flex justify-end gap-2">
          <Button
            variant="ghost"
            size="sm"
            @click="showCreateForm = false"
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            :disabled="versionStore.loading"
            :loading="versionStore.loading"
            @click="handleCreateVersion"
          >
            Create
          </Button>
        </div>
      </div>

      <!-- Compare Button -->
      <div
        v-if="selectedVersions.length === 2"
        class="mt-3"
      >
        <Button
          variant="secondary"
          size="sm"
          class="w-full"
          @click="handleCompare"
        >
          <template #icon>
            <DocumentDuplicateIcon class="h-4 w-4" />
          </template>
          <span>Compare selected versions</span>
        </Button>
      </div>
    </div>

    <!-- Error -->
    <div
      v-if="errorMessage || versionStore.error"
      class="mx-4 mt-3 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300"
    >
      <span class="flex-1">{{ errorMessage || versionStore.error }}</span>
      <button
        type="button"
        class="rounded-sm opacity-60 transition-opacity hover:opacity-100"
        aria-label="Dismiss error"
        @click="errorMessage = null; versionStore.clearError()"
      >
        <XMarkIcon class="h-3.5 w-3.5" />
      </button>
    </div>

    <!-- Version List -->
    <div class="flex-1 overflow-y-auto">
      <!-- Loading -->
      <div
        v-if="versionStore.loading"
        class="p-4 text-center text-neutral-500"
      >
        <span
          class="inline-block h-6 w-6 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
          aria-hidden="true"
        />
      </div>

      <!-- Empty State -->
      <div
        v-else-if="versionStore.versions.length === 0"
        class="p-6 text-center text-neutral-500 dark:text-neutral-400"
      >
        <ClockIcon class="mx-auto mb-2 h-10 w-10 opacity-50" />
        <p class="text-sm">No versions yet</p>
        <p class="mt-1 text-xs">Create a version to track changes</p>
      </div>

      <!-- Versions -->
      <div
        v-else
        class="divide-y divide-neutral-100 dark:divide-neutral-800"
      >
        <div
          v-for="version in versionStore.sortedVersions"
          :key="version.id"
          class="group"
        >
          <div
            class="cursor-pointer px-4 py-3 transition-colors duration-200 hover:bg-neutral-50 dark:hover:bg-neutral-800/50"
            @click="toggleExpand(version.id)"
          >
            <div class="flex items-start gap-3">
              <!-- Checkbox for comparison -->
              <button
                type="button"
                class="mt-0.5 flex h-4 w-4 items-center justify-center rounded border transition-colors duration-200"
                :class="selectedVersions.includes(version.id)
                  ? 'border-brand-500 bg-brand-500 text-white'
                  : 'border-neutral-300 hover:border-brand-400 dark:border-neutral-600'"
                :aria-label="`Select version ${version.versionTag || version.versionNum} for comparison`"
                @click.stop="toggleSelect(version.id)"
              >
                <svg
                  v-if="selectedVersions.includes(version.id)"
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="3"
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              </button>

              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-neutral-950 dark:text-neutral-50">
                    {{ version.versionTag || `v${version.versionNum}` }}
                  </span>
                  <span class="font-mono text-3xs text-neutral-400 dark:text-neutral-500">
                    #{{ version.versionNum }}
                  </span>
                  <CheckCircleIcon
                    v-if="version.isCurrent"
                    class="h-4 w-4 text-green-600 dark:text-green-400"
                    title="Current version"
                  />
                </div>
                <p class="mt-0.5 text-xs text-neutral-500 dark:text-neutral-400">
                  {{ formatRelativeDate(version.createdAt) }}
                </p>
              </div>

              <component
                :is="expandedVersionId === version.id ? ChevronUpIcon : ChevronDownIcon"
                class="h-4 w-4 text-neutral-400"
              />
            </div>
          </div>

          <!-- Expanded Details -->
          <div
            v-if="expandedVersionId === version.id"
            class="border-t border-neutral-100 bg-neutral-50 px-4 py-3 dark:border-neutral-800 dark:bg-neutral-950"
          >
            <!-- Author -->
            <div class="mb-2 flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
              <UserIcon class="h-3.5 w-3.5" />
              <span>{{ version.author }}</span>
              <span class="text-neutral-300 dark:text-neutral-600">|</span>
              <span>{{ formatDate(version.createdAt) }}</span>
            </div>

            <!-- Description -->
            <p
              v-if="version.description"
              class="mb-2 text-sm text-neutral-700 dark:text-neutral-300"
            >
              {{ version.description }}
            </p>

            <!-- Changes -->
            <div
              v-if="version.changes?.length"
              class="mb-3"
            >
              <h4 class="mb-1 text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
                Changes:
              </h4>
              <ul class="space-y-0.5 text-xs text-neutral-600 dark:text-neutral-400">
                <li
                  v-for="(change, i) in version.changes.slice(0, 5)"
                  :key="i"
                  class="flex items-start gap-1"
                >
                  <span class="text-neutral-400">-</span>
                  {{ change }}
                </li>
                <li class="italic text-neutral-400">
                  +{{ version.changes.length - 5 }} more changes
                </li>
              </ul>
            </div>

            <!-- Tags -->
            <div
              v-if="version.tags?.length"
              class="mb-3 flex flex-wrap gap-1"
            >
              <span
                v-for="tag in version.tags"
                :key="tag"
                class="inline-flex h-5 items-center gap-1 rounded-lg border border-neutral-200 bg-neutral-100 px-2 text-2xs text-neutral-600 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-300"
              >
                <TagIcon class="h-3 w-3" />
                {{ tag }}
              </span>
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-2">
              <button
                v-if="!version.isCurrent"
                type="button"
                class="flex items-center gap-1.5 rounded-lg bg-brand-50 px-2.5 py-1.5 text-xs text-brand-700 transition-colors duration-200 hover:bg-brand-100 dark:bg-brand-950/50 dark:text-brand-300 dark:hover:bg-brand-900/50"
                @click.stop="handleRestore(version)"
              >
                <ArrowPathIcon class="h-3.5 w-3.5" />
                Restore
              </button>
              <button
                type="button"
                class="flex items-center gap-1.5 rounded-lg bg-neutral-100 px-2.5 py-1.5 text-xs text-neutral-700 transition-colors duration-200 hover:bg-neutral-200 dark:bg-neutral-800 dark:text-neutral-300 dark:hover:bg-neutral-700"
                @click.stop="emit('viewVersion', version)"
              >
                <DocumentDuplicateIcon class="h-3.5 w-3.5" />
                View
              </button>
              <button
                v-if="!version.isCurrent"
                type="button"
                class="flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs text-red-700 transition-colors duration-200 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/50"
                @click.stop="handleDelete(version)"
              >
                <TrashIcon class="h-3.5 w-3.5" />
                Delete
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Footer -->
    <div class="border-t border-neutral-200 px-4 py-2 text-xs text-neutral-500 dark:border-neutral-800 dark:text-neutral-400">
      {{ versionStore.total }} version{{ versionStore.total !== 1 ? 's' : '' }}
    </div>
  </div>
</template>
