import apiClient from './client'
import type {
  Credential,
  CredentialCreate,
  CredentialSchema,
  CredentialTestResult,
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
