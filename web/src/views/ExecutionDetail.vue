<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { VueFlow } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import '@vue-flow/core/dist/style.css'
import {
  ArrowLeftIcon,
  CheckCircleIcon,
  XCircleIcon,
  ClockIcon,
  ArrowPathIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  CodeBracketIcon,
  DocumentTextIcon,
  ExclamationTriangleIcon,
  PlayIcon,
} from '@heroicons/vue/24/outline'
import { useExecutionStore, useWorkflowStore } from '@/stores'
import { buildFlowNodes, buildFlowEdges } from '@/lib/workflowGraph'
import type { Workflow, WorkflowNode, DataItem } from '@/types'

const route = useRoute()
const router = useRouter()
const executionStore = useExecutionStore()
const workflowStore = useWorkflowStore()

const executionId = computed(() => route.params.id as string)
const execution = computed(() => executionStore.currentExecution)
const workflow = computed(() => workflowStore.currentWorkflow)

const selectedNodeName = ref<string | null>(null)
const expandedSections = ref<Set<string>>(
  new Set(['input', 'output'])
)

onMounted(async () => {
  await executionStore.fetchExecution(executionId.value)
  if (executionStore.currentExecution?.workflowId) {
    await workflowStore.fetchWorkflow(executionStore.currentExecution.workflowId)
  }
  // Default to the first node that produced output so the right pane
  // has content even when there is no explicit selection.
  if (!selectedNodeName.value) {
    const firstNode = workflow.value?.nodes?.[0]
    if (firstNode) selectedNodeName.value = firstNode.name
  }
})

// Build flow nodes/edges from the workflow. We color each node by its
// per-execution state so the operator can see at a glance which nodes
// succeeded, failed, or never ran.
type NodeState = 'pending' | 'success' | 'failed' | 'running' | 'skipped'

const nodeStates = computed(() => {
  const map = new Map<string, NodeState>()
  if (!workflow.value || !execution.value) return map
  const nodeData = execution.value.nodeData ?? {}

  // Build adjacency from connections so we can mark downstream as
  // "skipped" when a node fails.
  const incoming = new Map<string, string[]>()
  for (const [src, conns] of Object.entries(workflow.value.connections ?? {})) {
    for (const outputs of conns.main ?? []) {
      for (const c of outputs) {
        if (!incoming.has(c.node)) incoming.set(c.node, [])
        incoming.get(c.node)!.push(src)
      }
    }
  }

  // First pass: assign success/failed/pending based on data presence.
  for (const node of workflow.value.nodes) {
    if (execution.value.status === 'failed') {
      const data = nodeData[node.name]
      if (data === undefined) {
        map.set(node.name, 'pending')
      } else if (
        Array.isArray(data) &&
        data.some((d) => d && (d as any).error !== undefined)
      ) {
        map.set(node.name, 'failed')
      } else {
        map.set(node.name, 'success')
      }
    } else {
      map.set(node.name, nodeData[node.name] !== undefined ? 'success' : 'pending')
    }
  }

  // Cascade-fail: walk downstream from any failed node and mark
  // downstream-of-failed nodes as skipped (n8n does this too).
  if (execution.value.status === 'failed') {
    const visited = new Set<string>()
    const queue: string[] = []
    for (const [name, state] of map.entries()) {
      if (state === 'failed') queue.push(name)
    }
    while (queue.length > 0) {
      const cur = queue.shift()!
      if (visited.has(cur)) continue
      visited.add(cur)
      for (const [otherName, conns] of Object.entries(
        workflow.value.connections ?? {}
      )) {
        if (otherName !== cur) continue
        for (const outputs of conns.main ?? []) {
          for (const c of outputs) {
            if (map.get(c.node) === 'pending') {
              map.set(c.node, 'skipped')
              queue.push(c.node)
            }
          }
        }
      }
    }
  }

  return map
})

// Vue Flow node list with state-coloured borders.
const flowNodes = computed(() => {
  const base = buildFlowNodes(workflow.value as Workflow | null)
  return base.map((n) => {
    const nodeName = (n.data as any).label as string
    const state = nodeStates.value.get(nodeName) ?? 'pending'
    const stateRing: Record<NodeState, string> = {
      pending: 'ring-1 ring-slate-300 dark:ring-slate-600',
      success: 'ring-2 ring-green-500',
      failed: 'ring-2 ring-red-500',
      running: 'ring-2 ring-blue-500 animate-pulse',
      skipped: 'ring-1 ring-slate-300 dark:ring-slate-600 opacity-50',
    }
    return {
      ...n,
      selected: n.id === selectedNodeId.value,
      class: stateRing[state],
    }
  })
})

