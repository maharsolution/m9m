import apiClient from './client'
import type { NodeType } from '@/types/node'

/**
 * Node-type catalog client.
 *
 * `GET /api/v1/node-types` returns the metadata that drives the whole
 * parameter panel: properties, displayOptions, typeOptions and the
 * credential slots each node declares.
 */
export async function getNodeTypes(): Promise<NodeType[]> {
  const response = await apiClient.get<NodeType[]>('/node-types')
  return response.data
}

export async function getNodeType(name: string): Promise<NodeType> {
  const response = await apiClient.get<NodeType>(`/node-types/${encodeURIComponent(name)}`)
  return response.data
}

interface NodeTypeOption {
  name: string
  value: string
}

function unwrapOptions(payload: unknown): NodeTypeOption[] {
  if (Array.isArray(payload)) return payload as NodeTypeOption[]
  if (payload && typeof payload === 'object') {
    const record = payload as { data?: unknown }
    if (Array.isArray(record.data)) return record.data as NodeTypeOption[]
  }
  return []
}

/**
 * Dynamic option list for a node property (n8n's `loadOptions` semantics).
 *
 * The backend endpoint is optional. We try the GET form first and fall
 * back to the POST form described in the parity plan; when neither
 * exists the request fails and the caller degrades to a static/plain
 * input rather than breaking the panel.
 */
export async function getNodeTypeOptions(
  name: string,
  property: string,
  params: Record<string, unknown> = {},
): Promise<NodeTypeOption[]> {
  const url = `/node-types/${encodeURIComponent(name)}/options`

  try {
    const response = await apiClient.get(url, {
      params: { property },
    })
    return unwrapOptions(response.data)
  } catch (getError) {
    const status = (getError as { response?: { status?: number } })?.response?.status
    if (status !== 404 && status !== 405) throw getError

    const response = await apiClient.post(url, { property, params })
    return unwrapOptions(response.data)
  }
}
