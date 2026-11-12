<script setup lang="ts">
/**
 * Execution Output Component
 * Displays workflow execution results with node-by-node data inspection
 */
import { computed, ref } from 'vue';
import {
  ChevronRightIcon,
  ChevronDownIcon,
  DocumentTextIcon,
  PhotoIcon,
  ClipboardDocumentIcon,
  CheckIcon,
  ExclamationCircleIcon,
  ClockIcon,
  PlayCircleIcon,
} from '@heroicons/vue/24/outline';
import type { WorkflowExecution, DataItem } from '@/types/workflow';

const props = defineProps<{
  execution: WorkflowExecution;
}>();

// Expanded nodes
const expandedNodes = ref<Set<string>>(new Set());

// Selected tab per node
const selectedTabs = ref<Record<string, 'json' | 'table' | 'binary'>>({});

// Copy success state
const copiedNodes = ref<Set<string>>(new Set());

// Get node execution data
const nodeResults = computed(() => {
  if (!props.execution.data) return [];

  const results: Array<{
    nodeId: string;
    nodeName: string;
    status: 'success' | 'error' | 'running';
    data: DataItem[];
    error?: string;
    startTime?: Date;
    endTime?: Date;
    executionTime?: number;
  }> = [];

  // Parse execution data - adapt based on actual data structure
  const executionData = props.execution.data;

  if (Array.isArray(executionData)) {
    // Simple array of results
    results.push({
      nodeId: 'output',
      nodeName: 'Output',
      status: 'success',
      data: executionData,
    });
  } else if (typeof executionData === 'object' && executionData !== null) {
    // Object with node-keyed results
    for (const [nodeId, nodeData] of Object.entries(executionData)) {
      const data = nodeData as { data?: DataItem[]; error?: string };
      results.push({
        nodeId,
        nodeName: nodeId,
        status: data.error ? 'error' : 'success',
        data: data.data || [],
        error: data.error,
      });
    }
  }

  return results;
});

// Status icon
function getStatusIcon(status: string) {
  switch (status) {
    case 'success':
    case 'completed':
      return CheckIcon;
    case 'error':
    case 'failed':
      return ExclamationCircleIcon;
    case 'running':
      return PlayCircleIcon;
    default:
      return ClockIcon;
  }
}

// Status colors
function getStatusColor(status: string) {
  switch (status) {
    case 'success':
    case 'completed':
      return 'text-green-600 dark:text-green-400';
    case 'error':
    case 'failed':
      return 'text-red-600 dark:text-red-400';
    case 'running':
      return 'text-blue-600 dark:text-blue-400';
    default:
      return 'text-neutral-500 dark:text-neutral-400';
  }
}

// Toggle node expansion
function toggleNode(nodeId: string) {
  if (expandedNodes.value.has(nodeId)) {
    expandedNodes.value.delete(nodeId);
  } else {
    expandedNodes.value.add(nodeId);
  }
}

// Get selected tab
function getSelectedTab(nodeId: string): 'json' | 'table' | 'binary' {
  return selectedTabs.value[nodeId] || 'json';
}

// Set selected tab
function setSelectedTab(nodeId: string, tab: 'json' | 'table' | 'binary') {
  selectedTabs.value[nodeId] = tab;
}

// Format JSON for display
function formatJSON(data: unknown): string {
  try {
    return JSON.stringify(data, null, 2);
  } catch {
    return String(data);
  }
}

// Copy data to clipboard
async function copyToClipboard(nodeId: string, data: unknown) {
  try {
    await navigator.clipboard.writeText(formatJSON(data));
    copiedNodes.value.add(nodeId);
    setTimeout(() => {
      copiedNodes.value.delete(nodeId);
    }, 2000);
  } catch (err) {
    console.error('Failed to copy:', err);
  }
}

// Check if data has binary content
function hasBinaryData(items: DataItem[]): boolean {
  return items.some((item) => item.binary && Object.keys(item.binary).length > 0);
}

function getItemJson(item: DataItem): Record<string, unknown> | null {
  const normalized = item.json ?? (item as DataItem & { JSON?: Record<string, unknown> }).JSON;
  return normalized && typeof normalized === 'object' ? normalized : null;
}