const flowEdges = computed(() => buildFlowEdges(workflow.value as Workflow | null))

const selectedNodeId = computed<string | null>(() => {
  if (!selectedNodeName.value || !workflow.value) return null
  const node = workflow.value.nodes.find((n) => n.name === selectedNodeName.value)
  return node?.id ?? null
})

const selectedNode = computed<WorkflowNode | null>(() => {
  if (!selectedNodeName.value || !workflow.value) return null
  return (
    workflow.value.nodes.find((n) => n.name === selectedNodeName.value) ??
    null
  )
})

const selectedNodeOutput = computed<DataItem[] | undefined>(() => {
  if (!selectedNodeName.value || !execution.value) return undefined
  return execution.value.nodeData?.[selectedNodeName.value]
})

const selectedNodeError = computed<string | undefined>(() => {
  const data = selectedNodeOutput.value
  if (!data || !Array.isArray(data)) return undefined
  const first = data[0]
  if (first && (first as any).error !== undefined) {
    const e = (first as any).error
    return typeof e === 'string' ? e : JSON.stringify(e)
  }
  return undefined
})

const selectedNodeState = computed<NodeState>(() => {
  if (!selectedNodeName.value) return 'pending'
  return nodeStates.value.get(selectedNodeName.value) ?? 'pending'
})

const formatDate = (dateStr: string) => new Date(dateStr).toLocaleString()

const formatDuration = (start: string, end?: string) => {
  if (!end) return 'Running...'
  const duration = new Date(end).getTime() - new Date(start).getTime()
  if (duration < 1000) return `${duration}ms`
  if (duration < 60000) return `${(duration / 1000).toFixed(2)}s`
  return `${Math.floor(duration / 60000)}m ${Math.floor((duration % 60000) / 1000)}s`
}

const getStatusIcon = (status: string) => {
  switch (status) {
    case 'completed':
      return CheckCircleIcon
    case 'failed':
      return XCircleIcon
    case 'running':
      return ArrowPathIcon
    default:
      return ClockIcon
  }
}

const getStatusColor = (status: string) => {
  switch (status) {
    case 'completed':
      return 'text-green-500 bg-green-100 dark:bg-green-900/30'
    case 'failed':
      return 'text-red-500 bg-red-100 dark:bg-red-900/30'
    case 'running':
      return 'text-blue-500 bg-blue-100 dark:bg-blue-900/30'
    default:
      return 'text-slate-500 bg-slate-100 dark:bg-slate-700'
  }
}

const stateColor = (state: NodeState) => {
  switch (state) {
    case 'success':
      return 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
    case 'failed':
      return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
    case 'running':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
    case 'skipped':
      return 'bg-slate-100 text-slate-500 dark:bg-slate-700 dark:text-slate-400'
    default:
      return 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
  }
}

const retryExecution = async () => {
  if (execution.value) {
    try {
      const newExecution = await executionStore.retryExecution(execution.value.id)
      router.push(`/executions/${newExecution.id}`)
    } catch (e) {
      console.error('Failed to retry execution:', e)
    }
  }
}

const goBack = () => router.push('/executions')

const toggleSection = (name: string) => {
  if (expandedSections.value.has(name)) {
    expandedSections.value.delete(name)
  } else {
    expandedSections.value.add(name)
  }
}

// Click handler for nodes on the canvas.
const onNodeClick = (event: { node: { id: string } }) => {
  const n = workflow.value?.nodes.find((node) => node.id === event.node.id)
  if (n) selectedNodeName.value = n.name
}

// Watch for execution changes (after retry) and reset selection.
watch(executionId, () => {
  selectedNodeName.value = null
})

// Pretty-print JSON with limit (avoids 100kB dumps)
const truncate = (s: string, max = 8000): string =>
  s.length > max ? s.slice(0, max) + '\n…(truncated)' : s

const prettyJson = (data: unknown): string =>
  truncate(JSON.stringify(data, null, 2))
</script>

