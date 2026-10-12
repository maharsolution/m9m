// Sub-workflow palette for BUG-ARCH-2026-10-12-004.
//
// n8n's "Execute Workflow" / "Execute Sub-workflow" node
// invokes another workflow by id. m9m already has the
// engine-side `n8n-nodes-base.executeWorkflow` executor,
// but the editor palette didn't surface the node type
// clearly: the canvas drag-and-drop menu grouped it
// under "Core" alongside the start node, which confused
// new users.
//
// This module adds:
//   1. A `getSubWorkflowPaletteEntries()` helper that
//      returns the editor's "Insert → Sub-workflow"
//      entries (currently just `executeWorkflow`).
//   2. A small validation helper: a sub-workflow
//      reference must point at a known, non-self
//      workflow.
//
// The palette entry is intentionally a plain data
// structure; the editor wires it into the drag-and-drop
// menu. We don't touch the existing
// `getNodeCategory(nodeType)` to keep this change
// additive.

import type { NodeType } from '@/types/node'

export interface SubWorkflowPaletteEntry {
  // The node type to insert (`n8n-nodes-base.executeWorkflow`).
  type: string
  // Display name in the palette.
  displayName: string
  // Subtitle (shown in italics under the name in the palette).
  subtitle: string
  // Default parameters for the new node.
  defaultParameters: Record<string, unknown>
  // Default name for the new node.
  defaultName: string
}

// getSubWorkflowPaletteEntries returns the list of
// sub-workflow-related nodes the palette should expose.
export function getSubWorkflowPaletteEntries(): SubWorkflowPaletteEntry[] {
  return [
    {
      type: 'n8n-nodes-base.executeWorkflow',
      displayName: 'Execute Sub-workflow',
      subtitle: 'Run another workflow and return its output',
      defaultName: 'Execute Sub-workflow',
      defaultParameters: {
        // Empty `workflowId` makes the engine prompt the
        // user to pick a workflow at design time. The
        // node form lists available workflows from
        // `GET /api/v1/workflows`.
        source: 'database',
      },
    },
    {
      type: 'n8n-nodes-base.executeWorkflowTrigger',
      displayName: 'Called by Another Workflow',
      subtitle: 'Start node for a workflow invoked from Execute Sub-workflow',
      defaultName: 'Called by Another Workflow',
      defaultParameters: {},
    },
  ]
}

// validateSubWorkflowRef is the read-side check the
// form uses to ensure the operator picked a real
// workflow. Returns an error message (or "") describing
// what the user needs to fix.
//
// `selfId` is the workflow the node is being added to;
// a sub-workflow must NOT point at itself (would cause
// infinite recursion).
export function validateSubWorkflowRef(
  ref: { value?: string } | null | undefined,
  knownWorkflowIds: string[],
  selfId: string
): string {
  if (!ref || !ref.value) return 'Pick a workflow to execute'
  if (ref.value === selfId) return 'A sub-workflow cannot call itself (infinite recursion)'
  if (!knownWorkflowIds.includes(ref.value)) return 'Picked workflow does not exist'
  return ''
}

// isSubWorkflowType is a small helper the palette's
// drag-and-drop handlers can use to decide whether the
// dragged type belongs to the "Sub-workflow" group.
export function isSubWorkflowType(nodeType: string): boolean {
  return (
    nodeType === 'n8n-nodes-base.executeWorkflow' ||
    nodeType === 'n8n-nodes-base.executeWorkflowTrigger'
  )
}

// nodeTypeForSubWorkflow returns a minimal NodeType
// the canvas can use to register the entry. The full
// NodeType (with Properties) comes from
// `GET /api/v1/node-types/{name}`; this stub is the
// fallback when the API is not available.
export function nodeTypeForSubWorkflow(entry: SubWorkflowPaletteEntry): NodeType {
  return {
    name: entry.type,
    displayName: entry.displayName,
    description: entry.subtitle,
    version: 1,
    defaults: { name: entry.defaultName },
    inputs: ['main'],
    outputs: ['main'],
    group: ['flow'],
    subtitle: '={{$parameter["workflowId"]}}',
  }
}
