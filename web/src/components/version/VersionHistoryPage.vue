<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowLeftIcon,
  ClockIcon,
  XMarkIcon,
  ArrowPathIcon,
} from '@heroicons/vue/24/outline'
import { useVersionStore } from '@/stores'
import { tagErrorMessage } from '@/api/tags'
import { getWorkflow } from '@/api/workflows'
import type { Workflow } from '@/types'
import type { WorkflowVersion } from '@/types/version'
import VersionHistory from './VersionHistory.vue'
import VersionCompare from './VersionCompare.vue'
import VersionRestoreDialog from './VersionRestoreDialog.vue'

const props = defineProps<{
  id: string
}>()

const router = useRouter()
const versionStore = useVersionStore()

const workflow = ref<Workflow | null>(null)
const loadingWorkflow = ref(true)
const restoreTarget = ref<WorkflowVersion | null>(null)
const preview = ref<WorkflowVersion | null>(null)
const compareFrom = ref<WorkflowVersion | null>(null)
const compareTo = ref<WorkflowVersion | null>(null)
const restoring = ref(false)
const notice = ref<{ type: 'success' | 'error'; message: string } | null>(null)

const compareOpen = computed(() => compareFrom.value !== null && compareTo.value !== null)

onMounted(async () => {
  try {
    workflow.value = await getWorkflow(props.id)
  } catch (e) {
    notice.value = {
      type: 'error',
      message: tagErrorMessage(e, 'Failed to load the workflow.'),
    }
  } finally {
    loadingWorkflow.value = false
  }
})

function openRestore(version: WorkflowVersion) {
  notice.value = null
  restoreTarget.value = version
}

function openCompare(from: WorkflowVersion, to: WorkflowVersion) {
  notice.value = null
  compareFrom.value = from
  compareTo.value = to
}

function closeCompare() {
  compareFrom.value = null
  compareTo.value = null
}

async function confirmRestore(createBackup: boolean, description: string) {
  const target = restoreTarget.value
  if (!target) return

  restoring.value = true
  notice.value = null
  try {
    const result = await versionStore.restoreVersion(props.id, target.id, {
      createBackup,
      description: description || undefined,
    })
    restoreTarget.value = null
    notice.value = {
      type: 'success',
      message: result?.message || 'Workflow restored successfully.',
    }
  } catch (e) {
    notice.value = {
      type: 'error',
      message: tagErrorMessage(e, 'Failed to restore the version.'),
    }
  } finally {
    restoring.value = false
  }
}
</script>

<template>
  <div class="p-6">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <div class="flex items-center gap-3">
        <button
          type="button"
          class="p-2 rounded-lg text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700"
          title="Back to the editor"
          aria-label="Back to the editor"
          @click="router.push(`/workflows/${id}`)"
        >
          <ArrowLeftIcon class="w-5 h-5" />
        </button>
        <div>
          <h1 class="text-2xl font-bold text-slate-900 dark:text-white flex items-center gap-2">
            <ClockIcon class="w-6 h-6 text-violet-500" />
            Version history
          </h1>
          <p class="text-slate-500 dark:text-slate-400">
            {{ loadingWorkflow ? 'Loading workflow...' : workflow?.name }}
          </p>
        </div>
      </div>
      <button
        type="button"
        class="btn-secondary"
        @click="router.push(`/workflows/${id}`)"
      >
        Open editor
      </button>
    </div>

    <!-- Notice -->
    <div
      v-if="notice"
      :class="notice.type === 'success'
        ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-800 dark:text-green-300'
        : 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-800 dark:text-red-300'"
      class="mb-4 flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
    >
      <p class="flex-1">{{ notice.message }}</p>
      <button type="button" class="opacity-60 hover:opacity-100" @click="notice = null">
        <XMarkIcon class="w-4 h-4" />
      </button>
    </div>

    <!-- Version preview -->
    <div
      v-if="preview"
      class="card p-4 mb-4 flex items-start justify-between gap-4"
    >
      <div class="min-w-0">
        <p class="text-sm font-semibold text-slate-900 dark:text-white">
          {{ preview.versionTag || `v${preview.versionNum}` }}
        </p>
        <p class="text-xs text-slate-500 dark:text-slate-400">
          {{ new Date(preview.createdAt).toLocaleString() }} by {{ preview.author }}
        </p>
        <p v-if="preview.description" class="mt-1 text-sm text-slate-600 dark:text-slate-300">
          {{ preview.description }}
        </p>
        <ul
          v-if="preview.workflow?.nodes?.length"
          class="mt-2 max-h-40 overflow-y-auto text-xs text-slate-600 dark:text-slate-400 space-y-0.5"
        >
          <li v-for="node in preview.workflow.nodes" :key="node.name">
            · {{ node.name }}
          </li>
        </ul>
      </div>
      <button
        type="button"
        class="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
        title="Close preview"
        aria-label="Close preview"
        @click="preview = null"
      >
        <XMarkIcon class="w-4 h-4" />
      </button>
    </div>

    <!-- History -->
    <div class="card overflow-hidden">
      <VersionHistory
        :workflow-id="id"
        @restore="openRestore"
        @compare="openCompare"
        @view-version="preview = $event"
      />
    </div>

    <!-- Restore confirmation -->
    <VersionRestoreDialog
      v-if="restoreTarget"
      :version="restoreTarget"
      @confirm="confirmRestore"
      @cancel="restoreTarget = null"
    />

    <!-- Compare -->
    <VersionCompare
      v-if="compareOpen"
      :workflow-id="id"
      :from-version="compareFrom!"
      :to-version="compareTo!"
      @close="closeCompare"
      @restore="openRestore"
    />

    <!-- Restoring overlay -->
    <div
      v-if="restoring"
      class="fixed inset-0 z-[60] bg-black/40 flex items-center justify-center"
    >
      <div class="flex items-center gap-3 rounded-lg bg-white dark:bg-slate-800 px-5 py-4 shadow-xl">
        <ArrowPathIcon class="w-5 h-5 animate-spin text-violet-500" />
        <span class="text-sm text-slate-700 dark:text-slate-200">Restoring version...</span>
      </div>
    </div>
  </div>
</template>
