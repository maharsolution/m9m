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
import { ref, computed, markRaw, onMounted, watch } from 'vue'
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
import { buildFlowNodes, buildFlowEdges, parseEdgeId } from '@/lib/workflowGraph'
import type { Workflow, WorkflowNode, DataItem } from '@/types'
import type { NodeType, NodeProperty } from '@/types/node'
import ExecutionDataView from '@/components/execution/ExecutionDataView.vue'
import PropertyEditor from '@/components/execution/PropertyEditor.vue'
import BaseNode from '@/components/nodes/BaseNode.vue'

// nodeTypes maps the `type: 'custom'` discriminator emitted by
// buildFlowNodes to the actual Vue component that should render
// each node. Without this mapping, Vue Flow falls back to its
// built-in default renderer which renders the node label as
// inline text with no styling — that's the bug behind "execution
// view shows tiny raw-text nodes". WorkflowCanvas registers the
// exact same mapping; we mirror it here so the editor and
// execution views look identical.
//
// `markRaw` tells Vue not to make the component definition
// reactive (the Vue Flow library keeps an internal Map of types
// and a reactive wrapper around it triggers spurious re-renders
// on every reactive change). WorkflowCanvas uses the same
// `markRaw` pattern — see node-types in that file.
const nodeTypes = {
  custom: markRaw(BaseNode),
}

type NodeState = 'pending' | 'success' | 'failed' | 'running' | 'skipped'
type DataTab = 'input' | 'output' | 'settings'
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

