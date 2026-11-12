<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import {
  BoltIcon,
  PlayIcon,
  CheckCircleIcon,
  ClockIcon,
  KeyIcon,
} from '@heroicons/vue/24/outline'
import { useWorkflowStore, useExecutionStore, useCredentialsStore } from '@/stores'
import Button from '@/components/common/Button.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import type { StatusTone } from '@/components/common/StatusBadge.vue'

const router = useRouter()
const workflowStore = useWorkflowStore()
const executionStore = useExecutionStore()
const credentialsStore = useCredentialsStore()

onMounted(async () => {
  await Promise.all([
    workflowStore.fetchWorkflows({ limit: 5 }),
    executionStore.fetchExecutions({ limit: 10 }),
    credentialsStore.fetchCredentials(),
  ])
})

// Count executions that started today (midnight-to-now, local time).
const todayExecutions = computed(() => {
  const start = new Date()
  start.setHours(0, 0, 0, 0)
  const startMs = start.getTime()
  return executionStore.executions.filter(
    (e) => new Date(e.startedAt).getTime() >= startMs
  ).length
})

const failureRate = computed(() => {
  const total = executionStore.executions.length
  if (total === 0) return '0%'
  const failed = executionStore.executionsByStatus.failed.length
  return ((failed / total) * 100).toFixed(1) + '%'
})

const stats = computed(() => [
  {
    name: 'Active Workflows',
    value: workflowStore.activeWorkflows.length,
    subtitle: `${workflowStore.total} total`,
    icon: BoltIcon,
    tone: 'bg-brand-50 text-brand-600 dark:bg-brand-950/50 dark:text-brand-300',
  },
  {
    name: 'Executions Today',
    value: todayExecutions.value,
    subtitle: `${executionStore.executions.length} total`,
    icon: PlayIcon,
    tone: 'bg-blue-50 text-blue-600 dark:bg-blue-950/50 dark:text-blue-300',
  },
  {
    name: 'Failure Rate',
    value: failureRate.value,
    subtitle: `${executionStore.executionsByStatus.completed.length} succeeded`,
    icon: CheckCircleIcon,
    tone: 'bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-300',
  },
  {
    name: 'Credentials',
    value: credentialsStore.count,
    subtitle: `${credentialsStore.credentialTypes.length} types in use`,
    icon: KeyIcon,
    tone: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-950/50 dark:text-yellow-300',
  },
])

const formatDate = (dateStr: string) => {
  const date = new Date(dateStr)
  return date.toLocaleString()
}

const formatDuration = (start: string, end?: string) => {
  if (!end) return 'Running...'
  const duration = new Date(end).getTime() - new Date(start).getTime()
  if (duration < 1000) return `${duration}ms`
  if (duration < 60000) return `${(duration / 1000).toFixed(1)}s`
  return `${Math.floor(duration / 60000)}m ${Math.floor((duration % 60000) / 1000)}s`
}

/** Status family tone of an execution. */
const getStatusTone = (status: string): StatusTone => {
  switch (status) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'running': return 'info'
    case 'cancelled': return 'warning'
    default: return 'info'
  }
}
</script>