// Get table columns from data
function getTableColumns(items: DataItem[]): string[] {
  const columns = new Set<string>();
  for (const item of items) {
    const jsonValue = getItemJson(item);
    if (!jsonValue) {
      continue;
    }
    for (const key of Object.keys(jsonValue)) {
      columns.add(key);
    }
  }
  return Array.from(columns);
}

// Format cell value
function formatCellValue(value: unknown): string {
  if (value === null || value === undefined) return '-';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

// Format execution time
function formatDuration(ms?: number): string {
  if (!ms) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function formatBinarySize(fileSize?: string): string {
  const bytes = Number(fileSize);
  if (!Number.isFinite(bytes) || bytes <= 0) return 'Unknown size';
  return `${(bytes / 1024).toFixed(1)} KB`;
}
</script>

<template>
  <div class="execution-output">
    <!-- Header -->
    <div class="mb-4 border-b border-neutral-200 pb-4 dark:border-neutral-800">
      <div class="flex items-center justify-between">
        <h3 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
          Execution Output
        </h3>
        <div class="flex items-center gap-2 text-sm text-neutral-500 dark:text-neutral-400">
          <component
            :is="getStatusIcon(execution.status)"
            class="h-5 w-5"
            :class="getStatusColor(execution.status)"
          />
          <span class="capitalize">{{ execution.status }}</span>
        </div>
      </div>
      <p class="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
        Started: {{ new Date(execution.startedAt).toLocaleString() }}
        <template v-if="execution.finishedAt">
          | Duration: {{ formatDuration(new Date(execution.finishedAt).getTime() - new Date(execution.startedAt).getTime()) }}
        </template>
      </p>
    </div>

    <!-- No results -->
    <div
      v-if="nodeResults.length === 0"
      class="px-4 py-8 text-center text-neutral-500 dark:text-neutral-400"
    >
      <DocumentTextIcon class="mx-auto mb-3 h-12 w-12 opacity-50" />
      <p class="text-sm">No execution data available</p>
    </div>

    <!-- Node results -->
    <div
      v-else
      class="space-y-3"
    >
      <div
        v-for="node in nodeResults"
        :key="node.nodeId"
        class="overflow-hidden rounded-lg border border-neutral-200 dark:border-neutral-800"
      >
        <!-- Node header -->
        <button
          type="button"
          class="flex w-full items-center justify-between bg-neutral-50 px-4 py-3 text-left transition-colors duration-200 hover:bg-neutral-100 dark:bg-neutral-950 dark:hover:bg-neutral-900"
          @click="toggleNode(node.nodeId)"
        >
          <div class="flex items-center gap-3">
            <component
              :is="expandedNodes.has(node.nodeId) ? ChevronDownIcon : ChevronRightIcon"
              class="h-5 w-5 text-neutral-400"
            />
            <component
              :is="getStatusIcon(node.status)"
              class="h-5 w-5"
              :class="getStatusColor(node.status)"
            />
            <span class="font-medium text-neutral-950 dark:text-neutral-50">
              {{ node.nodeName }}
            </span>
          </div>
          <div class="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
            <span>{{ node.data.length }} items</span>
            <span v-if="node.executionTime">{{ formatDuration(node.executionTime) }}</span>
          </div>
        </button>

        <!-- Node content (expanded) -->
        <div
          v-if="expandedNodes.has(node.nodeId)"
          class="border-t border-neutral-200 dark:border-neutral-800"
        >
          <!-- Error message -->
          <div
            v-if="node.error"
            class="border-b border-red-200 bg-red-50 p-4 dark:border-red-900 dark:bg-red-950/50"
          >
            <p class="font-mono text-xs text-red-800 dark:text-red-300">
              {{ node.error }}
            </p>
          </div>

          <!-- Tabs -->
          <div
            v-if="node.data.length > 0"
            class="border-b border-neutral-200 dark:border-neutral-800"
          >
            <div class="flex gap-1 p-2">
              <button
                type="button"
                class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors duration-200"
                :class="getSelectedTab(node.nodeId) === 'json'
                  ? 'bg-brand-50 text-brand-700 dark:bg-brand-950/50 dark:text-brand-300'
                  : 'text-neutral-600 hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800'"
                @click="setSelectedTab(node.nodeId, 'json')"
              >
                JSON
              </button>
              <button
                type="button"
                class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors duration-200"
                :class="getSelectedTab(node.nodeId) === 'table'
                  ? 'bg-brand-50 text-brand-700 dark:bg-brand-950/50 dark:text-brand-300'
                  : 'text-neutral-600 hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800'"
                @click="setSelectedTab(node.nodeId, 'table')"
              >
                Table
              </button>
              <button
                v-if="hasBinaryData(node.data)"
                type="button"
                class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors duration-200"
                :class="getSelectedTab(node.nodeId) === 'binary'
                  ? 'bg-brand-50 text-brand-700 dark:bg-brand-950/50 dark:text-brand-300'
                  : 'text-neutral-600 hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800'"
                @click="setSelectedTab(node.nodeId, 'binary')"
              >
                Binary
              </button>

              <!-- Copy button -->
              <button
                type="button"
                class="ml-auto flex items-center gap-1 rounded-md px-3 py-1.5 text-sm font-medium text-neutral-600 transition-colors duration-200 hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800"
                @click="copyToClipboard(node.nodeId, node.data)"
              >
                <component
                  :is="copiedNodes.has(node.nodeId) ? CheckIcon : ClipboardDocumentIcon"
                  class="h-4 w-4"
                />
                {{ copiedNodes.has(node.nodeId) ? 'Copied!' : 'Copy' }}
              </button>
            </div>
          </div>

          <!-- JSON view -->
          <div
            v-if="getSelectedTab(node.nodeId) === 'json' && node.data.length > 0"
            class="max-h-96 overflow-auto p-4"
          >
            <pre class="whitespace-pre-wrap font-mono text-xs text-neutral-700 dark:text-neutral-300">{{ formatJSON(node.data) }}</pre>
          </div>

          <!-- Table view -->
          <div
            v-else-if="getSelectedTab(node.nodeId) === 'table' && node.data.length > 0"
            class="overflow-x-auto"
          >
            <table class="min-w-full divide-y divide-neutral-200 dark:divide-neutral-800">
              <thead class="bg-neutral-50 dark:bg-neutral-950">
                <tr>
                  <th
                    v-for="col in getTableColumns(node.data)"
                    :key="col"
                    class="px-4 py-2 text-left text-2xs font-medium uppercase tracking-wide text-neutral-600 dark:text-neutral-400"
                  >
                    {{ col }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-neutral-200 bg-white dark:divide-neutral-800 dark:bg-neutral-900">
                <tr
                  v-for="(item, idx) in node.data"
                  :key="idx"
                >
                  <td
                    v-for="col in getTableColumns(node.data)"
                    :key="col"
                    class="whitespace-nowrap px-4 py-2 font-mono text-xs text-neutral-950 dark:text-neutral-100"
                  >
                    {{ formatCellValue(getItemJson(item)?.[col]) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Binary view -->
          <div
            v-else-if="getSelectedTab(node.nodeId) === 'binary'"
            class="p-4"
          >
            <div
              v-for="(item, idx) in node.data"
              :key="idx"
              class="mb-4"
            >
              <template v-if="item.binary">
                <div
                  v-for="(binary, key) in item.binary"
                  :key="key"
                  class="flex items-center gap-3 rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-neutral-800 dark:bg-neutral-950"
                >
                  <PhotoIcon class="h-8 w-8 text-neutral-400" />
                  <div>
                    <p class="font-medium text-neutral-950 dark:text-neutral-50">{{ key }}</p>
                    <p class="text-xs text-neutral-500 dark:text-neutral-400">
                      {{ binary.mimeType }} | {{ formatBinarySize(binary.fileSize) }}
                    </p>
                  </div>
                </div>
              </template>
            </div>
          </div>

          <!-- Empty data -->
          <div
            v-if="node.data.length === 0 && !node.error"
            class="px-4 py-8 text-center text-neutral-500 dark:text-neutral-400"
          >
            <p class="text-sm">No output data</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
pre {
  background: transparent;
}
</style>
