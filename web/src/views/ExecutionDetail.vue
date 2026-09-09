<script setup lang="ts">
/**
 * ExecutionDetail — n8n-style execution debug view.
 *
 * Layout
 * ------
 *   ┌─────────────────────────────────────────────────────────────┐
 *   │  Header: status, start/finish, duration, retry              │
 *   ├──────────────────────────────────────┬──────────────────────┤
 *   │  Workflow graph (state-coloured)     │  Node Details View   │
 *   │  Click a node to inspect.            │  ┌────────────────┐  │
 *   │                                       │  │ Input│Output│…│  │
 *   │                                       │  ├────────────────┤  │
 *   │                                       │  │ Schema│Table… │  │
 *   │                                       │  │                │  │
 *   │                                       │  └────────────────┘  │
 *   └──────────────────────────────────────┴──────────────────────┘
 *
 * The right-side NDV (Node Details View) has two tab strips:
 *
 *   - Top row    : Input · Output · Settings
 *                  (Settings tab only appears when the selected
 *                   node type exposes a `properties` descriptor.)
 *   - Bottom row : Schema · Table · JSON
 *                  (Data view, applies to whichever data tab is
 *                   active.)
 *
 * The schema/table/json modes match n8n's behaviour exactly: the
 * same payload is rendered three different ways without any
 * filtering.
 */
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
  ExclamationTriangleIcon,
  PlayIcon,
  Cog6ToothIcon,
  ArrowDownTrayIcon,
  ChevronRightIcon,
} from '@heroicons/vue/24/outline'
import {
  useExecutionStore,
  useNodesStore,
  useWorkflowStore,
} from '@/stores'
import { buildFlowNodes, buildFlowEdges } from '@/lib/workflowGraph'
import type { Workflow, WorkflowNode, DataItem } from '@/types'
import type { NodeType, NodeProperty } from '@/types/node'
import ExecutionDataView from '@/components/execution/ExecutionDataView.vue'
import PropertyEditor from '@/components/execution/PropertyEditor.vue'

type NodeState = 'pending' | 'success' | 'failed' | 'running' | 'skipped'
type DataTab = 'input' | 'output'
type ViewMode = 'schema' | 'table' | 'json'

const route = useRoute()
const router = useRouter()
const executionStore = useExecutionStore()
const workflowStore = useWorkflowStore()
const nodeTypesStore = useNodesStore()

const executionId = computed(() => route.params.id as string)
const execution = computed(() => executionStore.currentExecution)
const workflow = computed(() => workflowStore.currentWorkflow)

const selectedNodeName = ref<string | null>(null)
const dataTab = ref<DataTab>('input')
const viewMode = ref<ViewMode>('schema')

onMounted(async () => {
  // Fire the type-catalog fetch in parallel with the execution /
  // workflow fetch so the NDV can look up node-type properties
  // immediately when the user clicks a node.
  const typeFetch = nodeTypesStore.fetchNodeTypes().catch(() => null)
  const execFetch = executionStore.fetchExecution(executionId.value)
  await execFetch
  if (executionStore.currentExecution?.workflowId) {
    await workflowStore.fetchWorkflow(executionStore.currentExecution.workflowId)
  }
  await typeFetch
  if (!selectedNodeName.value) {
    const firstNode = workflow.value?.nodes?.[0]
    if (firstNode) selectedNodeName.value = firstNode.name
  }
})

