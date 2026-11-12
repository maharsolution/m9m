<script setup lang="ts">
import { ref } from 'vue'
import type { WorkflowVersion } from '@/types/version'
import {
  XMarkIcon,
  ArrowPathIcon,
  ExclamationTriangleIcon,
  ShieldCheckIcon,
} from '@heroicons/vue/24/outline'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'

defineProps<{
  version: WorkflowVersion
}>()

const emit = defineEmits<{
  confirm: [createBackup: boolean, description: string]
  cancel: []
}>()

const createBackup = ref(true)
const description = ref('')

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString()
}

function handleConfirm() {
  emit('confirm', createBackup.value, description.value)
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[oklch(100%_0_89.88/0.8)] p-4 backdrop-blur-[2px] dark:bg-black/60">
    <div class="w-full max-w-lg overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900">
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <div class="flex items-center gap-3">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-yellow-100 text-yellow-700 dark:bg-yellow-950/50 dark:text-yellow-400">
            <ArrowPathIcon class="h-4 w-4" />
          </div>
          <h2 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
            Restore Version
          </h2>
        </div>
        <IconButton
          label="Close"
          @click="emit('cancel')"
        >
          <XMarkIcon class="h-4 w-4" />
        </IconButton>
      </div>

      <!-- Content -->
      <div class="p-6">
        <!-- Warning -->
        <div class="mb-4 flex items-start gap-3 rounded-lg border border-yellow-200 bg-yellow-50 p-4 dark:border-yellow-900 dark:bg-yellow-950/50">
          <ExclamationTriangleIcon class="mt-0.5 h-4 w-4 shrink-0 text-yellow-700 dark:text-yellow-400" />
          <div>
            <p class="text-sm text-yellow-900 dark:text-yellow-200">
              This will replace the current workflow with the state from version <strong>{{ version.versionTag }}</strong>.
            </p>
            <p class="mt-1 text-xs text-yellow-700 dark:text-yellow-400">
              Created on {{ formatDate(version.createdAt) }} by {{ version.author }}
            </p>
          </div>
        </div>

        <!-- Version Info -->
        <div class="mb-4 rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-neutral-800 dark:bg-neutral-950">
          <h4 class="mb-2 font-medium text-neutral-950 dark:text-neutral-50">
            {{ version.versionTag }}
          </h4>
          <p
            v-if="version.description"
            class="mb-2 text-sm text-neutral-600 dark:text-neutral-400"
          >
            {{ version.description }}
          </p>
          <div
            v-if="version.changes?.length"
            class="text-xs text-neutral-500 dark:text-neutral-400"
          >
            <span class="font-medium">Changes: </span>
            {{ version.changes.slice(0, 3).join(', ') }}
            <span v-if="version.changes.length > 3">
              and {{ version.changes.length - 3 }} more
            </span>
          </div>
        </div>

        <!-- Backup Option -->
        <div class="mb-4 flex items-center justify-between rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-900 dark:bg-green-950/50">
          <div class="flex items-center gap-3">
            <ShieldCheckIcon class="h-5 w-5 text-green-600 dark:text-green-400" />
            <div>
              <p class="text-sm font-medium text-green-900 dark:text-green-200">
                Create backup before restoring
              </p>
              <p class="text-xs text-green-700 dark:text-green-400">
                Recommended to preserve current state
              </p>
            </div>
          </div>
          <button
            type="button"
            :aria-pressed="createBackup"
            class="relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-green-600"
            :class="createBackup ? 'bg-green-600' : 'bg-neutral-200 dark:bg-neutral-700'"
            @click="createBackup = !createBackup"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow transition duration-200"
              :class="createBackup ? 'translate-x-5' : 'translate-x-0'"
            />
          </button>
        </div>

        <!-- Description -->
        <div>
          <label class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
            Restore note (optional)
          </label>
          <textarea
            v-model="description"
            rows="2"
            placeholder="Why are you restoring this version?"
            class="input"
          />
        </div>
      </div>

      <!-- Footer -->
      <div class="flex justify-end gap-3 border-t border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <Button
          variant="secondary"
          size="sm"
          @click="emit('cancel')"
        >
          Cancel
        </Button>
        <Button
          variant="primary"
          size="sm"
          @click="handleConfirm"
        >
          Restore version
        </Button>
      </div>
    </div>
  </div>
</template>
