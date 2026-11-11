<script setup lang="ts">
import { ref, computed } from 'vue'
import { XMarkIcon, ClockIcon, ArrowPathIcon } from '@heroicons/vue/24/outline'
import { useVersionStore } from '@/stores'
import { tagErrorMessage } from '@/api/tags'
import type { WorkflowVersion } from '@/types/version'
import VersionHistory from './VersionHistory.vue'
import VersionCompare from './VersionCompare.vue'
import VersionRestoreDialog from './VersionRestoreDialog.vue'

const props = defineProps<{
  workflowId: string
}>()

const emit = defineEmits<{
  close: []
  restored: [version: WorkflowVersion]
}>()

const versionStore = useVersionStore()

const restoreTarget = ref<WorkflowVersion | null>(null)
const compareFrom = ref<WorkflowVersion | null>(null)
const compareTo = ref<WorkflowVersion | null>(null)
const preview = ref<WorkflowVersion | null>(null)
const restoring = ref(false)
const notice = ref<{ type: 'success' | 'error'; message: string } | null>(null)

const compareOpen = computed(() => compareFrom.value !== null && compareTo.value !== null)

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
    // POST /workflows/{id}/versions/{versionId}/restore
    // → {message, restoredFrom, backupCreated}
    const result = await versionStore.restoreVersion(props.workflowId, target.id, {
      createBackup,
      description: description || undefined,
    })
    restoreTarget.value = null
    notice.value = {
      type: 'success',
      message: result?.message || 'Workflow restored successfully.',
    }
    if (result?.restoredFrom) {
      emit('restored', result.restoredFrom)
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

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString()
}
</script>

<template>
  <div
    class="fixed inset-0 z-40 bg-black/30 flex justify-end"
    @click.self="emit('close')"
  >
    <!-- Drawer -->
    <aside
      class="w-full sm:w-96 h-full bg-white dark:bg-slate-800 border-l border-slate-200 dark:border-slate-700 shadow-xl flex flex-col"
    >
      <!-- Header -->
      <div class="flex items-center justify-between px-4 py-3 border-b border-slate-200 dark:border-slate-700">
        <div class="flex items-center gap-2">
          <ClockIcon class="w-5 h-5 text-violet-500" />
          <h3 class="font-semibold text-slate-900 dark:text-white">Version history</h3>
        </div>
        <button
          type="button"
          class="p-1.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-700"
          title="Close version history"
          aria-label="Close version history"
          @click="emit('close')"
        >
          <XMarkIcon class="w-5 h-5" />
        </button>
      </div>

      <!-- Notice -->
      <div
        v-if="notice"
        :class="notice.type === 'success'
          ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-800 dark:text-green-300'
          : 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-800 dark:text-red-300'"
        class="mx-4 mt-3 flex items-start gap-2 rounded-lg border px-3 py-2 text-sm"
      >
        <p class="flex-1">{{ notice.message }}</p>
        <button type="button" class="opacity-60 hover:opacity-100" @click="notice = null">
          <XMarkIcon class="w-4 h-4" />
        </button>
      </div>

      <!-- Version preview -->
      <div
        v-if="preview"
        class="mx-4 mt-3 rounded-lg border border-slate-200 dark:border-slate-700 p-3"
      >
        <div class="flex items-start justify-between gap-2">
          <div>
            <p class="text-sm font-medium text-slate-900 dark:text-white">
              {{ preview.versionTag || `v${preview.versionNum}` }}
            </p>
            <p class="text-xs text-slate-500 dark:text-slate-400">
              {{ formatDate(preview.createdAt) }} by {{ preview.author }}
            </p>
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
        <p v-if="preview.description" class="mt-2 text-xs text-slate-600 dark:text-slate-400">
          {{ preview.description }}
        </p>
        <p class="mt-2 text-xs text-slate-500 dark:text-slate-400">
          {{ preview.workflow?.nodes?.length ?? 0 }} nodes ·
          {{ preview.workflow?.connections ? Object.keys(preview.workflow.connections).length : 0 }} connection groups
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

      <!-- History list -->
      <div class="flex-1 min-h-0">
        <VersionHistory
          :workflow-id="workflowId"
          @restore="openRestore"
          @compare="openCompare"
          @view-version="preview = $event"
        />
      </div>

      <!-- Footer -->
      <div class="px-4 py-2 border-t border-slate-200 dark:border-slate-700 text-xs text-slate-500 dark:text-slate-400 flex items-center justify-between">
        <span>
          {{ versionStore.total }} version{{ versionStore.total !== 1 ? 's' : '' }}
        </span>
        <button type="button" class="btn-ghost py-1 px-2 text-xs" @click="emit('close')">
          Close
        </button>
      </div>
    </aside>

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
      :workflow-id="workflowId"
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