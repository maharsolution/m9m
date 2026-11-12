<script setup lang="ts">
import { ref, computed } from 'vue'
import { XMarkIcon, ClockIcon, ArrowPathIcon } from '@heroicons/vue/24/outline'
import { useVersionStore } from '@/stores'
import { tagErrorMessage } from '@/api/tags'
import type { WorkflowVersion } from '@/types/version'
import VersionHistory from './VersionHistory.vue'
import VersionCompare from './VersionCompare.vue'
import VersionRestoreDialog from './VersionRestoreDialog.vue'
import IconButton from '@/components/common/IconButton.vue'

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
    class="fixed inset-0 z-40 flex justify-end bg-black/30"
    @click.self="emit('close')"
  >
    <!-- Drawer -->
    <aside class="flex h-full w-full flex-col border-l border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900 sm:w-96">
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-neutral-200 px-4 py-3 dark:border-neutral-800">
        <div class="flex items-center gap-2">
          <ClockIcon class="h-4 w-4 text-brand-500" />
          <h3 class="font-semibold text-neutral-950 dark:text-neutral-50">
            Version history
          </h3>
        </div>
        <IconButton
          label="Close version history"
          @click="emit('close')"
        >
          <XMarkIcon class="h-4 w-4" />
        </IconButton>
      </div>

      <!-- Notice -->
      <div
        v-if="notice"
        class="mx-4 mt-3 flex items-start gap-2 rounded-lg border px-3 py-2 text-sm"
        :class="notice.type === 'success'
          ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/50 dark:text-green-300'
          : 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300'"
      >
        <p class="flex-1">{{ notice.message }}</p>
        <button
          type="button"
          class="rounded-sm opacity-60 transition-opacity hover:opacity-100"
          aria-label="Dismiss"
          @click="notice = null"
        >
          <XMarkIcon class="h-4 w-4" />
        </button>
      </div>

      <!-- Version preview -->
      <div
        v-if="preview"
        class="mx-4 mt-3 rounded-lg border border-neutral-200 p-3 dark:border-neutral-800"
      >
        <div class="flex items-start justify-between gap-2">
          <div>
            <p class="text-sm font-medium text-neutral-950 dark:text-neutral-50">
              {{ preview.versionTag || `v${preview.versionNum}` }}
            </p>
            <p class="text-xs text-neutral-500 dark:text-neutral-400">
              {{ formatDate(preview.createdAt) }} by {{ preview.author }}
            </p>
          </div>
          <button
            type="button"
            class="rounded-sm p-1 text-neutral-400 transition-colors hover:text-neutral-700 dark:hover:text-neutral-200"
            title="Close preview"
            aria-label="Close preview"
            @click="preview = null"
          >
            <XMarkIcon class="h-4 w-4" />
          </button>
        </div>
        <p
          v-if="preview.description"
          class="mt-2 text-xs text-neutral-600 dark:text-neutral-400"
        >
          {{ preview.description }}
        </p>
        <p class="mt-2 text-xs text-neutral-500 dark:text-neutral-400">
          {{ preview.workflow?.nodes?.length ?? 0 }} nodes ·
          {{ preview.workflow?.connections ? Object.keys(preview.workflow.connections).length : 0 }} connection groups
        </p>
        <ul
          v-if="preview.workflow?.nodes?.length"
          class="mt-2 max-h-40 space-y-0.5 overflow-y-auto text-xs text-neutral-600 dark:text-neutral-400"
        >
          <li
            v-for="node in preview.workflow.nodes"
            :key="node.name"
          >
            · {{ node.name }}
          </li>
        </ul>
      </div>

      <!-- History list -->
      <div class="min-h-0 flex-1">
        <VersionHistory
          :workflow-id="workflowId"
          @restore="openRestore"
          @compare="openCompare"
          @view-version="preview = $event"
        />
      </div>

      <!-- Footer -->
      <div class="flex items-center justify-between border-t border-neutral-200 px-4 py-2 text-xs text-neutral-500 dark:border-neutral-800 dark:text-neutral-400">
        <span>
          {{ versionStore.total }} version{{ versionStore.total !== 1 ? 's' : '' }}
        </span>
        <button
          type="button"
          class="btn-ghost px-2 py-1 text-xs"
          @click="emit('close')"
        >
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
      class="fixed inset-0 z-[60] flex items-center justify-center bg-black/40"
    >
      <div class="flex items-center gap-3 rounded-lg bg-white px-5 py-4 shadow-xl dark:bg-neutral-900">
        <ArrowPathIcon class="h-5 w-5 animate-spin text-brand-500" />
        <span class="text-sm text-neutral-700 dark:text-neutral-200">Restoring version…</span>
      </div>
    </div>
  </div>
</template>
