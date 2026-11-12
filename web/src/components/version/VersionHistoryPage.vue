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
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'

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
  <div>
    <!-- Page header -->
    <div class="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div class="flex items-center gap-3">
        <IconButton
          label="Back to the editor"
          @click="router.push(`/workflows/${id}`)"
        >
          <ArrowLeftIcon class="h-4 w-4" />
        </IconButton>
        <div>
          <h1 class="flex items-center gap-2 text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
            <ClockIcon class="h-6 w-6 text-brand-500" />
            Version history
          </h1>
          <p class="mt-1 text-xs text-neutral-600 dark:text-neutral-400">
            {{ loadingWorkflow ? 'Loading workflow…' : workflow?.name }}
          </p>
        </div>
      </div>
      <Button
        variant="secondary"
        size="sm"
        @click="router.push(`/workflows/${id}`)"
      >
        Open editor
      </Button>
    </div>

    <!-- Notice -->
    <div
      v-if="notice"
      class="mb-4 flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
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
      class="mb-4 flex items-start justify-between gap-4 rounded-xl border border-neutral-200 bg-white p-4 dark:border-neutral-800 dark:bg-neutral-900"
    >
      <div class="min-w-0">
        <p class="text-sm font-semibold text-neutral-950 dark:text-neutral-50">
          {{ preview.versionTag || `v${preview.versionNum}` }}
        </p>
        <p class="text-xs text-neutral-500 dark:text-neutral-400">
          {{ new Date(preview.createdAt).toLocaleString() }} by {{ preview.author }}
        </p>
        <p
          v-if="preview.description"
          class="mt-1 text-sm text-neutral-600 dark:text-neutral-300"
        >
          {{ preview.description }}
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

    <!-- History -->
    <div class="overflow-hidden rounded-xl border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
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
      class="fixed inset-0 z-[60] flex items-center justify-center bg-black/40"
    >
      <div class="flex items-center gap-3 rounded-lg bg-white px-5 py-4 shadow-xl dark:bg-neutral-900">
        <ArrowPathIcon class="h-5 w-5 animate-spin text-brand-500" />
        <span class="text-sm text-neutral-700 dark:text-neutral-200">Restoring version…</span>
      </div>
    </div>
  </div>
</template>
