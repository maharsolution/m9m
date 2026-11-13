/**
 * Node placement helpers.
 *
 * Two ways lead a node onto the canvas:
 *
 *   * the node was **dragged** out of the node creator — it lands
 *     where it was dropped (the canvas owns that coordinate);
 *   * the node was **clicked** in the node creator (n8n parity) — it
 *     lands in the middle of the visible canvas, on a free spot.
 *
 * The naming rule is n8n's: the first instance keeps the bare
 * display name ("HTTP Request"), duplicates get a numeric suffix
 * without a space ("HTTP Request1"). Node names are the keys of the
 * n8n `connections` map, so they must stay unique inside a workflow.
 */

import type { WorkflowNode } from '@/types'

/** Horizontal gap between two node cards (card width + margin). */
export const NODE_SPACING_X = 280

/** The box a node card occupies around its position (canvas units). */
export const NODE_CARD_BOX = { width: 220, height: 140 }

/**
 * A node name that is not taken yet: the bare display name, or the
 * display name with the lowest free numeric suffix.
 */
export function uniqueNodeName(baseName: string, taken: Iterable<string>): string {
  const used = taken instanceof Set ? taken : new Set(taken)
  if (!used.has(baseName)) return baseName

  let counter = 1
  while (used.has(`${baseName}${counter}`)) {
    counter += 1
  }

  return `${baseName}${counter}`
}

/**
 * The centre of a rectangle — the screen position a click-to-add node
 * lands on, projected onto the canvas by the caller.
 */
export function centerOfRect(rect: {
  left: number
  top: number
  width: number
  height: number
}): { x: number; y: number } {
  return {
    x: rect.left + rect.width / 2,
    y: rect.top + rect.height / 2,
  }
}

/**
 * True when a node card already covers the given canvas position.
 */
export function isSpotTaken(
  spot: [number, number],
  nodes: WorkflowNode[]
): boolean {
  return nodes.some(
    (node) =>
      Math.abs(node.position[0] - spot[0]) < NODE_CARD_BOX.width &&
      Math.abs(node.position[1] - spot[1]) < NODE_CARD_BOX.height
  )
}

/**
 * A free spot for a node that was clicked into the canvas.
 *
 * The node goes to the middle of the visible canvas. When a card
 * already covers that spot, the new node steps to the right until it
 * finds a free one — so a clicked node is never hidden behind an
 * existing card.
 */
export function freeNodePosition(
  center: [number, number],
  nodes: WorkflowNode[]
): [number, number] {
  if (!isSpotTaken(center, nodes)) return center

  for (let step = 1; step <= nodes.length; step += 1) {
    const candidate: [number, number] = [center[0] + step * NODE_SPACING_X, center[1]]
    if (!isSpotTaken(candidate, nodes)) return candidate
  }

  return [center[0] + (nodes.length + 1) * NODE_SPACING_X, center[1]]
}
