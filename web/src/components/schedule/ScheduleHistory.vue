<script setup lang="ts">
import { computed } from 'vue'
import type { ExecutionHistory, ExecutionRecord } from '@/types/schedule'
import {
  XMarkIcon,
  ClockIcon,
  CheckCircleIcon,
  XCircleIcon,
  ExclamationTriangleIcon,
  ArrowPathIcon,
  ChartBarIcon,
} from '@heroicons/vue/24/outline'
import IconButton from '@/components/common/IconButton.vue'

const props = defineProps<{
  history: ExecutionHistory | null
  loading?: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString()
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60000).toFixed(1)}m`
}

function getStatusIcon(status: ExecutionRecord['status']) {
  switch (status) {
    case 'success':
      return CheckCircleIcon
    case 'failed':
      return XCircleIcon
    case 'timeout':
      return ExclamationTriangleIcon
    case 'running':
      return ArrowPathIcon
    default:
      return ClockIcon
  }
}

function getStatusClass(status: ExecutionRecord['status']) {
  switch (status) {
    case 'success':
      return 'bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-400'
    case 'failed':
      return 'bg-red-50 text-red-600 dark:bg-red-950/50 dark:text-red-400'
    case 'timeout':
      return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-950/50 dark:text-yellow-400'
    case 'running':
      return 'bg-blue-50 text-blue-600 dark:bg-blue-950/50 dark:text-blue-400'
    default:
      return 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
  }
}

const successRate = computed(() => {
  if (!props.history) return 0
  const total = props.history.successCount + props.history.failureCount
  if (total === 0) return 0
  return Math.round((props.history.successCount / total) * 100)
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[oklch(100%_0_89.88/0.8)] p-4 backdrop-blur-[2px] dark:bg-black/60">
    <div class="flex max-h-[90vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900">
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <div class="flex items-center gap-3">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-950/50 dark:text-brand-400">
            <ChartBarIcon class="h-4 w-4" />
          </div>
          <h2 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
            Execution History
          </h2>
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

      <!-- Content -->
      <div
        v-else-if="history"
        class="flex-1 overflow-y-auto"
      >
        <!-- Stats Cards -->
        <div class="grid grid-cols-2 gap-4 border-b border-neutral-200 p-6 dark:border-neutral-800 lg:grid-cols-4">
          <div class="rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-neutral-800 dark:bg-neutral-950">
            <p class="text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
              Total Executions
            </p>
            <p class="mt-1 text-2xl font-semibold text-neutral-950 dark:text-neutral-50">
              {{ history.successCount + history.failureCount }}
            </p>
          </div>
          <div class="rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-900 dark:bg-green-950/50">
            <p class="text-2xs font-medium uppercase tracking-wide text-green-700 dark:text-green-400">
              Successful
            </p>
            <p class="mt-1 text-2xl font-semibold text-green-800 dark:text-green-300">
              {{ history.successCount }}
            </p>
          </div>
          <div class="rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-900 dark:bg-red-950/50">
            <p class="text-2xs font-medium uppercase tracking-wide text-red-700 dark:text-red-400">
              Failed
            </p>
            <p class="mt-1 text-2xl font-semibold text-red-800 dark:text-red-300">
              {{ history.failureCount }}
            </p>
          </div>
          <div class="rounded-lg border border-brand-200 bg-brand-50 p-4 dark:border-brand-900 dark:bg-brand-950/50">
            <p class="text-2xs font-medium uppercase tracking-wide text-brand-700 dark:text-brand-400">
              Success Rate
            </p>
            <p class="mt-1 text-2xl font-semibold text-brand-800 dark:text-brand-300">
              {{ successRate }}%
            </p>
          </div>
        </div>

        <!-- Additional Stats -->
        <div class="grid grid-cols-1 gap-4 border-b border-neutral-200 bg-neutral-50 px-6 py-4 dark:border-neutral-800 dark:bg-neutral-950 sm:grid-cols-2">
          <div class="flex items-center gap-3">
            <ClockIcon class="h-5 w-5 text-neutral-400" />
            <div>
              <p class="text-xs text-neutral-500 dark:text-neutral-400">Average Duration</p>
              <p class="text-sm font-medium text-neutral-950 dark:text-neutral-50">
                {{ formatDuration(history.averageTime) }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <CheckCircleIcon class="h-5 w-5 text-green-600 dark:text-green-400" />
            <div>
              <p class="text-xs text-neutral-500 dark:text-neutral-400">Last Success</p>
              <p class="text-sm font-medium text-neutral-950 dark:text-neutral-50">
                {{ history.lastSuccess ? formatDate(history.lastSuccess) : 'Never' }}
              </p>
            </div>
          </div>
        </div>

        <!-- Execution List -->
        <div class="p-6">
          <h3 class="mb-4 text-sm font-medium text-neutral-700 dark:text-neutral-300">
            Recent Executions
          </h3>

          <div
            v-if="history.executions.length === 0"
            class="px-4 py-8 text-center text-neutral-600 dark:text-neutral-400"
          >
            <ClockIcon class="mx-auto mb-3 h-12 w-12 opacity-50" />
            <p class="text-sm">No executions yet</p>
          </div>

          <div
            v-else
            class="space-y-3"
          >
            <div
              v-for="execution in history.executions"
              :key="execution.id"
              class="rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-neutral-800 dark:bg-neutral-950"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-center gap-3">
                  <span
                    class="flex h-8 w-8 items-center justify-center rounded-lg"
                    :class="getStatusClass(execution.status)"
                  >
                    <component
                      :is="getStatusIcon(execution.status)"
                      class="h-4 w-4"
                    />
                  </span>
                  <div>
                    <p class="text-sm font-medium capitalize text-neutral-950 dark:text-neutral-50">
                      {{ execution.status }}
                    </p>
                    <p class="text-xs text-neutral-500 dark:text-neutral-400">
                      {{ formatDate(execution.startTime) }}
                    </p>
                  </div>
                </div>
                <div class="text-right">
                  <p class="font-mono text-sm text-neutral-950 dark:text-neutral-50">
                    {{ formatDuration(execution.duration) }}
                  </p>
                  <p
                    v-if="execution.metrics"
                    class="text-xs text-neutral-500 dark:text-neutral-400"
                  >
                    {{ execution.metrics.nodesExecuted }} nodes
                  </p>
                </div>
              </div>

              <!-- Error Message -->
              <div
                v-if="execution.error"
                class="mt-3 rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-900 dark:bg-red-950/50"
              >
                <p class="font-mono text-xs text-red-800 dark:text-red-300">
                  {{ execution.error }}
                </p>
              </div>

              <!-- Metrics -->
              <div
                v-if="execution.metrics"
                class="mt-3 flex items-center gap-4 font-mono text-2xs text-neutral-500 dark:text-neutral-400"
              >
                <span>Data: {{ (execution.metrics.dataProcessed / 1024).toFixed(1) }}KB</span>
                <span>Memory: {{ (execution.metrics.memoryUsed / 1024 / 1024).toFixed(1) }}MB</span>
                <span>CPU: {{ execution.metrics.cpuTime }}ms</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div
        v-else
        class="flex flex-1 items-center justify-center p-8 text-neutral-600 dark:text-neutral-400"
      >
        <div class="text-center">
          <ChartBarIcon class="mx-auto mb-3 h-12 w-12 opacity-50" />
          <p class="text-sm">No history available</p>
        </div>
      </div>
    </div>
  </div>
</template>
