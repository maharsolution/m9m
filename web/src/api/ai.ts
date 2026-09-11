import apiClient from './client'
import type { AIConfigView, AIOverride } from '@/types/ai'

/**
 * GET /api/v1/ai — returns the three-layer view (env defaults, the
 * persisted override, the merged effective config) plus the provider
 * catalog. The Settings page renders `providers` to drive the
 * provider/model dropdowns.
 */
export async function getAIConfig(): Promise<AIConfigView> {
  const response = await apiClient.get<AIConfigView>('/ai')
  return response.data
}

/**
 * PUT /api/v1/ai — persists the per-field override and triggers a
 * live reload of the AI runtime. Returns the new merged view so the
 * UI can refresh without a second GET.
 */
export async function updateAIConfig(override: AIOverride): Promise<AIConfigView> {
  const response = await apiClient.put<AIConfigView>('/ai', override)
  return response.data
}

/**
 * DELETE /api/v1/ai — clears the persisted override, restoring the
 * env-driven defaults. The handler also applies the env-only config
 * to the runtime so the change takes effect without a restart.
 */
export async function clearAIConfig(): Promise<AIConfigView> {
  const response = await apiClient.delete<AIConfigView>('/ai')
  return response.data
}

/**
 * POST /api/v1/ai/test — sends a one-shot chat prompt through the
 * currently-configured AI service. Used by the "Send test chat"
 * button in Settings → AI to confirm the provider, key, and base URL
 * all line up before the operator saves the override.
 */
export async function sendAITestChat(prompt: string): Promise<{ message: string; elapsed: string }> {
  const response = await apiClient.post<{ message: string; elapsed: string }>('/ai/test', { prompt })
  return response.data
}

/**
 * GET /api/v1/ai/health — quick liveness probe for the AI configuration.
 * Returns 200 with `{ enabled, provider, model, baseUrl }` so the
 * Settings card can render a status pill without parsing the full
 * config view.
 */
export async function getAIHealth(): Promise<{ enabled: boolean; provider?: string; model?: string; baseUrl?: string; reason?: string }> {
  const response = await apiClient.get<{ enabled: boolean; provider?: string; model?: string; baseUrl?: string; reason?: string }>('/ai/health')
  return response.data
}
