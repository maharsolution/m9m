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