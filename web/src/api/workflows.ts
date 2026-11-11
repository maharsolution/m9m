import apiClient from './client'
import type { Workflow, WorkflowFilters, DataItem, N8nWorkflowDocument } from '@/types'
import type {
  PaginatedResponse,
  WorkflowExportFile,
  WorkflowImportSummary,
} from '@/types/api'

export async function getWorkflows(filters?: WorkflowFilters): Promise<PaginatedResponse<Workflow>> {
  const params = new URLSearchParams()
  if (filters?.search) params.append('search', filters.search)
  if (filters?.active !== undefined) params.append('active', String(filters.active))
  if (filters?.offset !== undefined) params.append('offset', String(filters.offset))
  if (filters?.limit !== undefined) params.append('limit', String(filters.limit))

  const response = await apiClient.get<PaginatedResponse<Workflow>>('/workflows', { params })
  return response.data
}

export async function getWorkflow(id: string): Promise<Workflow> {
  const response = await apiClient.get<Workflow>(`/workflows/${id}`)
  return response.data
}

export async function createWorkflow(workflow: Partial<Workflow>): Promise<Workflow> {
  const response = await apiClient.post<Workflow>('/workflows', workflow)
  return response.data
}

export async function updateWorkflow(id: string, workflow: Partial<Workflow>): Promise<Workflow> {
  const response = await apiClient.put<Workflow>(`/workflows/${id}`, workflow)
  return response.data
}

export async function deleteWorkflow(id: string): Promise<void> {
  await apiClient.delete(`/workflows/${id}`)
}

export async function activateWorkflow(id: string): Promise<{ message: string; active: boolean }> {
  const response = await apiClient.post<{ message: string; active: boolean }>(`/workflows/${id}/activate`)
  return response.data
}

export async function deactivateWorkflow(id: string): Promise<{ message: string; active: boolean }> {
  const response = await apiClient.post<{ message: string; active: boolean }>(`/workflows/${id}/deactivate`)
  return response.data
}

export async function executeWorkflow(id: string, inputData?: DataItem[]): Promise<import('@/types').WorkflowExecution> {
  const response = await apiClient.post(`/workflows/${id}/execute`, inputData)
  return response.data
}

export async function duplicateWorkflow(id: string): Promise<Workflow> {
  const original = await getWorkflow(id)
  const duplicate: Partial<Workflow> = {
    ...original,
    id: undefined,
    name: `${original.name} (copy)`,
    active: false,
    createdAt: undefined,
    updatedAt: undefined,
  }
  return createWorkflow(duplicate)
}

/**
 * How the `active` flag of an imported document is treated.
 *
 * - `false` (default): every imported workflow is stored inactive,
 *   exactly like n8n's `import:workflow --activeState=false`.
 * - `fromJson`: the `active` flag of the document is honoured, like
 *   n8n's `--activeState=fromJson`.
 */
export type WorkflowImportActiveState = 'false' | 'fromJson'

/**
 * Reads the file name out of a Content-Disposition header.
 */
function filenameFromDisposition(disposition: string | undefined, fallback: string): string {
  if (!disposition) return fallback

  const utf8 = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  if (utf8?.[1]) {
    try {
      return decodeURIComponent(utf8[1].trim())
    } catch {
      // Malformed percent-encoding: fall through to the plain parameter.
    }
  }

  const plain = /filename="?([^";]+)"?/i.exec(disposition)
  return plain?.[1]?.trim() || fallback
}

/**
 * Downloads one workflow in the n8n wire format.
 *
 * GET /workflows/{id}/export answers with a SINGLE workflow object
 * (n8n editor "Download" parity).
 */
export async function exportWorkflow(id: string): Promise<WorkflowExportFile> {
  const response = await apiClient.get(`/workflows/${id}/export`, { responseType: 'blob' })
  return {
    blob: response.data as Blob,
    filename: filenameFromDisposition(response.headers['content-disposition'], `${id}.json`),
  }
}

/**
 * Downloads several workflows in the n8n wire format.
 *
 * POST /workflows/export answers with an ARRAY of workflow objects,
 * also for a single id (n8n CLI `export:workflow` parity).
 */
export async function exportWorkflows(ids: string[]): Promise<WorkflowExportFile> {
  const response = await apiClient.post(
    '/workflows/export',
    { ids },
    { responseType: 'blob' }
  )
  return {
    blob: response.data as Blob,
    filename: filenameFromDisposition(
      response.headers['content-disposition'],
      'workflows-export.json'
    ),
  }
}

/**
 * Imports an n8n workflow document uploaded as a .json file
 * (multipart/form-data).
 */
export async function importWorkflowFile(
  file: File,
  options?: { activeState?: WorkflowImportActiveState }
): Promise<WorkflowImportSummary> {
  const formData = new FormData()
  formData.append('file', file, file.name)

  const response = await apiClient.post<WorkflowImportSummary>('/workflows/import', formData, {
    // The axios instance defaults to `Content-Type: application/json`,
    // which makes axios turn FormData into JSON. Setting the multipart
    // type keeps the FormData intact; axios drops the header again
    // before the request is sent, so the browser adds the boundary.
    headers: { 'Content-Type': 'multipart/form-data' },
    params: options?.activeState ? { activeState: options.activeState } : undefined,
  })
  return response.data
}

/**
 * Imports an n8n workflow document that is already in memory. The
 * document may be a single workflow object or an array of workflow
 * objects.
 */
export async function importWorkflowDocument(
  document: N8nWorkflowDocument | N8nWorkflowDocument[],
  options?: { activeState?: WorkflowImportActiveState }
): Promise<WorkflowImportSummary> {
  const response = await apiClient.post<WorkflowImportSummary>('/workflows/import', document, {
    params: options?.activeState ? { activeState: options.activeState } : undefined,
  })
  return response.data
}

/**
 * Hands an exported workflow file to the browser as a download.
 */
export function downloadWorkflowExport(file: WorkflowExportFile): void {
  const url = URL.createObjectURL(file.blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = file.filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}

/**
 * Extracts a human-readable message from an API error. The export
 * endpoints answer with `responseType: 'blob'`, so an error body is
 * a Blob and has to be read asynchronously.
 */
export async function apiErrorMessage(error: unknown, fallback: string): Promise<string> {
  const response = (error as { response?: { data?: unknown } })?.response
  const data = response?.data

  if (typeof data === 'string' && data.trim()) return data

  if (typeof Blob !== 'undefined' && data instanceof Blob) {
    try {
      const text = await data.text()
      if (!text.trim()) return fallback
      try {
        const parsed = JSON.parse(text)
        if (parsed?.message) return String(parsed.message)
      } catch {
        // Not a JSON body - use the raw text.
      }
      return text
    } catch {
      return fallback
    }
  }

  if (data && typeof data === 'object' && 'message' in data) {
    const message = (data as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }

  return fallback
}
