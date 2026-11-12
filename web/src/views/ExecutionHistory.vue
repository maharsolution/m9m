<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import {
  CheckCircleIcon,
  XCircleIcon,
  ClockIcon,
  ArrowPathIcon,
  TrashIcon,
  FunnelIcon,
} from '@heroicons/vue/24/outline'
import { useExecutionStore, useWorkflowStore } from '@/stores'
import type { ExecutionStatus } from '@/types'
import IconButton from '@/components/common/IconButton.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import type { StatusTone } from '@/components/common/StatusBadge.vue'

const router = useRouter()
const executionStore = useExecutionStore()
const workflowStore = useWorkflowStore()

const statusFilter = ref<ExecutionStatus | 'all'>('all')
const workflowFilter = ref<string>('')

onMounted(async () => {
  await Promise.all([
    executionStore.fetchExecutions(),
    workflowStore.fetchWorkflows(),
  ])
  executionStore.setupWebSocketHandlers()
})

const filteredExecutions = computed(() => {
  let executions = executionStore.executions

  if (statusFilter.value !== 'all') {
    executions = executions.filter((e) => e.status === statusFilter.value)
  }

  if (workflowFilter.value) {
    executions = executions.filter((e) => e.workflowId === workflowFilter.value)
  }

  return executions.sort(
    (a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
  )
})

const formatDate = (dateStr: string) => {
  return new Date(dateStr).toLocaleString()
}

const formatDuration = (start: string, end?: string) => {
  if (!end) return 'Running...'
  const duration = new Date(end).getTime() - new Date(start).getTime()
  if (duration < 1000) return `${duration}ms`
  if (duration < 60000) return `${(duration / 1000).toFixed(1)}s`
  return `${Math.floor(duration / 60000)}m ${Math.floor((duration % 60000) / 1000)}s`
}

const getStatusIcon = (status: string) => {
  switch (status) {
    case 'completed': return CheckCircleIcon
    case 'failed': return XCircleIcon
    case 'running': return ArrowPathIcon
    default: return ClockIcon
  }
}

const getStatusColor = (status: string) => {
  switch (status) {
    case 'completed': return 'text-green-600 dark:text-green-400'
    case 'failed': return 'text-red-600 dark:text-red-400'
    case 'running': return 'text-blue-600 dark:text-blue-400 animate-spin'
    case 'cancelled': return 'text-yellow-700 dark:text-yellow-400'
    default: return 'text-neutral-500 dark:text-neutral-400'
  }
}

/** Status family tone of an execution. */
const getStatusTone = (status: string): StatusTone => {
  switch (status) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'running': return 'info'
    case 'cancelled': return 'warning'
    default: return 'neutral'
  }
}

const getWorkflowName = (workflowId: string) => {
  const workflow = workflowStore.workflows.find((w) => w.id === workflowId)
  return workflow?.name || 'Unknown Workflow'
}

const viewExecution = (id: string) => {
  router.push(`/executions/${id}`)
}

const retryExecution = async (id: string) => {
  try {
    await executionStore.retryExecution(id)
  } catch (e) {
    console.error('Failed to retry execution:', e)
  }
}

const deleteExecution = async (id: string) => {
  if (confirm('Are you sure you want to delete this execution?')) {
    try {
      await executionStore.deleteExecution(id)
    } catch (e) {
      console.error('Failed to delete execution:', e)
    }
  }
}
</script>

