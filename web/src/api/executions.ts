import apiClient from './client'
import type { WorkflowExecution, ExecutionFilters, DataItem } from '@/types'
import type { PaginatedResponse } from '@/types/api'

export async function getExecutions(filters?: ExecutionFilters): Promise<PaginatedResponse<WorkflowExecution>> {
  const params = new URLSearchParams()
  if (filters?.workflowId) params.append('workflowId', filters.workflowId)
  if (filters?.status) params.append('status', filters.status)
  if (filters?.offset !== undefined) params.append('offset', String(filters.offset))
  if (filters?.limit !== undefined) params.append('limit', String(filters.limit))

  const response = await apiClient.get<PaginatedResponse<WorkflowExecution>>('/executions', { params })
  return response.data
}

export async function getExecution(id: string): Promise<WorkflowExecution> {
  const response = await apiClient.get<WorkflowExecution>(`/executions/${id}`)
  return response.data
}

export async function deleteExecution(id: string): Promise<void> {
  await apiClient.delete(`/executions/${id}`)
}

export async function retryExecution(id: string): Promise<WorkflowExecution> {
  const response = await apiClient.post<WorkflowExecution>(`/executions/${id}/retry`)
  return response.data
}

// RetryNodeRequest mirrors the backend's `api.RetryNodeRequest`
// struct. `inputData` is optional — when omitted, the backend
// replays the upstream node's last successful output from the
// saved NodeData snapshot. Pass `inputData` to override the
// upstream payload (matches n8n's NDV "Run with edited input"
// affordance).
export interface RetryNodeRequest {
  nodeName: string
  inputData?: DataItem[]
  mode?: string
}

// retryNode asks the backend to re-run an execution starting from
// `nodeName` instead of from the workflow's start node. The target
// node AND every node downstream of it will execute again; nodes
// upstream of the target are skipped and their saved output is
// replayed as the target's input.
//
// Returns the new execution record (with mode="retry-node" and a
// Metadata.parentExecutionId pointing back at the original).
export async function retryNode(
  executionId: string,
  req: RetryNodeRequest
): Promise<WorkflowExecution> {
  const response = await apiClient.post<WorkflowExecution>(
    `/executions/${executionId}/retry-node`,
    req
  )
  return response.data
}

export async function cancelExecution(id: string): Promise<{ message: string; status: string }> {
  const response = await apiClient.post<{ message: string; status: string }>(`/executions/${id}/cancel`)
  return response.data
}
