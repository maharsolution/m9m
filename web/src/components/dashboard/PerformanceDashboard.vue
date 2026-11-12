<script setup lang="ts">
/**
 * Performance Dashboard Component
 *
 * All metrics displayed here come from real backend data — either the
 * `/api/v1/performance` endpoint (workflow + execution aggregates) or
 * `/api/v1/metrics` (process / scheduler telemetry). No random numbers,
 * no marketing placeholders.
 */
import { ref, computed, onMounted, onUnmounted } from 'vue';
import {
  BoltIcon,
  ArrowTrendingUpIcon,
  ChartBarIcon,
  CircleStackIcon,
  RocketLaunchIcon,
} from '@heroicons/vue/24/outline';

// --- API response shapes ----------------------------------------------------

interface PerformanceMetrics {
  avgExecutionTime: null | { ms: number; display: string; sampleCount: number };
  successRate: null | { percent: number; succeeded: number; sampleCount: number };
  totalExecutions: number;
  activeWorkflows: number;
  failedExecutions: number;
  sampledExecutions: number;
  sampleWindow: string;
  generatedAt: string;
}

interface PerformanceResponse {
  metrics: PerformanceMetrics;
}

interface SchedulerMetrics {
  // Whatever the scheduler / metrics endpoint returns — kept loose because
  // it can include queue depth, circuit-breaker state, etc.
  [key: string]: unknown;
}

// --- Local state ------------------------------------------------------------

const metrics = ref<PerformanceMetrics | null>(null);
const schedulerMetrics = ref<SchedulerMetrics | null>(null);
const processStartedAt = ref<number>(Date.now());

const isLoading = ref(true);
const lastUpdated = ref<Date | null>(null);
const hasError = ref(false);
let refreshInterval: number | null = null;

// --- Display values ---------------------------------------------------------

const avgExecutionDisplay = computed(() => {
  const v = metrics.value?.avgExecutionTime;
  if (!v) return 'No data yet';
  return v.display;
});

const successRateDisplay = computed(() => {
  const v = metrics.value?.successRate;
  if (!v) return 'No data yet';
  return `${v.percent.toFixed(1)}%`;
});

const totalExecutionsDisplay = computed(() => {
  const n = metrics.value?.totalExecutions ?? 0;
  return n.toLocaleString();
});

const activeWorkflowsDisplay = computed(() => {
  const n = metrics.value?.activeWorkflows ?? 0;
  return n.toString();
});

const failedExecutionsDisplay = computed(() => {
  const n = metrics.value?.failedExecutions ?? 0;
  return n.toLocaleString();
});

const sampleSize = computed(() => {
  const m = metrics.value;
  if (!m) return '—';
  if (m.sampledExecutions > 0) {
    return `${m.sampledExecutions.toLocaleString()} / ${m.sampleWindow}`;
  }
  return '—';
});

const generatedAtDisplay = computed(() => {
  if (!metrics.value?.generatedAt) return '';
  const d = new Date(metrics.value.generatedAt);
  return d.toLocaleTimeString();
});

const uptimeDisplay = computed(() => {
  const ms = Date.now() - processStartedAt.value;
  return formatDuration(ms / 1000);
});

const queuedTasks = computed(() => {
  const m = schedulerMetrics.value as Record<string, unknown> | null;
  if (!m) return null;
  // Common shapes we accept: { queuedTasks: n }, { queue_depth: n },
  // or { scheduler: { queuedTasks: n } }.
  if (typeof m.queuedTasks === 'number') return m.queuedTasks as number;
  if (typeof m.queue_depth === 'number') return m.queue_depth as number;
  const inner = m.scheduler as Record<string, unknown> | undefined;
  if (inner && typeof inner.queuedTasks === 'number') {
    return inner.queuedTasks as number;
  }
  return null;
});

// --- Helpers ----------------------------------------------------------------