<template>
  <div>
    <!-- Page header -->
    <div class="mb-6">
      <h1 class="text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
        Execution History
      </h1>
      <p class="mt-1 text-xs text-neutral-600 dark:text-neutral-400">
        View and manage workflow executions
      </p>
    </div>

    <!-- Filters -->
    <div class="mb-6 flex flex-wrap items-center gap-3">
      <!-- Status Filter -->
      <div class="flex items-center gap-2">
        <FunnelIcon class="h-4 w-4 text-neutral-400" />
        <select
          v-model="statusFilter"
          class="h-9 rounded-lg border border-neutral-300 bg-white px-3 text-sm text-neutral-950 transition-colors duration-200 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10"
        >
          <option value="all">All Statuses</option>
          <option value="completed">Completed</option>
          <option value="failed">Failed</option>
          <option value="running">Running</option>
          <option value="cancelled">Cancelled</option>
        </select>
      </div>

      <!-- Workflow Filter -->
      <select
        v-model="workflowFilter"
        class="h-9 rounded-lg border border-neutral-300 bg-white px-3 text-sm text-neutral-950 transition-colors duration-200 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10"
      >
        <option value="">All Workflows</option>
        <option
          v-for="workflow in workflowStore.workflows"
          :key="workflow.id"
          :value="workflow.id"
        >
          {{ workflow.name }}
        </option>
      </select>
    </div>

    <!-- Executions Table -->
    <div class="overflow-hidden rounded-xl border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead class="bg-neutral-50 dark:bg-neutral-950">
            <tr>
              <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
                Status
              </th>
              <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
                Workflow
              </th>
              <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
                Started
              </th>
              <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
                Duration
              </th>
              <th class="px-4 py-2.5 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
                Mode
              </th>
              <th class="px-4 py-2.5 text-right text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400">
                Actions
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-neutral-100 dark:divide-neutral-800">
            <tr
              v-for="execution in filteredExecutions"
              :key="execution.id"
              class="cursor-pointer transition-colors duration-200 hover:bg-neutral-50 dark:hover:bg-neutral-800/40"
              @click="viewExecution(execution.id)"
            >
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <component
                    :is="getStatusIcon(execution.status)"
                    class="h-4 w-4"
                    :class="getStatusColor(execution.status)"
                  />
                  <StatusBadge
                    :status="getStatusTone(execution.status)"
                    :label="execution.status"
                  />
                </div>
              </td>
              <td class="px-4 py-3">
                <span class="text-sm font-medium text-neutral-950 dark:text-neutral-50">
                  {{ getWorkflowName(execution.workflowId) }}
                </span>
              </td>
              <td class="px-4 py-3">
                <span class="whitespace-nowrap text-xs text-neutral-600 dark:text-neutral-400">
                  {{ formatDate(execution.startedAt) }}
                </span>
              </td>
              <td class="px-4 py-3">
                <span class="whitespace-nowrap text-xs text-neutral-600 dark:text-neutral-400">
                  {{ formatDuration(execution.startedAt, execution.finishedAt) }}
                </span>
              </td>
              <td class="px-4 py-3">
                <StatusBadge
                  status="info"
                  :label="execution.mode"
                />
              </td>
              <td class="px-4 py-3 text-right">
                <div
                  class="flex items-center justify-end gap-1"
                  @click.stop
                >
                  <IconButton
                    v-if="execution.status === 'failed'"
                    label="Retry"
                    @click="retryExecution(execution.id)"
                  >
                    <ArrowPathIcon class="h-4 w-4" />
                  </IconButton>
                  <IconButton
                    label="Delete"
                    tone="danger"
                    @click="deleteExecution(execution.id)"
                  >
                    <TrashIcon class="h-4 w-4" />
                  </IconButton>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Empty State -->
      <div
        v-if="filteredExecutions.length === 0 && !executionStore.loading"
        class="px-4 py-12 text-center"
      >
        <ClockIcon class="mx-auto h-12 w-12 text-neutral-300 dark:text-neutral-700" />
        <h3 class="mt-4 text-lg font-semibold text-neutral-950 dark:text-neutral-50">
          No executions found
        </h3>
        <p class="mt-1 text-sm text-neutral-600 dark:text-neutral-400">
          Run a workflow to see executions here
        </p>
      </div>

      <!-- Loading State -->
      <div
        v-if="executionStore.loading"
        class="px-4 py-12 text-center"
      >
        <span
          class="mx-auto inline-block h-8 w-8 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
          aria-hidden="true"
        />
        <p class="mt-3 text-sm text-neutral-600 dark:text-neutral-400">
          Loading executions…
        </p>
      </div>
    </div>
  </div>
</template>