<template>
  <div class="space-y-6">
    <!-- Page header -->
    <div>
      <h1 class="text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
        Dashboard
      </h1>
      <p class="mt-1 text-xs text-neutral-600 dark:text-neutral-400">
        An overview of your workflows, executions and credentials
      </p>
    </div>

    <!-- Stats Grid -->
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <div
        v-for="stat in stats"
        :key="stat.name"
        class="rounded-lg border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900"
      >
        <div class="flex items-center gap-4">
          <div
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
            :class="stat.tone"
          >
            <component
              :is="stat.icon"
              class="h-5 w-5"
            />
          </div>
          <div class="min-w-0">
            <p class="text-2xl font-semibold leading-tight text-neutral-950 dark:text-neutral-50">
              {{ stat.value }}
            </p>
            <p class="text-sm font-medium text-neutral-800 dark:text-neutral-200">
              {{ stat.name }}
            </p>
            <p
              v-if="stat.subtitle"
              class="mt-0.5 text-xs text-neutral-500 dark:text-neutral-400"
            >
              {{ stat.subtitle }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <!-- Recent Workflows -->
      <section class="overflow-hidden rounded-lg border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
        <header class="flex items-center justify-between border-b border-neutral-200 px-4 py-3 dark:border-neutral-800">
          <h2 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
            Recent Workflows
          </h2>
          <RouterLink
            to="/workflows"
            class="rounded-sm text-sm font-medium text-brand-600 transition-colors hover:text-brand-700 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500 dark:text-brand-400 dark:hover:text-brand-300"
          >
            View all
          </RouterLink>
        </header>
        <div class="divide-y divide-neutral-100 dark:divide-neutral-800">
          <div
            v-for="workflow in workflowStore.workflows.slice(0, 5)"
            :key="workflow.id"
            class="cursor-pointer px-4 py-3 transition-colors duration-200 hover:bg-neutral-50 dark:hover:bg-neutral-800/50"
            @click="router.push(`/workflows/${workflow.id}`)"
          >
            <div class="flex items-center justify-between gap-3">
              <div class="flex min-w-0 items-center gap-3">
                <span
                  class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
                  :class="workflow.active
                    ? 'bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-400'
                    : 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'"
                >
                  <BoltIcon class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <p class="truncate text-sm font-semibold text-neutral-950 dark:text-neutral-50">
                    {{ workflow.name }}
                  </p>
                  <p class="text-xs text-neutral-500 dark:text-neutral-400">
                    {{ workflow.nodes.length }} nodes
                  </p>
                </div>
              </div>
              <StatusBadge
                :status="workflow.active ? 'success' : 'neutral'"
                :label="workflow.active ? 'Active' : 'Inactive'"
                dot
              />
            </div>
          </div>
          <div
            v-if="workflowStore.workflows.length === 0"
            class="px-6 py-10 text-center"
          >
            <BoltIcon class="mx-auto h-10 w-10 text-neutral-300 dark:text-neutral-700" />
            <p class="mt-2 text-sm text-neutral-600 dark:text-neutral-400">
              No workflows yet
            </p>
            <Button
              variant="secondary"
              size="sm"
              class="mt-4"
              @click="router.push('/workflows/new')"
            >
              Create your first workflow
            </Button>
          </div>
        </div>
      </section>

      <!-- Recent Executions -->
      <section class="overflow-hidden rounded-lg border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
        <header class="flex items-center justify-between border-b border-neutral-200 px-4 py-3 dark:border-neutral-800">
          <h2 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
            Recent Executions
          </h2>
          <RouterLink
            to="/executions"
            class="rounded-sm text-sm font-medium text-brand-600 transition-colors hover:text-brand-700 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500 dark:text-brand-400 dark:hover:text-brand-300"
          >
            View all
          </RouterLink>
        </header>
        <div class="divide-y divide-neutral-100 dark:divide-neutral-800">
          <div
            v-for="execution in executionStore.recentExecutions"
            :key="execution.id"
            class="cursor-pointer px-4 py-3 transition-colors duration-200 hover:bg-neutral-50 dark:hover:bg-neutral-800/50"
            @click="router.push(`/executions/${execution.id}`)"
          >
            <div class="flex items-center justify-between gap-3">
              <div class="flex min-w-0 items-center gap-3">
                <span
                  class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
                  :class="execution.status === 'completed'
                    ? 'bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-400'
                    : execution.status === 'failed'
                      ? 'bg-red-50 text-red-600 dark:bg-red-950/50 dark:text-red-400'
                      : 'bg-blue-50 text-blue-600 dark:bg-blue-950/50 dark:text-blue-400'"
                >
                  <ClockIcon class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <p class="truncate font-mono text-sm font-medium text-neutral-950 dark:text-neutral-50">
                    {{ execution.id.slice(0, 8) }}…
                  </p>
                  <p class="text-xs text-neutral-500 dark:text-neutral-400">
                    {{ formatDate(execution.startedAt) }}
                  </p>
                </div>
              </div>
              <div class="text-right">
                <StatusBadge
                  :status="getStatusTone(execution.status)"
                  :label="execution.status"
                  dot
                />
                <p class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                  {{ formatDuration(execution.startedAt, execution.finishedAt) }}
                </p>
              </div>
            </div>
          </div>
          <div
            v-if="executionStore.executions.length === 0"
            class="px-6 py-10 text-center"
          >
            <ClockIcon class="mx-auto h-10 w-10 text-neutral-300 dark:text-neutral-700" />
            <p class="mt-2 text-sm text-neutral-600 dark:text-neutral-400">
              No executions yet
            </p>
            <p class="mt-1 text-xs text-neutral-500 dark:text-neutral-500">
              Run a workflow to see executions here
            </p>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
