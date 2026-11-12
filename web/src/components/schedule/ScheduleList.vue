<script setup lang="ts">
import type { Schedule } from '@/types/schedule'
import {
  ClockIcon,
  PencilSquareIcon,
  TrashIcon,
  CalendarDaysIcon,
  CheckCircleIcon,
  XCircleIcon,
} from '@heroicons/vue/24/outline'
import IconButton from '@/components/common/IconButton.vue'

defineProps<{
  schedules: Schedule[]
  loading?: boolean
}>()

const emit = defineEmits<{
  edit: [schedule: Schedule]
  delete: [schedule: Schedule]
  toggle: [schedule: Schedule]
  viewHistory: [schedule: Schedule]
}>()

function formatDate(dateString?: string): string {
  if (!dateString) return '-'
  const date = new Date(dateString)
  return date.toLocaleString()
}

function formatNextRun(dateString?: string): string {
  if (!dateString) return 'Not scheduled'
  const date = new Date(dateString)
  const now = new Date()
  const diff = date.getTime() - now.getTime()

  if (diff < 0) return 'Overdue'
  if (diff < 60000) return 'Less than a minute'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} minutes`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} hours`
  return `${Math.floor(diff / 86400000)} days`
}
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
    <!-- Loading State -->
    <div
      v-if="loading"
      class="px-6 py-10 text-center text-neutral-600 dark:text-neutral-400"
    >
      <span
        class="inline-block h-8 w-8 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
        aria-hidden="true"
      />
      <p class="mt-3 text-sm">Loading schedules…</p>
    </div>

    <!-- Empty State -->
    <div
      v-else-if="!schedules.length"
      class="px-6 py-10 text-center text-neutral-600 dark:text-neutral-400"
    >
      <CalendarDaysIcon class="mx-auto mb-3 h-12 w-12 opacity-50" />
      <p class="text-sm font-medium">No schedules found</p>
      <p class="mt-1 text-xs">Create a schedule to automate workflow execution</p>
    </div>

    <!-- Schedule List -->
    <div
      v-else
      class="overflow-x-auto"
    >
      <table class="min-w-full divide-y divide-neutral-100 dark:divide-neutral-800">
        <thead class="bg-neutral-50 dark:bg-neutral-950">
          <tr>
            <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
              Status
            </th>
            <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
              Workflow
            </th>
            <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
              Schedule
            </th>
            <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
              Next Run
            </th>
            <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
              Runs
            </th>
            <th class="px-4 py-2.5 text-right text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
              Actions
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-neutral-100 dark:divide-neutral-800">
          <tr
            v-for="schedule in schedules"
            :key="schedule.id"
            class="transition-colors duration-200 hover:bg-neutral-50 dark:hover:bg-neutral-800/40"
          >
            <!-- Status -->
            <td class="whitespace-nowrap px-4 py-3">
              <button
                type="button"
                class="flex items-center gap-2 rounded-sm focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500"
                :title="schedule.enabled ? 'Click to disable' : 'Click to enable'"
                @click="emit('toggle', schedule)"
              >
                <span
                  class="flex h-7 w-7 items-center justify-center rounded-lg transition-colors duration-200"
                  :class="schedule.enabled
                    ? 'bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-400'
                    : 'bg-neutral-100 text-neutral-400 dark:bg-neutral-800 dark:text-neutral-500'"
                >
                  <CheckCircleIcon
                    v-if="schedule.enabled"
                    class="h-4 w-4"
                  />
                  <XCircleIcon
                    v-else
                    class="h-4 w-4"
                  />
                </span>
                <span
                  class="text-xs font-medium"
                  :class="schedule.enabled
                    ? 'text-green-700 dark:text-green-400'
                    : 'text-neutral-500 dark:text-neutral-500'"
                >
                  {{ schedule.enabled ? 'Active' : 'Inactive' }}
                </span>
              </button>
            </td>

            <!-- Workflow -->
            <td class="px-4 py-3">
              <div class="text-sm font-medium text-neutral-950 dark:text-neutral-50">
                {{ schedule.workflowName || 'Unknown Workflow' }}
              </div>
              <div class="font-mono text-2xs text-neutral-500 dark:text-neutral-400">
                {{ schedule.workflowId }}
              </div>
            </td>

            <!-- Schedule -->
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <ClockIcon class="h-4 w-4 text-neutral-400" />
                <code class="rounded-md border border-neutral-200 bg-neutral-50 px-2 py-0.5 font-mono text-xs text-neutral-800 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-200">
                  {{ schedule.cronExpression }}
                </code>
              </div>
              <div class="mt-1 text-2xs text-neutral-500 dark:text-neutral-400">
                {{ schedule.timezone }}
              </div>
            </td>

            <!-- Next Run -->
            <td class="px-4 py-3">
              <div
                v-if="schedule.enabled && schedule.nextRun"
                class="text-sm text-neutral-950 dark:text-neutral-50"
              >
                {{ formatNextRun(schedule.nextRun) }}
              </div>
              <div
                v-else
                class="text-sm text-neutral-400 dark:text-neutral-500"
              >
                -
              </div>
              <div
                v-if="schedule.nextRun"
                class="text-2xs text-neutral-500 dark:text-neutral-400"
              >
                {{ formatDate(schedule.nextRun) }}
              </div>
            </td>

            <!-- Runs -->
            <td class="px-4 py-3">
              <div class="text-sm text-neutral-950 dark:text-neutral-50">
                {{ schedule.runCount }} / {{ schedule.maxRuns === 0 ? 'Unlimited' : schedule.maxRuns }}
              </div>
              <div
                v-if="schedule.lastRun"
                class="text-2xs text-neutral-500 dark:text-neutral-400"
              >
                Last: {{ formatDate(schedule.lastRun) }}
              </div>
            </td>

            <!-- Actions -->
            <td class="whitespace-nowrap px-4 py-3 text-right">
              <div class="flex items-center justify-end gap-1">
                <IconButton
                  label="View execution history"
                  @click="emit('viewHistory', schedule)"
                >
                  <CalendarDaysIcon class="h-4 w-4" />
                </IconButton>
                <IconButton
                  label="Edit schedule"
                  @click="emit('edit', schedule)"
                >
                  <PencilSquareIcon class="h-4 w-4" />
                </IconButton>
                <IconButton
                  label="Delete schedule"
                  tone="danger"
                  @click="emit('delete', schedule)"
                >
                  <TrashIcon class="h-4 w-4" />
                </IconButton>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