// Top-level tabs rendered in the NDV header. Mirrors n8n's
// `Input | Output | Settings` tab row. The Settings tab is only
// included when the selected node exposes a `properties`
// descriptor (i.e. has parameters / engine settings to display);
// otherwise the tab list is just Input/Output so the user always
// has the data tabs front-and-center.
const dataTabs = computed<Array<{ id: DataTab; label: string; icon?: any; title?: string }>>(() => {
  const tabs: Array<{ id: DataTab; label: string; icon?: any; title?: string }> = [
    { id: 'input', label: 'Input' },
    { id: 'output', label: 'Output' },
  ]
  if (hasSettingsTab.value) {
    tabs.push({
      id: 'settings',
      label: `Settings (${settingsProperties.value.length + parameterProperties.value.length})`,
      icon: Cog6ToothIcon,
      title: 'Node parameters and engine settings',
    })
  }
  return tabs
})

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

  // `nodeData` is gated by workflow.Debug (see model.Workflow.Debug
  // and buildExecutionNodeData in the API). When Debug is OFF the
  // backend keeps only the start node and last node — every other
  // node has no entry and the loop below would mark them all
  // 'pending'. We want the canvas to still show them as 'success'
  // (or 'failed' when status==='failed') based on the workflow's
  // final status, because the engine DID execute them; we just
  // didn't capture the snapshot.
  //
  // We therefore resolve a "did this node run?" signal that doesn't
  // depend on the per-node snapshot:
  //   - status==='completed' : every node ran successfully
  //   - status==='failed'    : nodes UPSTREAM of the failed
  //                            branch ran; downstream didn't. The
  //                            walk below paints the cascade.
  //   - status==='running'   : every node is in-flight until proven
  //                            otherwise; mark all 'running' so the
  //                            canvas reflects the live state.

  for (const node of workflow.value.nodes) {
    if (execution.value.status === 'failed') {
      const data = nodeData[node.name]
      if (data !== undefined) {
        // Per-node snapshot exists — use it.
        if (
          Array.isArray(data) &&
          data.some((d) => d && (d as any).error !== undefined)
        ) {
          map.set(node.name, 'failed')
        } else {
          map.set(node.name, 'success')
        }
      } else {
        // No snapshot. Two possibilities:
        //   1. Debug=OFF: this isn't a start/end node, so backend
        //      didn't record it. Mark 'pending' so the user knows
        //      there's no per-node detail.
        //   2. Debug=ON: backend always records, so absence means
        //      the engine didn't get to this node yet.
        // We can't tell those apart without the snapshot, so
        // default to 'pending' and let the cascade-fail walk
        // below promote failed nodes correctly.
        map.set(node.name, 'pending')
      }
    } else if (execution.value.status === 'running') {
      map.set(node.name, 'running')
    } else {
      // completed / cancelled — every node that the engine ran is
      // either 'success' (snapshot present) or 'pending' (no
      // snapshot — Debug=OFF).
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

// workflowDebug mirrors model.Workflow.Debug on the JS side.
// When false, the backend only stores the start node's input
// and the last node's output, so the per-node Input/Output tabs
// for any OTHER node in this execution will be empty. The hint
// in the NDV explains that and points the user at the workflow
// editor's "Debug" toggle so they can re-run with full capture.
const workflowDebug = computed(() => workflow.value?.debug === true)

// startNodeName / lastNodeName mirror buildExecutionNodeData's
// helpers on the JS side so we can decide, for a given selected
// node, whether per-node data is expected to be present.
//
// Sticky Notes (and other decorative nodes) are skipped: they
// have no executor, never carry data, and would otherwise hijack
// the slot with an empty payload.
const DECORATIVE_NODE_TYPES = new Set([
  'n8n-nodes-base.stickyNote',
  'n8n-nodes-base.note',
  '@n8n/n8n-nodes-langchain.note',
])

const startNodeName = computed<string | null>(() => {
  if (!workflow.value?.nodes?.length) return null
  const hasIncoming = new Set<string>()
  for (const conns of Object.values(workflow.value.connections ?? {})) {
    for (const outputs of (conns as any).main ?? []) {
      for (const c of outputs) hasIncoming.add(c.node)
    }
  }
  for (const n of workflow.value.nodes) {
    if (DECORATIVE_NODE_TYPES.has(n.type)) continue
    if (!hasIncoming.has(n.name)) return n.name
  }
  return null
})

const lastNodeName = computed<string | null>(() => {
  if (!workflow.value?.nodes?.length) return null
  const hasOutgoing = new Set<string>()
  for (const [source, conns] of Object.entries(workflow.value.connections ?? {})) {
    for (const outputs of (conns as any).main ?? []) {
      if (outputs.length > 0) hasOutgoing.add(source)
    }
  }
  for (let i = workflow.value.nodes.length - 1; i >= 0; i--) {
    const n = workflow.value.nodes[i]
    if (DECORATIVE_NODE_TYPES.has(n.type)) continue
    if (!hasOutgoing.has(n.name)) return n.name
  }
  return null
})

// perNodeDataAvailable tells the NDV whether to show the data
// tabs for the currently-selected node, or render a hint that
// the per-node snapshot wasn't captured (Debug=OFF).
//
// Returns one of:
//   - 'available'  — the node is in nodeData; show the tabs
//   - 'unavailable' — Debug=OFF AND this node isn't start/end
//   - 'empty'      — Debug=ON but no data; node ran with no items
const perNodeDataAvailable = computed(() => {
  if (!workflowDebug.value) {
    const sel = selectedNodeName.value
    if (sel && sel !== startNodeName.value && sel !== lastNodeName.value) {
      return 'unavailable'
    }
  }
  const out = selectedNodeOutput.value
  if (out && Array.isArray(out) && out.length > 0) return 'available'
  return 'empty'
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
    // Execution canvas reads more like a debugger than an editor,
    // so render every node at 'comfortable' density — bigger hit
    // target, bigger icon, easier to scan at a glance. Editor
    // canvas (WorkflowCanvas) keeps the compact default.
    return {
      ...n,
      data: { ...(n.data as Record<string, unknown>), density: 'comfortable' },
      selected: n.id === selectedNodeId.value,
      class: stateRing[state],
    }
  })
})

