import apiClient from './client'
import type {
  Credential,
  CredentialCreate,
  CredentialSchema,
  CredentialTestResult,
  CredentialTypeSummary,
} from '@/types/api'

interface CredentialListResponse {
  data: Credential[]
  nextCursor?: string | null
}

export async function getCredentials(limit = 100): Promise<Credential[]> {
  const response = await apiClient.get<CredentialListResponse>('/credentials', {
    params: { limit },
  })
  return response.data.data
}

export async function getCredential(id: string): Promise<Credential> {
  const response = await apiClient.get<Credential>(`/credentials/${id}`)
  return response.data
}

export async function createCredential(
  credential: CredentialCreate
): Promise<Credential> {
  const response = await apiClient.post<Credential>('/credentials', credential)
  return response.data
}

export async function updateCredential(
  id: string,
  credential: Partial<CredentialCreate>
): Promise<Credential> {
  // PATCH for partial update (n8n parity).
  const response = await apiClient.patch<Credential>(`/credentials/${id}`, credential)
  return response.data
}

export async function deleteCredential(id: string): Promise<void> {
  await apiClient.delete(`/credentials/${id}`)
}

export async function getCredentialSchema(
  type: string
): Promise<CredentialSchema> {
  const response = await apiClient.get<CredentialSchema>(
    `/credentials/schema/${encodeURIComponent(type)}`
  )
  return response.data
}

/**
 * Every credential type the backend ships a schema for. The UI's type
 * chooser is built from this list so it can never drift from the
 * backend (mongoDb / kafka were missing from the old hardcoded list).
 */
export async function getCredentialTypes(): Promise<CredentialTypeSummary[]> {
  const response = await apiClient.get<{ data: CredentialTypeSummary[] }>(
    '/credentials/types'
  )
  return response.data.data
}

/**
 * Validate a credential payload that has NOT been saved yet. The
 * backend checks the exact values currently in the form, so the test
 * button honours unsaved edits.
 */
export async function testCredentialPayload(
  type: string,
  data: Record<string, unknown>
): Promise<CredentialTestResult> {
  const response = await apiClient.post<CredentialTestResult>(
    '/credentials/test',
    { type, data }
  )
  return response.data
}

export async function testCredential(
  id: string
): Promise<CredentialTestResult> {
  const response = await apiClient.post<CredentialTestResult>(
    `/credentials/${id}/test`
  )
  return response.data
}

export async function transferCredential(
  id: string,
  destinationProjectId: string
): Promise<Credential> {
  const response = await apiClient.put<Credential>(
    `/credentials/${id}/transfer`,
    { destinationProjectId }
  )
  return response.data
}