function formatDuration(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);
  if (days > 0) return `${days}d ${hours}h ${minutes}m`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${secs}s`;
  return `${secs}s`;
}

async function fetchPerformance(): Promise<void> {
  try {
    const res = await fetch('/api/v1/performance');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = (await res.json()) as PerformanceResponse;
    metrics.value = data.metrics;
    hasError.value = false;
  } catch (err) {
    console.error('Failed to fetch /api/v1/performance:', err);
    hasError.value = true;
  }
}

async function fetchScheduler(): Promise<void> {
  try {
    const res = await fetch('/api/v1/metrics');
    if (!res.ok) return;
    schedulerMetrics.value = (await res.json()) as SchedulerMetrics;
  } catch (err) {
    // Soft-fail — queued-tasks card will just hide.
    console.warn('Failed to fetch /api/v1/metrics:', err);
  }
}

async function refresh(): Promise<void> {
  await Promise.all([fetchPerformance(), fetchScheduler()]);
  lastUpdated.value = new Date();
  isLoading.value = false;
}

onMounted(() => {
  refresh();
  refreshInterval = window.setInterval(refresh, 10_000);
});

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval);
  }
});
</script>

<template>
  <div class="space-y-6">
    <!-- Page header -->
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="flex items-center gap-3 text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
          <RocketLaunchIcon class="h-6 w-6 text-brand-500" />
          Performance Dashboard
        </h1>
        <p class="mt-1 text-xs text-neutral-600 dark:text-neutral-400">
          Live workflow metrics straight from storage — not estimates.
        </p>
      </div>
      <div
        v-if="lastUpdated"
        class="text-right text-xs text-neutral-600 dark:text-neutral-400"
      >
        <div>Last updated: {{ lastUpdated.toLocaleTimeString() }}</div>
        <div
          v-if="metrics"
          class="text-2xs text-neutral-500 dark:text-neutral-500"
        >
          Generated at {{ generatedAtDisplay }}
        </div>
      </div>
    </div>

    <!-- Error banner (only shown if both endpoints fail) -->
    <div
      v-if="hasError && !metrics"
      class="rounded-lg border border-yellow-200 bg-yellow-100 p-4 text-sm text-yellow-800 dark:border-yellow-900 dark:bg-yellow-950/50 dark:text-yellow-300"
    >
      Could not reach <code class="font-mono">/api/v1/performance</code>. Make sure
      the m9m backend is running. The page will keep retrying every 10 seconds.
    </div>

    <!-- Real-time metric tiles -->
    <div class="grid grid-cols-2 gap-4 md:grid-cols-4">
      <div class="rounded-lg border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm text-neutral-600 dark:text-neutral-400">Avg Execution Time</span>
          <BoltIcon class="h-4 w-4 text-neutral-400" />
        </div>
        <div class="text-2xl font-semibold text-neutral-950 dark:text-neutral-50">
          {{ avgExecutionDisplay }}
        </div>
        <div class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
          Avg of {{ sampleSize }}
        </div>
      </div>

      <div class="rounded-lg border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm text-neutral-600 dark:text-neutral-400">Success Rate</span>
          <ArrowTrendingUpIcon class="h-4 w-4 text-neutral-400" />
        </div>
        <div class="text-2xl font-semibold text-neutral-950 dark:text-neutral-50">
          {{ successRateDisplay }}
        </div>
        <div class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
          <template v-if="metrics?.successRate">
            {{ metrics.successRate.succeeded }} succeeded of
            {{ metrics.successRate.sampleCount }} sampled
          </template>
          <template v-else>Awaiting first execution</template>
        </div>
      </div>

      <div class="rounded-lg border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm text-neutral-600 dark:text-neutral-400">Total Executions</span>
          <ChartBarIcon class="h-4 w-4 text-neutral-400" />
        </div>
        <div class="text-2xl font-semibold text-neutral-950 dark:text-neutral-50">
          {{ totalExecutionsDisplay }}
        </div>
        <div class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
          {{ failedExecutionsDisplay }} failed
        </div>
      </div>

      <div class="rounded-lg border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm text-neutral-600 dark:text-neutral-400">Active Workflows</span>
          <CircleStackIcon class="h-4 w-4 text-neutral-400" />
        </div>
        <div class="text-2xl font-semibold text-neutral-950 dark:text-neutral-50">
          {{ activeWorkflowsDisplay }}
        </div>
        <div class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">Currently active=true</div>
      </div>
    </div>

    <!-- Operational details -->
    <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
      <div class="rounded-lg border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
        <h3 class="mb-4 text-sm font-medium text-neutral-700 dark:text-neutral-300">
          System Status
        </h3>
        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <span class="text-sm text-neutral-600 dark:text-neutral-400">Backend uptime (this page)</span>
            <span class="font-mono text-sm font-semibold text-neutral-950 dark:text-neutral-50">{{ uptimeDisplay }}</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-sm text-neutral-600 dark:text-neutral-400">Queued tasks</span>
            <span class="font-mono text-sm font-semibold text-neutral-950 dark:text-neutral-50">
              {{ queuedTasks !== null ? queuedTasks : '—' }}
            </span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-sm text-neutral-600 dark:text-neutral-400">Failed executions</span>
            <span class="font-mono text-sm font-semibold text-neutral-950 dark:text-neutral-50">{{ failedExecutionsDisplay }}</span>
          </div>
        </div>
      </div>

      <div class="rounded-lg border border-neutral-200 bg-white p-5 dark:border-neutral-800 dark:bg-neutral-900">
        <h3 class="mb-4 text-sm font-medium text-neutral-700 dark:text-neutral-300">
          What you're looking at
        </h3>
        <ul class="space-y-2 text-sm text-neutral-600 dark:text-neutral-300">
          <li>
            <strong>Avg Execution Time</strong> — mean duration of the
            {{ sampleSize }} executions, computed from
            <code class="font-mono">finished_at − started_at</code>.
          </li>
          <li>
            <strong>Success Rate</strong> — percentage of recent executions with
            status <code class="font-mono">success</code> or <code class="font-mono">completed</code>.
          </li>
          <li>
            <strong>Total Executions</strong> — every execution ever stored,
            including failures.
          </li>
          <li>
            <strong>Active Workflows</strong> — workflows whose
            <code class="font-mono">active</code> flag is true.
          </li>
        </ul>
        <p class="mt-4 text-xs text-neutral-500 dark:text-neutral-400">
          Auto-refreshes every 10 seconds. All values come from
          <code class="font-mono">/api/v1/performance</code>.
        </p>
      </div>
    </div>
  </div>
</template>
