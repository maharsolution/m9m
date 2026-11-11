/**
 * Auto-layout ("tidy up") for the workflow canvas.
 *
 * Uses dagre to compute a left-to-right layered layout of the
 * workflow graph and hands the resulting top-left coordinates back to
 * the caller. The layout is intentionally non-destructive: it returns
 * a map of node id → position, the caller decides what to do with
 * it (the editor store writes the positions back into the workflow
 * document and records a history entry).
 *
 * Sticky notes are free-floating annotations in n8n and are *not*
 * part of the flow graph, so they are excluded from the layout by
 * default and keep their position.
 */

import { graphlib, layout } from '@dagrejs/dagre'
import type { NodeConnections, WorkflowNode } from '@/types'
import {
  STICKY_NOTE_DEFAULT_HEIGHT,
  STICKY_NOTE_DEFAULT_WIDTH,
  STICKY_NOTE_TYPE,
} from '@/types'

export type LayoutDirection = 'LR' | 'TB'

export interface AutoLayoutOptions {
  /** Rank direction. `LR` = left to right (n8n's default). */
  direction?: LayoutDirection
  /** Width assumed for a regular node when computing the layout. */
  nodeWidth?: number
  /** Height assumed for a regular node when computing the layout. */
  nodeHeight?: number
  /** Space between nodes of the same rank. */
  nodesep?: number
  /** Space between ranks. */
  ranksep?: number
  /** Outer margin around the whole layout. */
  margin?: number
  /** Lay sticky notes out as well (off by default, like n8n). */
  includeStickyNotes?: boolean
}

export interface LayoutPosition {
  x: number
  y: number
}

const DEFAULT_NODE_WIDTH = 220
const DEFAULT_NODE_HEIGHT = 80

function readNumber(value: unknown, fallback: number): number {
  const parsed = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

/**
 * The size dagre should reserve for a node. Sticky notes carry their
 * own geometry in `parameters.width` / `parameters.height`.
 */
export function layoutNodeSize(
  node: WorkflowNode,
  options: AutoLayoutOptions
): { width: number; height: number } {
  if (node.type === STICKY_NOTE_TYPE) {
    return {
      width: readNumber(node.parameters?.width, STICKY_NOTE_DEFAULT_WIDTH),
      height: readNumber(node.parameters?.height, STICKY_NOTE_DEFAULT_HEIGHT),
    }
  }
  return {
    width: options.nodeWidth ?? DEFAULT_NODE_WIDTH,
    height: options.nodeHeight ?? DEFAULT_NODE_HEIGHT,
  }
}

/**
 * Collects the edges of the workflow graph. The workflow document
 * keys connections by *node name*; dagre works on node ids, so the
 * names are translated back to ids here.
 */
export function collectLayoutEdges(
  nodes: WorkflowNode[],
  connections: Record<string, NodeConnections>
): Array<{ source: string; target: string }> {
  const idByName = new Map(nodes.map((node) => [node.name, node.id]))
  const edges: Array<{ source: string; target: string }> = []
  const seen = new Set<string>()

  Object.entries(connections ?? {}).forEach(([sourceName, nodeConnections]) => {
    const sourceId = idByName.get(sourceName)
    if (!sourceId) {
      return
    }
    nodeConnections.main?.forEach((outputs) => {
      outputs.forEach((connection) => {
        const targetId = idByName.get(connection.node)
        if (!targetId || targetId === sourceId) {
          return
        }
        const key = `${sourceId}->${targetId}`
        if (seen.has(key)) {
          return
        }
        seen.add(key)
        edges.push({ source: sourceId, target: targetId })
      })
    })
  })

  return edges
}

/**
 * Computes the layout. Returns a map of node id → top-left position
 * (the same coordinate space Vue Flow uses for `position`).
 */
export function computeAutoLayout(
  nodes: WorkflowNode[],
  connections: Record<string, NodeConnections>,
  options: AutoLayoutOptions = {}
): Map<string, LayoutPosition> {
  const positions = new Map<string, LayoutPosition>()
  if (!nodes?.length) {
    return positions
  }

  const includeStickyNotes = options.includeStickyNotes ?? false
  const layoutNodes = includeStickyNotes
    ? nodes
    : nodes.filter((node) => node.type !== STICKY_NOTE_TYPE)

  if (!layoutNodes.length) {
    return positions
  }

  const graph = new graphlib.Graph({ directed: true, multigraph: false })
  graph.setGraph({
    rankdir: options.direction ?? 'LR',
    nodesep: options.nodesep ?? 60,
    ranksep: options.ranksep ?? 120,
    marginx: options.margin ?? 20,
    marginy: options.margin ?? 20,
  })
  // dagre writes the edge geometry (`points`) onto the edge label, so
  // every edge needs a label object. Without this default the layout
  // throws "Cannot set properties of undefined (setting 'points')".
  graph.setDefaultEdgeLabel(() => ({}))

  layoutNodes.forEach((node) => {
    const size = layoutNodeSize(node, options)
    graph.setNode(node.id, { width: size.width, height: size.height })
  })

  collectLayoutEdges(layoutNodes, connections ?? {}).forEach((edge) => {
    if (graph.hasNode(edge.source) && graph.hasNode(edge.target)) {
      graph.setEdge(edge.source, edge.target)
    }
  })

  layout(graph)

  layoutNodes.forEach((node) => {
    const laidOut = graph.node(node.id)
    if (!laidOut || typeof laidOut.x !== 'number' || typeof laidOut.y !== 'number') {
      return
    }
    const size = layoutNodeSize(node, options)
    positions.set(node.id, {
      // dagre reports the centre of the node, Vue Flow wants the
      // top-left corner.
      x: Math.round(laidOut.x - size.width / 2),
      y: Math.round(laidOut.y - size.height / 2),
    })
  })

  return positions
}