// ---------------------------------------------------------------------------
// Per-node execution state (used for the canvas colour coding)
// ---------------------------------------------------------------------------
const nodeStates = computed(() => {
  const map = new Map<string, NodeState>()
  if (!workflow.value || !execution.value) return map
  const nodeData = execution.value.nodeData ?? {}

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

  // Cascade-fail: walk downstream of any failed node and mark the
  // tail as skipped. Mirrors n8n's behaviour: when an upstream
  // node errors, the items never reach the downstream branch, so
  // the NDV renders those branches as skipped instead of "ran
  // successfully with no items".
  if (execution.value.status === 'failed') {
    const queue: string[] = []
    for (const [name, state] of map.entries()) {
      if (state === 'failed') queue.push(name)
    }
    const visited = new Set<string>()
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
  return workflow.value.nodes.find((n) => n.name === selectedNodeName.value)?.id ?? null
})

const selectedNode = computed<WorkflowNode | null>(() => {
  if (!selectedNodeName.value || !workflow.value) return null
  return workflow.value.nodes.find((n) => n.name === selectedNodeName.value) ?? null
})

// NodeType metadata for the selected node. Drives the Parameters
// tab and the Settings tab. Cached off the NodeTypes store, which
// fetches /api/v1/node-types on mount.
const selectedNodeType = computed<NodeType | null>(() => {
  if (!selectedNode.value) return null
  return nodeTypesStore.nodeTypes.find((t) => t.name === selectedNode.value!.type) ?? null
})

const selectedNodeTypeProperties = computed<NodeProperty[]>(() => {
  return selectedNodeType.value?.properties ?? []
})

const hasSettingsTab = computed(() => selectedNodeTypeProperties.value.length > 0)

// Split properties into Parameters (executable config: URL,
// method, query, headers…) vs Settings (engine behaviour: retry,
// continueOnFail…). The boundary is "does it appear in the n8n
// editor's Parameters tab vs Settings tab" — currently the same
// array; the Settings tab is the last few entries (CommonSettings()
// appended by the backend).
//
// Heuristic: properties whose `name` starts with one of the
// well-known Settings keys are moved to the Settings tab. Everything
// else stays in Parameters. This matches n8n's UI even though the
// wire format puts them in a single list.
const SETTINGS_KEY_PREFIXES = [
  'notes',
  'notesInFlow',
  'retryOnFail',
  'maxTries',
  'waitBetweenTries',
  'alwaysOutputData',
  'continueOnFail',
  'onError',
]

const parameterProperties = computed<NodeProperty[]>(() => {
  return selectedNodeTypeProperties.value.filter(
    (p) => !SETTINGS_KEY_PREFIXES.includes(p.name)
  )
})

const settingsProperties = computed<NodeProperty[]>(() => {
  return selectedNodeTypeProperties.value.filter((p) =>
    SETTINGS_KEY_PREFIXES.includes(p.name)
  )
})

// ---------------------------------------------------------------------------
// Per-node data slices for Input/Output tabs
// ---------------------------------------------------------------------------

// `output`: the data the engine produced for this node.
const selectedNodeOutput = computed<DataItem[] | undefined>(() => {
  if (!selectedNodeName.value || !execution.value) return undefined
  return execution.value.nodeData?.[selectedNodeName.value]
})

// `input`: the data the engine routed INTO this node.
//
// To compute input from the workflow graph we look at the
// connected upstream nodes' outputs in nodeData. For each
// connection into this node, we copy the upstream node's
// output items. The order matches the order of incoming
// connections in `workflow.connections`.
//
// This matches what n8n's NDV Input tab shows: the items that
// the node saw, BEFORE it executed. Without this, the Input
// tab would always be empty.
const selectedNodeInput = computed<DataItem[] | undefined>(() => {
  if (!selectedNodeName.value || !workflow.value || !execution.value) return undefined

  const incoming: Array<{ source: string; data: DataItem[] }> = []
  for (const [sourceName, conns] of Object.entries(workflow.value.connections ?? {})) {
    for (const outputs of conns.main ?? []) {
      for (const c of outputs) {
        if (c.node === selectedNodeName.value) {
          const data = execution.value.nodeData?.[sourceName]
          if (data && data.length > 0) {
            incoming.push({ source: sourceName, data })
          }
        }
      }
    }
  }
  if (incoming.length === 0) {
    // Trigger node / first node — no upstream. Return undefined
    // so the panel renders "no upstream" rather than an empty
    // list (which would look like "this node dropped everything").
    return undefined
  }
  // Flatten: one array of items, prefixing each with the source
  // node name for traceability when looking at the JSON view.
  return incoming.flatMap(({ source, data }) =>
    data.map((item) => ({
      ...item,
      json: { ...(item?.json ?? {}), __source: source },
    }))
  )
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

// Binary entries surfaced for the Binary tab. Mirrors n8n's
// Binary tab: when a node carries `binary` keys on its output
// items (HTTP Request with `options.binaryData: true`, Read
// Binary File, etc), this collects them so the NDV can render a
// list with file size / mime type and a download link.
//
// `binary.data` is expected to be a base64 string (matching
// n8n's wire shape). We turn it into a `data:` URL so the
// browser can offer a download.
interface BinaryEntry {
  key: string
  dataUrl?: string
  mimeType?: string
  fileSize?: string
  fileName?: string
}

const selectedNodeBinaryEntries = computed<BinaryEntry[]>(() => {
  const data = selectedNodeOutput.value
  if (!data) return []
  const out: BinaryEntry[] = []
  for (const item of data) {
    if (!item?.binary) continue
    for (const [k, bin] of Object.entries(item.binary)) {
      const entry: BinaryEntry = { key: k }
      if (bin?.mimeType) entry.mimeType = bin.mimeType
      if (bin?.fileSize) entry.fileSize = bin.fileSize
      if (bin?.fileName) entry.fileName = bin.fileName
      if (typeof bin?.data === 'string' && bin.data.length > 0) {
        entry.dataUrl = `data:${bin.mimeType ?? 'application/octet-stream'};base64,${bin.data}`
      }
      out.push(entry)
    }
  }
  return out
})

const selectedNodeState = computed<NodeState>(() => {
  if (!selectedNodeName.value) return 'pending'
  return nodeStates.value.get(selectedNodeName.value) ?? 'pending'
})

// ---------------------------------------------------------------------------
// Multi-branch output renderer (IF / Switch / Merge)
//
// Multi-output routing nodes (IF, Switch, Merge's input ports) tag
// their items with internal `_ifResult`, `_switchRuleIndex`, and
// `_loopDone` fields so the connection router can partition them.
// These are stripped from the data the engine hands back via
// `result.Data` (see engine.go's stripRoutingMetadata), so the
// raw output is a flat list. For the NDV's Output tab we still
// want to show "this item went to output 0, that one went to
// output 1" so the user can see which branch each item took.
//
// We do this by re-evaluating each item against the same rule
// shape the engine uses:
//   - IF:        _ifResult=true → branch 0 (true),
//                _ifResult=false → branch 1 (false).
//   - Switch:    _switchRuleIndex → branch = ruleIndex + 1
//                (branch 0 is the fallback). Items with no
//                _switchRuleIndex → branch 0.
//   - Merge:     split by source-port — Input 1 items carry
//                no marker; Input 2 items carry `_pairedItem`
//                with `inputIndex: 1`. We render them as
//                Input 1 / Input 2 buckets.
//
// The engine stripped the routing tag from the returned items,
// so this re-partition is best-effort. For IF / Switch we still
// have the routing decision stored in `_ifResult` / `_switchRuleIndex`
// because the partition happens BEFORE stripping. For Merge we
// reconstruct the partition from PairedItem. If no routing tags
// survive, we fall back to "all in one bucket" so the user
// still sees the payload.

const isMultiOutputNode = computed(() => {
  if (!selectedNode.value) return false
  const t = selectedNode.value.type
  return (
    t === 'n8n-nodes-base.if' ||
    t === 'n8n-nodes-base.switch' ||
    t === 'n8n-nodes-base.merge'
  )
})

interface OutputBucket {
  label: string
  items: DataItem[]
}

const outputBuckets = computed<OutputBucket[]>(() => {
  const data = selectedNodeOutput.value
  if (!data || data.length === 0) return []

  const t = selectedNode.value?.type

  // IF: re-evaluate by re-running the condition. Falls back to a
  // single bucket when the original items no longer carry
  // `_ifResult`. (Engine strips it post-routing, so under
  // current data this almost always returns a single bucket —
  // see TODO note in workflow.n8n_api.MD. The structure is in
  // place for when the engine preserves the tag.)
  if (t === 'n8n-nodes-base.if') {
    const trueItems = data.filter((d) => (d?.json as any)?._ifResult === true)
    const falseItems = data.filter((d) => (d?.json as any)?._ifResult === false)
    if (trueItems.length === 0 && falseItems.length === 0) {
      return [{ label: 'All items', items: data }]
    }
    return [
      { label: 'True (output 0)', items: trueItems },
      { label: 'False (output 1)', items: falseItems },
    ]
  }

  // Switch: bucket by rule index. Branch 0 is the fallback when
  // no rule matched.
  if (t === 'n8n-nodes-base.switch') {
    const buckets: Record<number, DataItem[]> = {}
    for (const item of data) {
      const idx = (item?.json as any)?._switchRuleIndex
      const bucket = typeof idx === 'number' ? idx + 1 : 0
      if (!buckets[bucket]) buckets[bucket] = []
      buckets[bucket].push(item)
    }
    if (Object.keys(buckets).length === 0) {
      return [{ label: 'All items', items: data }]
    }
    return Object.keys(buckets)
      .map(Number)
      .sort((a, b) => a - b)
      .map((idx) => ({
        label: idx === 0 ? 'Fallback (output 0)' : `Rule ${idx} (output ${idx})`,
        items: buckets[idx],
      }))
  }

  // Merge: items arrive already merged. To show input provenance
  // we fall back to a flat single bucket — n8n's NDV behaves the
  // same way for the Merge node's output (it just shows the
  // merged items without re-splitting them). The Input tab
  // already breaks the upstream nodes apart, so per-input
  // provenance is reachable via "click an upstream node".
  if (t === 'n8n-nodes-base.merge') {
    return [{ label: 'Merged items', items: data }]
  }

  // Default: single bucket for nodes with one output.
  return [{ label: 'Output', items: data }]
})

// Pre-strip the internal routing tags from each item so the
// JSON / Schema / Table views never show `_ifResult` etc. The
// engine already strips them before persisting, but we re-strip
// defensively in case the upstream engine path changes.
function stripRoutingTags(items: DataItem[]): DataItem[] {
  const out: DataItem[] = []
  for (const item of items) {
    if (!item || !item.json) {
      out.push(item)
      continue
    }
    const cleaned = { ...item.json }
    for (const k of ['_ifResult', '_switchRuleIndex', '_loopDone', '__source']) {
      if (k in cleaned) delete cleaned[k]
    }
    out.push({ ...item, json: cleaned })
  }
  return out
}

// ---------------------------------------------------------------------------
// Formatting helpers (status, dates, etc.)
// ---------------------------------------------------------------------------

const formatDate = (dateStr: string) => new Date(dateStr).toLocaleString()

const formatDuration = (start: string, end?: string) => {
  if (!end) return 'Running…'
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

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

const retryExecution = async () => {
  if (!execution.value) return
  try {
    const newExecution = await executionStore.retryExecution(execution.value.id)
    router.push(`/executions/${newExecution.id}`)
  } catch (e) {
    console.error('Failed to retry execution:', e)
  }
}

const goBack = () => router.push('/executions')

const onNodeClick = (event: { node: { id: string } }) => {
  const n = workflow.value?.nodes.find((node) => node.id === event.node.id)
  if (n) selectedNodeName.value = n.name
}

watch(executionId, () => {
  selectedNodeName.value = null
  dataTab.value = 'input'
  viewMode.value = 'schema'
})

// Resolve a property's current value out of the node's parameters
// map. Supports dotted paths (`a.b.c`) since n8n parameter names
// frequently nest (e.g. `conditions.combinator`, `options.caseSensitive`).
function resolveParamValue(name: string): unknown {
  if (!selectedNode.value?.parameters) return undefined
  const parts = name.split('.')
  let cur: any = selectedNode.value.parameters
  for (const part of parts) {
    if (cur && typeof cur === 'object' && part in cur) {
      cur = cur[part]
    } else {
      return undefined
    }
  }
  return cur
}

// Download the currently-displayed data tab as a JSON file. n8n
// does not expose this from the NDV directly, but it's a small
// affordance for debugging.
function downloadData() {
  const payload = dataTab.value === 'input' ? selectedNodeInput.value : selectedNodeOutput.value
  if (!payload) return
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${selectedNode.value?.name ?? 'node'}-${dataTab.value}.json`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
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
          <h1 class="text-xl font-bold text-slate-900 dark:text-white truncate">
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

    <!-- Body -->
    <div v-if="execution" class="flex-1 flex min-h-0">
      <!-- Left: workflow graph -->
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

      <!-- Right: Node Details View -->
      <div class="w-[520px] border-l border-slate-200 dark:border-slate-700 flex flex-col bg-white dark:bg-slate-800">
        <div v-if="selectedNode" class="flex-1 flex flex-col min-h-0">
          <!-- Node header -->
          <div class="p-4 border-b border-slate-200 dark:border-slate-700">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0 flex-1">
                <p class="text-[10px] text-slate-500 dark:text-slate-400 uppercase tracking-wide truncate">
                  {{ selectedNode.type }}
                </p>
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

          <!-- Top tab row: Input · Output · Settings -->
          <div class="flex border-b border-slate-200 dark:border-slate-700">
            <button
              v-for="tab in (['input', 'output'] as DataTab[])"
              :key="tab"
              @click="dataTab = tab"
              :class="[
                'flex-1 px-3 py-2 text-xs font-medium uppercase tracking-wide transition-colors',
                dataTab === tab
                  ? 'text-primary-600 dark:text-primary-400 border-b-2 border-primary-500 bg-primary-50/40 dark:bg-primary-900/20'
                  : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'
              ]"
            >
              {{ tab }}
            </button>
            <button
              v-if="hasSettingsTab"
              @click="dataTab = 'output'"
              class="flex-1 px-3 py-2 text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 flex items-center justify-center gap-1.5"
              :title="dataTab === 'input' ? 'Settings tab is part of the right pane — open the workflow editor to edit' : 'This node exposes ' + settingsProperties.length + ' settings'"
            >
              <Cog6ToothIcon class="w-3.5 h-3.5" />
              Settings ({{ settingsProperties.length }})
            </button>
            <button
              v-if="dataTab !== 'input' || selectedNodeInput"
              @click="downloadData"
              class="px-3 text-slate-400 hover:text-slate-700 dark:hover:text-slate-200"
              title="Download as JSON"
            >
              <ArrowDownTrayIcon class="w-4 h-4" />
            </button>
          </div>

          <!-- Error banner -->
          <div
            v-if="selectedNodeError"
            class="m-4 p-3 rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-900"
          >
            <div class="flex items-center gap-2 mb-1">
              <XCircleIcon class="w-5 h-5 text-red-500" />
              <h3 class="font-semibold text-red-700 dark:text-red-300 text-sm">Error</h3>
            </div>
            <pre class="text-xs text-red-700 dark:text-red-300 whitespace-pre-wrap font-mono">{{ selectedNodeError }}</pre>
          </div>

          <!-- View-mode row: Schema · Table · JSON -->
          <div class="flex border-b border-slate-200 dark:border-slate-700 px-4">
            <button
              v-for="m in (['schema', 'table', 'json'] as ViewMode[])"
              :key="m"
              @click="viewMode = m"
              :class="[
                'px-3 py-1.5 text-xs font-medium uppercase tracking-wide',
                viewMode === m
                  ? 'text-primary-600 dark:text-primary-400 border-b-2 border-primary-500'
                  : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'
              ]"
            >
              {{ m }}
            </button>
          </div>

          <!-- Data content -->
          <div class="flex-1 overflow-y-auto">
            <ExecutionDataView
              v-if="dataTab === 'input'"
              :data="selectedNodeInput"
              :mode="viewMode"
              empty-label="No upstream items — this is a trigger node."
            />
            <!-- Multi-branch output (IF / Switch): render one
                 collapsible section per branch so the operator can
                 tell which items went to the true vs false
                 output, etc. Falls back to a single
                 ExecutionDataView for normal nodes. -->
            <div v-else-if="isMultiOutputNode && outputBuckets.length > 1" class="divide-y divide-slate-200 dark:divide-slate-700">
              <details
                v-for="bucket in outputBuckets"
                :key="bucket.label"
                class="group"
                open
              >
                <summary class="cursor-pointer list-none px-4 py-2 flex items-center gap-2 bg-slate-50 dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800">
                  <ChevronRightIcon class="w-3 h-3 transition-transform group-open:rotate-90 text-slate-400" />
                  <span class="text-xs font-medium text-slate-700 dark:text-slate-300">{{ bucket.label }}</span>
                  <span class="ml-auto text-[10px] text-slate-400">{{ bucket.items.length }} item{{ bucket.items.length === 1 ? '' : 's' }}</span>
                </summary>
                <ExecutionDataView
                  :data="stripRoutingTags(bucket.items)"
                  :mode="viewMode"
                  empty-label="No items on this branch."
                />
              </details>
            </div>
            <ExecutionDataView
              v-else
              :data="stripRoutingTags(selectedNodeOutput ?? [])"
              :mode="viewMode"
              empty-label="No output for this node."
            />

            <!-- Binary data: shown after the JSON/Schema/Table
                 views when an item carries `binary` keys
                 (HTTP Request with `binaryData: true`,
                 Read Binary File, Write Binary File, etc). -->
            <div
              v-if="dataTab === 'output' && selectedNodeBinaryEntries.length > 0"
              class="border-t border-slate-200 dark:border-slate-700"
            >
              <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 px-4 pt-4 pb-2">
                Binary
              </h3>
              <div class="px-4 pb-4 space-y-2">
                <div
                  v-for="entry in selectedNodeBinaryEntries"
                  :key="entry.key"
                  class="flex items-center justify-between text-xs p-2 rounded-md bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700"
                >
                  <div class="min-w-0">
                    <div class="font-mono text-slate-700 dark:text-slate-300 truncate">
                      {{ entry.key }}
                    </div>
                    <div class="text-[10px] text-slate-500 dark:text-slate-400 truncate">
                      {{ entry.mimeType || 'application/octet-stream' }}
                      <span v-if="entry.fileSize"> · {{ entry.fileSize }}</span>
                      <span v-if="entry.fileName"> · {{ entry.fileName }}</span>
                    </div>
                  </div>
                  <a
                    v-if="entry.dataUrl"
                    :href="entry.dataUrl"
                    :download="entry.fileName || entry.key"
                    class="text-primary-600 dark:text-primary-400 text-xs hover:underline"
                  >
                    Download
                  </a>
                </div>
              </div>
            </div>

            <!-- Per-node-type parameter rendering (Parameters section) -->
            <div
              v-if="parameterProperties.length > 0 && dataTab === 'output'"
              class="border-t border-slate-200 dark:border-slate-700 p-4"
            >
              <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 mb-3">
                Parameters
              </h3>
              <div
                v-for="prop in parameterProperties"
                :key="prop.name"
              >
                <PropertyEditor
                  :property="prop"
                  :value="resolveParamValue(prop.name)"
                />
              </div>
            </div>

            <!-- Settings section (retry / continueOnFail / etc) -->
            <div
              v-if="settingsProperties.length > 0 && dataTab === 'output'"
              class="border-t border-slate-200 dark:border-slate-700 p-4"
            >
              <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 mb-3">
                Settings
              </h3>
              <div
                v-for="prop in settingsProperties"
                :key="prop.name"
              >
                <PropertyEditor
                  :property="prop"
                  :value="resolveParamValue(prop.name)"
                />
              </div>
            </div>

            <!-- Credentials section -->
            <div
              v-if="selectedNode.credentials && Object.keys(selectedNode.credentials).length > 0"
              class="border-t border-slate-200 dark:border-slate-700 p-4"
            >
              <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 mb-3">
                Credentials
              </h3>
              <div class="space-y-1">
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
        </div>

        <div v-else class="flex-1 flex items-center justify-center text-slate-400 dark:text-slate-500 text-sm">
          <div class="text-center">
            <PlayIcon class="w-8 h-8 mx-auto mb-2 opacity-50" />
            Click a node on the canvas to see its input, output, and parameters.
          </div>
        </div>
      </div>
    </div>

    <div v-else class="flex-1 flex items-center justify-center">
      <div class="text-center">
        <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto" />
        <p class="mt-4 text-slate-500 dark:text-slate-400">Loading execution details…</p>
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
