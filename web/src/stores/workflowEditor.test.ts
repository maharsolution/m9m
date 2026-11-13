import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useWorkflowEditorStore } from './workflowEditor'
import { useWorkflowStore } from './workflow'
import { createEmptyWorkflow } from '@/lib/workflowGraph'
import type { Workflow, WorkflowNode } from '@/types'

const webhookNode = (): WorkflowNode => ({
  id: 'node_webhook',
  name: 'Webhook',
  type: 'n8n-nodes-base.webhook',
  typeVersion: 1,
  position: [80, 80],
  parameters: {},
})

const setNode = (): WorkflowNode => ({
  id: 'node_set',
  name: 'Set',
  type: 'n8n-nodes-base.set',
  typeVersion: 1,
  position: [420, 80],
  parameters: {},
})

/** The Vue Flow id of the connection between the two nodes above. */
const EDGE_ID = 'workflow-edge:node_webhook:0:node_set:0'

/** A workflow with two nodes and one connection between them. */
const connectedWorkflow = (): Workflow => ({
  ...createEmptyWorkflow(),
  nodes: [webhookNode(), setNode()],
  connections: {
    Webhook: { main: [[{ node: 'Set', type: 'main', index: 0 }]] },
  },
})

describe('workflow editor store — new workflow reset', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  /**
   * A NEW workflow has to start empty. The canvas renders the nodes of
   * the workflow in the store, so a stale draft is a stale canvas.
   */
  it('starts a new workflow without nodes or connections', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()

    workflowStore.setCurrentWorkflow({
      ...createEmptyWorkflow(),
      nodes: [webhookNode()],
      connections: {
        Webhook: { main: [[{ node: 'Code', type: 'main', index: 0 }]] },
      },
    })

    editorStore.createNewWorkflow()

    expect(workflowStore.currentWorkflow?.nodes).toEqual([])
    expect(workflowStore.currentWorkflow?.connections).toEqual({})
  })

  it('clears the selection and the undo history for a new workflow', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()

    workflowStore.setCurrentWorkflow(createEmptyWorkflow())
    editorStore.addNode(webhookNode())
    editorStore.selectNode('node_webhook')
    expect(editorStore.canUndo).toBe(true)

    editorStore.createNewWorkflow()

    expect(editorStore.selectedNodeIds).toEqual([])
    expect(editorStore.canUndo).toBe(false)
    expect(editorStore.canRedo).toBe(false)
  })

  it('keeps the nodes of a duplicated workflow', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()

    const source: Workflow = {
      ...createEmptyWorkflow(),
      nodes: [webhookNode()],
    }
    workflowStore.setCurrentWorkflow(source)

    editorStore.createNewWorkflow(source, {
      id: '',
      name: 'Webhook (copy)',
    })

    expect(workflowStore.currentWorkflow?.nodes).toHaveLength(1)
    expect(workflowStore.currentWorkflow?.name).toBe('Webhook (copy)')
  })

  /**
   * Loading another workflow has to drop the selection, the open node
   * details view and the history of the previous document.
   */
  it('resets the editor state', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()

    workflowStore.setCurrentWorkflow(createEmptyWorkflow())
    editorStore.addNode(webhookNode())
    editorStore.selectNode('node_webhook')
    editorStore.openNodeDetails('node_webhook')

    editorStore.resetEditorState()

    expect(editorStore.selectedNodeIds).toEqual([])
    expect(editorStore.ndvNodeId).toBeNull()
    expect(editorStore.canUndo).toBe(false)
    expect(editorStore.isDirty).toBe(false)
  })
})

describe('workflow editor store — connection selection', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('selects a single connection on a click', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()
    workflowStore.setCurrentWorkflow(connectedWorkflow())

    editorStore.selectEdge(EDGE_ID)

    expect(editorStore.selectedEdgeIds).toEqual([EDGE_ID])
    expect(editorStore.selectedEdgeCount).toBe(1)
  })

  it('adds a connection to the selection on a modified click', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()
    workflowStore.setCurrentWorkflow(connectedWorkflow())

    editorStore.selectEdge(EDGE_ID)
    editorStore.selectEdge('workflow-edge:node_set:0:node_webhook:0', true)

    expect(editorStore.selectedEdgeCount).toBe(2)
  })

  it('drops the connection selection', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()
    workflowStore.setCurrentWorkflow(connectedWorkflow())

    editorStore.selectEdge(EDGE_ID)
    editorStore.clearEdgeSelection()

    expect(editorStore.selectedEdgeIds).toEqual([])
  })

  /**
   * n8n parity: a selected connection is removed with the delete key.
   * The removal is one undo step and it marks the workflow as changed.
   */
  it('removes the selected connection and keeps it undoable', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()
    workflowStore.setCurrentWorkflow(connectedWorkflow())
    editorStore.markClean()

    editorStore.selectEdge(EDGE_ID)
    const removed = editorStore.removeSelectedNodes()

    expect(removed).toBe(1)
    // The last connection of a node removes the whole entry.
    expect(workflowStore.currentWorkflow?.connections.Webhook).toBeUndefined()
    expect(editorStore.selectedEdgeIds).toEqual([])
    expect(editorStore.isDirty).toBe(true)
    expect(editorStore.canUndo).toBe(true)
  })

  it('restores the connection on undo', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()
    workflowStore.setCurrentWorkflow(connectedWorkflow())

    editorStore.selectEdge(EDGE_ID)
    editorStore.removeSelectedNodes()
    editorStore.undo()

    expect(workflowStore.currentWorkflow?.connections.Webhook?.main?.[0]).toEqual([
      { node: 'Set', type: 'main', index: 0 },
    ])
  })

  it('removes a connection through the dedicated action as well', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()
    workflowStore.setCurrentWorkflow(connectedWorkflow())

    editorStore.selectEdge(EDGE_ID)
    const removed = editorStore.removeSelectedEdges()

    expect(removed).toBe(1)
    expect(workflowStore.currentWorkflow?.connections.Webhook).toBeUndefined()
  })

  it('removes the connections of a deleted node as well', () => {
    const workflowStore = useWorkflowStore()
    const editorStore = useWorkflowEditorStore()
    workflowStore.setCurrentWorkflow(connectedWorkflow())

    editorStore.selectNode('node_webhook')
    editorStore.removeSelectedNodes()

    expect(workflowStore.currentWorkflow?.nodes).toHaveLength(1)
    expect(workflowStore.currentWorkflow?.connections.Webhook).toBeUndefined()
  })
})