const flowEdges = computed(() => {
  const base = buildFlowEdges(workflow.value as Workflow | null)
  // The engine publishes a per-edge "did this connection carry
  // items?" map keyed by `<sourceID>:<outputIndex>:<targetID>:
  // <inputIndex>` — the same tuple the frontend uses to build
  // its own edge IDs, minus the leading `workflow-edge:` prefix
  // and the URL-encoding the frontend adds for safety. Look up
  // by the raw tuple (no prefix, no encoding) so a green line
  // for a connection that actually carried data is preserved
  // even when Debug=OFF (and per-node snapshots are absent).
  const edgesTaken = execution.value?.edgesTaken ?? {}
  return base.map((e) => {
    const sourceName = workflow.value?.nodes.find((n) => n.id === e.source)?.name
    const targetName = workflow.value?.nodes.find((n) => n.id === e.target)?.name
    const sourceState = sourceName ? nodeStates.value.get(sourceName) : undefined
    const targetState = targetName ? nodeStates.value.get(targetName) : undefined
    // `parseEdgeId` re-exports the URL-decoded source/target IDs
    // and the integer indices; that's the same tuple shape the
    // engine emits, so we can index into edgesTaken directly.
    const parsed = parseEdgeId(e.id)
    const edgeTaken = parsed
      ? edgesTaken[`${parsed.sourceNodeId}:${parsed.sourceOutput}:${parsed.targetNodeId}:${parsed.targetInput}`]
      : false
    let stroke = '#94a3b8' // neutral slate-400 for pending/default
    let animated = false
    // Per-edge branch colouring has priority over per-node state
    // colouring. When the engine recorded that this specific
    // connection didn't carry items (e.g. the IF routed everything
    // down main[0] and main[1] is empty), paint it grey regardless
    // of whether both endpoints are 'success' — the user is asking
    // to see "which path ran", not "which nodes are healthy". The
    // per-node state colour is still the fallback for runs where
    // edgesTaken is missing (older executions) or undefined for
    // this specific edge.
    if (execution.value && Object.keys(edgesTaken).length > 0) {
      if (edgeTaken) {
        if (sourceState === 'failed' || targetState === 'failed') {
          stroke = '#ef4444' // red-500 — taken but failed upstream/downstream
        } else if (sourceState === 'running' || targetState === 'running') {
          stroke = '#3b82f6' // blue-500 — taken and currently running
          animated = true
        } else {
          stroke = '#22c55e' // green-500 — taken, healthy
        }
      } else {
        stroke = '#cbd5e1' // slate-300 — not taken, branch didn't run
      }
    } else if (sourceState === 'success' && targetState !== 'skipped') {
      stroke = '#22c55e' // green-500
    } else if (sourceState === 'failed' || targetState === 'failed') {
      stroke = '#ef4444' // red-500
    } else if (sourceState === 'running' || targetState === 'running') {
      stroke = '#3b82f6' // blue-500
      animated = true
    } else if (targetState === 'skipped') {
      stroke = '#cbd5e1' // slate-300
    }
    return {
      ...e,
      animated,
      style: { ...(e.style ?? {}), stroke, strokeWidth: 2.5 },
    }
  })
})

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
    // Trigger node / start node — no upstream. n8n's NDV Input
    // tab shows the trigger payload on a trigger node, so
    // mirror that: render the node's own output (which IS the
    // trigger payload for a Webhook, Cron, etc.). Falls through
    // to undefined only when there's genuinely no data anywhere
    // for this node, so the panel renders "no upstream" rather
    // than an empty list (which would look like "this node
    // dropped everything").
    const own = execution.value.nodeData?.[selectedNodeName.value]
    if (own && own.length > 0) {
      return own
    }
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

// `retryingNode` flips the spinner on the "Retry from here"
// button so the user gets feedback while the backend builds the
// sub-workflow and runs it. We don't disable the whole canvas
// because the request is fast in practice; a per-button spinner
// is enough.
const retryingNode = ref(false)
const retryFromSelected = async () => {
  if (!execution.value || !selectedNodeName.value || retryingNode.value) return
  retryingNode.value = true
  try {
    const newExecution = await executionStore.retryNode(execution.value.id, {
      nodeName: selectedNodeName.value,
    })
    router.push(`/executions/${newExecution.id}`)
  } catch (e) {
    console.error('Failed to retry from node:', e)
  } finally {
    retryingNode.value = false
  }
}

const goBack = () => router.push('/executions')

