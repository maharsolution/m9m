import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { Workflow, WorkflowNode } from '@/types'
import { useWorkflowStore } from './workflow'
import {
  addConnectionByNodeId,
  addNodeToWorkflow,
  createWorkflowDraft,
  removeConnectionByNodeId,
  removeNodeFromWorkflow,
  updateNodeInWorkflow,
} from '@/lib/workflowGraph'

export const useWorkflowEditorStore = defineStore('workflowEditor', () => {
  const workflowStore = useWorkflowStore()

  const selectedNodeIds = ref<string[]>([])
  const isDirty = ref(false)

  const selectedNodes = computed(() => {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return []
    }

    return workflow.nodes.filter((node) => selectedNodeIds.value.includes(node.id))
  })

  const selectedNode = computed(() => {
    const workflow = workflowStore.currentWorkflow
    if (!workflow || selectedNodeIds.value.length !== 1) {
      return null
    }

    return workflow.nodes.find((node) => node.id === selectedNodeIds.value[0]) ?? null
  })

  function resetEditorState() {
    selectedNodeIds.value = []
    isDirty.value = false
  }

  function markDirty() {
    isDirty.value = true
  }

  function markClean() {
    isDirty.value = false
  }

  function createNewWorkflow(sourceWorkflow?: Workflow, overrides: Partial<Workflow> = {}) {
    workflowStore.setCurrentWorkflow(createWorkflowDraft(sourceWorkflow, overrides))
    selectedNodeIds.value = []
    isDirty.value = true
  }

  function addNode(node: WorkflowNode) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    addNodeToWorkflow(workflow, node)
    selectedNodeIds.value = [node.id]
    isDirty.value = true
  }

  function updateNode(nodeId: string, updates: Partial<WorkflowNode>) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    // Special handling for `credentials`: merge instead of replace, so a
    // caller can bind one credential type without dropping the others.
    if (updates.credentials) {
      const existing = workflow.nodes.find((n) => n.id === nodeId)
      if (existing) {
        updates = {
          ...updates,
          credentials: {
            ...(existing.credentials ?? {}),
            ...updates.credentials,
          },
        }
      }
    }

    if (updateNodeInWorkflow(workflow, nodeId, updates)) {
      isDirty.value = true
    }
  }

  /**
   * Bind a credential to a node. Pass `null` for the credential id to
   * unbind that type. Other credential types already bound to the node
   * are preserved.
   */
  function bindCredential(
    nodeId: string,
    credType: string,
    credential: { id: string; name: string } | null
  ) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) return

    const node = workflow.nodes.find((n) => n.id === nodeId)
    if (!node) return

    const next = { ...(node.credentials ?? {}) } as Record<
      string,
      { id?: string; name: string }
    >
    if (credential === null) {
      delete next[credType]
    } else {
      next[credType] = { id: credential.id, name: credential.name }
    }
    updateNode(nodeId, { credentials: next as any })
  }

  function removeNode(nodeId: string) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    if (removeNodeFromWorkflow(workflow, nodeId)) {
      selectedNodeIds.value = selectedNodeIds.value.filter((id) => id !== nodeId)
      isDirty.value = true
    }
  }

  function addConnection(sourceNodeId: string, targetNodeId: string, sourceOutput = 0, targetInput = 0) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    if (addConnectionByNodeId(workflow, sourceNodeId, targetNodeId, sourceOutput, targetInput)) {
      isDirty.value = true
    }
  }

  function removeConnection(sourceNodeId: string, targetNodeId: string, sourceOutput = 0, targetInput = 0) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    if (removeConnectionByNodeId(workflow, sourceNodeId, targetNodeId, sourceOutput, targetInput)) {
      isDirty.value = true
    }
  }

  function selectNode(nodeId: string, addToSelection = false) {
    if (addToSelection) {
      if (selectedNodeIds.value.includes(nodeId)) {
        selectedNodeIds.value = selectedNodeIds.value.filter((id) => id !== nodeId)
      } else {
        selectedNodeIds.value = [...selectedNodeIds.value, nodeId]
      }
      return
    }

    selectedNodeIds.value = [nodeId]
  }

  function selectNodes(nodeIds: string[]) {
    selectedNodeIds.value = [...nodeIds]
  }

  function clearSelection() {
    selectedNodeIds.value = []
  }

  function setWorkflowName(name: string) {
    const workflow = workflowStore.currentWorkflow
    if (!workflow) {
      return
    }

    workflow.name = name
    isDirty.value = true
  }

  return {
    selectedNodeIds,
    selectedNodes,
    selectedNode,
    isDirty,
    resetEditorState,
    markDirty,
    markClean,
    createNewWorkflow,
    addNode,
    updateNode,
    removeNode,
    bindCredential,
    addConnection,
    removeConnection,
    selectNode,
    selectNodes,
    clearSelection,
    setWorkflowName,
  }
})