<template>
  <div class="h-full flex flex-col">
    <!-- Header -->
    <div class="px-6 pt-6 pb-4 border-b border-slate-200 dark:border-slate-700">
      <div class="flex items-center gap-4">
        <button
          @click="goBack"
          class="p-2 rounded-lg text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700"
        >
          <ArrowLeftIcon class="w-5 h-5" />
        </button>
        <div class="flex-1 min-w-0">
          <h1 class="text-xl font-bold text-slate-900 dark:text-white">
            {{ workflow?.name || 'Execution' }}
          </h1>
          <p class="text-sm text-slate-500 dark:text-slate-400">
            <code class="text-xs">{{ execution?.id?.slice(0, 8) }}</code>
            · started {{ execution ? formatDate(execution.startedAt) : '—' }}
          </p>
        </div>
        <div v-if="execution" class="flex items-center gap-3">
          <span
            :class="['inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-sm font-medium', getStatusColor(execution.status)]"
          >
            <component
              :is="getStatusIcon(execution.status)"
              :class="[
                'w-4 h-4',
                execution.status === 'running' ? 'animate-spin' : ''
              ]"
            />
            {{ execution.status }}
          </span>
          <button
            v-if="execution.status === 'failed'"
            @click="retryExecution"
            class="btn-secondary flex items-center gap-2"
          >
            <ArrowPathIcon class="w-4 h-4" />
            Retry
          </button>
        </div>
      </div>

      <!-- Timing row -->
      <div v-if="execution" class="grid grid-cols-4 gap-4 mt-4">
        <div>
          <p class="text-xs text-slate-500 dark:text-slate-400 uppercase tracking-wide">Started</p>
          <p class="text-sm font-medium text-slate-900 dark:text-white">{{ formatDate(execution.startedAt) }}</p>
        </div>
        <div>
          <p class="text-xs text-slate-500 dark:text-slate-400 uppercase tracking-wide">Finished</p>
          <p class="text-sm font-medium text-slate-900 dark:text-white">
            {{ execution.finishedAt ? formatDate(execution.finishedAt) : '—' }}
          </p>
        </div>
        <div>
          <p class="text-xs text-slate-500 dark:text-slate-400 uppercase tracking-wide">Duration</p>
          <p class="text-sm font-medium text-slate-900 dark:text-white">
            {{ formatDuration(execution.startedAt, execution.finishedAt) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-slate-500 dark:text-slate-400 uppercase tracking-wide">Mode</p>
          <p class="text-sm font-medium text-slate-900 dark:text-white capitalize">{{ execution.mode }}</p>
        </div>
      </div>
    </div>

    <!-- Body: workflow canvas (left) + node detail (right) -->
    <div v-if="execution" class="flex-1 flex min-h-0">
      <!-- Left: workflow graph with state-coloured nodes -->
      <div class="flex-1 relative bg-[#f5f5f7] dark:bg-slate-900">
        <VueFlow
          :nodes="flowNodes"
          :edges="flowEdges"
          :default-viewport="{ x: 100, y: 100, zoom: 0.8 }"
          :min-zoom="0.2"
          :max-zoom="2"
          :nodes-draggable="false"
          :nodes-connectable="false"
          :elements-selectable="true"
          @node-click="onNodeClick"
        >
          <Background :pattern-color="'#cbd5e1'" :gap="20" />
        </VueFlow>
        <!-- Legend -->
        <div class="absolute bottom-3 left-3 bg-white/95 dark:bg-slate-800/95 backdrop-blur rounded-lg shadow p-2 flex items-center gap-3 text-xs">
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded-full bg-green-500"></span>
            <span class="text-slate-600 dark:text-slate-300">Success</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded-full bg-red-500"></span>
            <span class="text-slate-600 dark:text-slate-300">Failed</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded-full bg-blue-500"></span>
            <span class="text-slate-600 dark:text-slate-300">Running</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded-full bg-slate-300"></span>
            <span class="text-slate-600 dark:text-slate-300">Pending / Skipped</span>
          </div>
        </div>
      </div>

      <!-- Right: per-node debug detail -->
      <div class="w-[480px] border-l border-slate-200 dark:border-slate-700 flex flex-col bg-white dark:bg-slate-800">
        <div v-if="selectedNode" class="flex-1 overflow-y-auto">
          <!-- Node header -->
          <div class="p-4 border-b border-slate-200 dark:border-slate-700">
            <div class="flex items-start justify-between">
              <div class="min-w-0">
                <p class="text-xs text-slate-500 dark:text-slate-400 uppercase tracking-wide">{{ selectedNode.type }}</p>
                <h2 class="text-lg font-semibold text-slate-900 dark:text-white truncate">
                  {{ selectedNode.name }}
                </h2>
              </div>
              <span
                :class="['inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium capitalize', stateColor(selectedNodeState)]"
              >
                <CheckCircleIcon v-if="selectedNodeState === 'success'" class="w-3 h-3" />
                <XCircleIcon v-else-if="selectedNodeState === 'failed'" class="w-3 h-3" />
                <ArrowPathIcon v-else-if="selectedNodeState === 'running'" class="w-3 h-3 animate-spin" />
                <ExclamationTriangleIcon v-else-if="selectedNodeState === 'skipped'" class="w-3 h-3" />
                <ClockIcon v-else class="w-3 h-3" />
                {{ selectedNodeState }}
              </span>
            </div>
            <div v-if="selectedNode.notes" class="mt-2 text-xs text-slate-500 dark:text-slate-400 italic">
              {{ selectedNode.notes }}
            </div>
          </div>

          <!-- Error section (always visible if a node errored) -->
          <div v-if="selectedNodeError" class="m-4 p-3 rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-900">
            <div class="flex items-center gap-2 mb-1">
              <XCircleIcon class="w-5 h-5 text-red-500" />
              <h3 class="font-semibold text-red-700 dark:text-red-300 text-sm">Error</h3>
            </div>
            <pre class="text-xs text-red-700 dark:text-red-300 whitespace-pre-wrap font-mono">{{ selectedNodeError }}</pre>
          </div>

          <!-- Output -->
          <div class="px-4">
            <button
              @click="toggleSection('output')"
              class="w-full flex items-center gap-2 py-2 text-sm font-medium text-slate-700 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white"
            >
              <ChevronDownIcon v-if="expandedSections.has('output')" class="w-4 h-4" />
              <ChevronRightIcon v-else class="w-4 h-4" />
              <DocumentTextIcon class="w-4 h-4" />
              Output
              <span class="ml-auto text-xs text-slate-400 font-normal">
                {{ selectedNodeOutput ? `${selectedNodeOutput.length} item${selectedNodeOutput.length === 1 ? '' : 's'}` : 'no output' }}
              </span>
            </button>
            <div v-if="expandedSections.has('output')" class="pb-3">
              <pre v-if="selectedNodeOutput" class="text-xs text-slate-600 dark:text-slate-400 whitespace-pre-wrap font-mono bg-slate-50 dark:bg-slate-900 p-3 rounded-lg overflow-auto max-h-80">{{ prettyJson(selectedNodeOutput) }}</pre>
              <p v-else class="text-xs text-slate-500 dark:text-slate-400 italic px-1">No output for this node.</p>
            </div>
          </div>

          <!-- Parameters (node config snapshot for debugging) -->
          <div class="px-4">
            <button
              @click="toggleSection('parameters')"
              class="w-full flex items-center gap-2 py-2 text-sm font-medium text-slate-700 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white"
            >
              <ChevronDownIcon v-if="expandedSections.has('parameters')" class="w-4 h-4" />
              <ChevronRightIcon v-else class="w-4 h-4" />
              <CodeBracketIcon class="w-4 h-4" />
              Parameters
            </button>
            <div v-if="expandedSections.has('parameters')" class="pb-3">
              <pre class="text-xs text-slate-600 dark:text-slate-400 whitespace-pre-wrap font-mono bg-slate-50 dark:bg-slate-900 p-3 rounded-lg overflow-auto max-h-80">{{ prettyJson(selectedNode.parameters) }}</pre>
            </div>
          </div>

          <!-- Credentials snapshot -->
          <div v-if="selectedNode.credentials && Object.keys(selectedNode.credentials).length > 0" class="px-4">
            <div class="py-2 text-sm font-medium text-slate-700 dark:text-slate-300 flex items-center gap-2">
              <DocumentTextIcon class="w-4 h-4" />
              Credentials
            </div>
            <div class="pb-3 space-y-1">
              <div
                v-for="(cred, type) in selectedNode.credentials"
                :key="type"
                class="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-400"
              >
                <span class="font-mono px-1.5 py-0.5 bg-slate-100 dark:bg-slate-700 rounded">{{ type }}</span>
                <span>→</span>
                <span>{{ cred.name }}</span>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="flex-1 flex items-center justify-center text-slate-400 dark:text-slate-500 text-sm">
          <div class="text-center">
            <PlayIcon class="w-8 h-8 mx-auto mb-2 opacity-50" />
            Click a node on the canvas to see its input, output, and parameters.
          </div>
        </div>
      </div>
    </div>

    <!-- Loading State -->
    <div v-else class="flex-1 flex items-center justify-center">
      <div class="text-center">
        <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto" />
        <p class="mt-4 text-slate-500 dark:text-slate-400">Loading execution details...</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.vue-flow__node {
  cursor: pointer;
}

.vue-flow__node.selected {
  outline: 3px solid rgb(99 102 241);
  outline-offset: 2px;
  border-radius: 8px;
}
</style>