const onNodeClick = (event: { node: { id: string } }) => {
  const n = workflow.value?.nodes.find((node) => node.id === event.node.id)
  if (n) {
    selectedNodeName.value = n.name
    // Always land on the Input tab when the user clicks a new
    // node so the first thing they see is real data, not the
    // Parameters pane (which was the prior "only parameter"
    // confusion).
    dataTab.value = 'input'
    viewMode.value = 'schema'
  }
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
          <p class="text-sm text-slate-500 dark:text-slate-400 flex items-center gap-2">
            <code class="text-xs">{{ execution?.id?.slice(0, 8) }}</code>
            · started {{ execution ? formatDate(execution.startedAt) : '—' }}
            <!-- Debug badge mirrors the workflow's debug flag so
                 the user knows whether intermediate-node data
                 will be present in this run. -->
            <span
              v-if="workflow"
              :class="[
                'inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium uppercase tracking-wide',
                workflowDebug
                  ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
                  : 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-400'
              ]"
              :title="workflowDebug
                ? 'Debug ON — per-node I/O captured for every step'
                : 'Debug OFF — only start and end nodes have per-node data'"
            >
              Debug: {{ workflowDebug ? 'ON' : 'OFF' }}
            </span>
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
          :node-types="nodeTypes"
          :default-viewport="{ x: 100, y: 100, zoom: 1 }"
          :min-zoom="0.3"
          :max-zoom="2"
          :nodes-draggable="false"
          :nodes-connectable="false"
          :elements-selectable="true"
          fit-view-on-init
          :fit-view-on-init-options="{ padding: 0.25, minZoom: 0.7, maxZoom: 1.2, includeHiddenNodes: false }"
          @node-click="onNodeClick"
        >
          <Background :pattern-color="'#cbd5e1'" :gap="20" />
        </VueFlow>

        <div class="absolute bottom-3 left-3 bg-white/95 dark:bg-slate-800/95 backdrop-blur rounded-lg shadow p-2 flex items-center gap-3 text-xs">
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded-full bg-green-500"></span>
            <span class="text-slate-600 dark:text-slate-300">Node: Success</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded-full bg-red-500"></span>
            <span class="text-slate-600 dark:text-slate-300">Node: Failed</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded-full bg-blue-500"></span>
            <span class="text-slate-600 dark:text-slate-300">Node: Running</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="w-3 h-3 rounded-full bg-slate-300"></span>
            <span class="text-slate-600 dark:text-slate-300">Node: Pending / Skipped</span>
          </div>
          <!-- Edge colouring legend. Edges on the path that ran
               are green; edges on branches that didn't carry
               items (e.g. the false side of an IF) are grey.
               Requires the engine to have published
               `edgesTaken`; otherwise the colour falls back to
               the per-node state. -->
          <div class="ml-2 pl-2 border-l border-slate-200 dark:border-slate-600 flex items-center gap-3">
            <div class="flex items-center gap-1">
              <span class="w-4 h-0.5 bg-green-500"></span>
              <span class="text-slate-600 dark:text-slate-300">Path: ran</span>
            </div>
            <div class="flex items-center gap-1">
              <span class="w-4 h-0.5 bg-slate-300"></span>
              <span class="text-slate-600 dark:text-slate-300">Path: skipped</span>
            </div>
          </div>
          <!-- Per-node retry. Disabled until the user selects a
               node; clicking re-runs the workflow starting from
               that node and navigates to the new execution. Mirrors
               n8n's "Retry from here" affordance on the failed
               node. -->
          <div class="ml-2 pl-2 border-l border-slate-200 dark:border-slate-600">
            <button
              :disabled="!selectedNodeName || retryingNode"
              @click="retryFromSelected"
              class="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium text-primary-700 dark:text-primary-300 hover:bg-primary-50 dark:hover:bg-primary-900/30 disabled:opacity-40 disabled:cursor-not-allowed"
              title="Re-run the workflow starting from the selected node"
            >
              <ArrowPathIcon :class="['w-3.5 h-3.5', retryingNode ? 'animate-spin' : '']" />
              {{ retryingNode ? 'Retrying…' : 'Retry from here' }}
            </button>
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
              v-for="tab in (dataTabs)"
              :key="tab.id"
              @click="dataTab = tab.id"
              :class="[
                'flex-1 px-3 py-2 text-xs font-medium uppercase tracking-wide transition-colors flex items-center justify-center gap-1.5',
                dataTab === tab.id
                  ? 'text-primary-600 dark:text-primary-400 border-b-2 border-primary-500 bg-primary-50/40 dark:bg-primary-900/20'
                  : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'
              ]"
              :title="tab.title"
            >
              <component v-if="tab.icon" :is="tab.icon" class="w-3.5 h-3.5" />
              {{ tab.label }}
            </button>
            <button
              v-if="dataTab === 'input' || dataTab === 'output'"
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

          <!-- View-mode row: Schema · Table · JSON (only for
               the data tabs; the Settings tab has no view modes) -->
          <div v-if="dataTab === 'input' || dataTab === 'output'" class="flex border-b border-slate-200 dark:border-slate-700 px-4">
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
            <!-- Per-node data unavailable hint. Shown when the
                 workflow's Debug flag is OFF and the user selected
                 a node that isn't the start or end node — the
                 backend didn't capture a snapshot for this node in
                 the latest run. The hint explains the cause and
                 points at the workflow editor's Debug toggle so
                 the user can flip it on and re-run. -->
            <div
              v-if="perNodeDataAvailable === 'unavailable' && (dataTab === 'input' || dataTab === 'output')"
              class="m-4 p-4 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-900"
            >
              <div class="flex items-center gap-2 mb-1">
                <ExclamationTriangleIcon class="w-5 h-5 text-amber-500" />
                <h3 class="font-semibold text-amber-700 dark:text-amber-300 text-sm">
                  Debug capture is OFF for this workflow
                </h3>
              </div>
              <p class="text-xs text-amber-700 dark:text-amber-300 leading-relaxed">
                Per-node input/output is only recorded for the start and last node
                when Debug is OFF. Open this workflow in the editor and flip
                <span class="font-semibold">Debug ON</span>, save, and re-run to see
                full per-node data for every step.
              </p>
            </div>
            <ExecutionDataView
              v-else-if="dataTab === 'input'"
              :data="selectedNodeInput"
              :mode="viewMode"
              empty-label="No upstream items — this is a trigger node."
            />
            <!-- Settings tab body: parameters + engine settings +
                 credentials. Rendered when the user clicks the
                 Settings tab. Mirroring n8n's NDV, parameters
                 now live behind their own tab so the data tabs
                 are pure data (was the user complaint:
                 "currently only parameter is shown"). -->
            <div
              v-else-if="dataTab === 'settings'"
              class="p-4 space-y-6"
            >
              <div v-if="parameterProperties.length > 0">
                <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 mb-3">
                  Parameters
                </h3>
                <div class="space-y-3">
                  <div v-for="prop in parameterProperties" :key="prop.name">
                    <PropertyEditor
                      :property="prop"
                      :value="resolveParamValue(prop.name)"
                    />
                  </div>
                </div>
              </div>
              <div v-if="settingsProperties.length > 0">
                <h3 class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 mb-3">
                  Engine Settings
                </h3>
                <div class="space-y-3">
                  <div v-for="prop in settingsProperties" :key="prop.name">
                    <PropertyEditor
                      :property="prop"
                      :value="resolveParamValue(prop.name)"
                    />
                  </div>
                </div>
              </div>
              <div v-if="selectedNode.credentials && Object.keys(selectedNode.credentials).length > 0">
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
              <div v-if="parameterProperties.length === 0 && settingsProperties.length === 0 && (!selectedNode.credentials || Object.keys(selectedNode.credentials).length === 0)" class="text-xs text-slate-400 italic">
                This node does not expose any parameters or settings.
              </div>
            </div>
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
              v-else-if="dataTab === 'output'"
              :data="stripRoutingTags(selectedNodeOutput ?? [])"
              :mode="viewMode"
              empty-label="No output for this node."
            />
          </div>

          <!-- Binary data: shown as a separate block below the
               data tabs (always visible when the Output tab has
               binary items, regardless of view mode). -->
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

/* Edge path styles. Vue Flow renders the SVG stroke from the
 * per-edge `style.stroke` we set in flowEdges, but we also need to
 * override the default fill / opacity rules so dark-mode edges
 * stay visible. */
.vue-flow__edge-path {
  fill: none;
}

.vue-flow__edge.selected .vue-flow__edge-path {
  stroke-width: 3;
}
</style>
