import apiClient from './client'
import type { OtelConfigView, OtelOverride } from '@/types/otel'

// GET /api/v1/otel — returns the three-layer view (env defaults, the
// persisted override, and the merged effective config). The UI shows
// every field from `effective` but pulls the per-field "use env
// default" checkbox from `override`'s undefined entries.
export async function getOtelConfig(): Promise<OtelConfigView> {
  const response = await apiClient.get<OtelConfigView>('/otel')
  return response.data
}

// PUT /api/v1/otel — persists the per-field override and triggers a
// tracer-provider reload. Returns the new merged view.
export async function updateOtelConfig(override: OtelOverride): Promise<OtelConfigView> {
  const response = await apiClient.put<OtelConfigView>('/otel', override)
  return response.data
}

// DELETE /api/v1/otel — clears the persisted override, restoring the
// env-driven defaults. The handler reloads the tracer as a side effect.
export async function clearOtelConfig(): Promise<OtelConfigView> {
  const response = await apiClient.delete<OtelConfigView>('/otel')
  return response.data
}

// POST /api/v1/otel/test — emits a `m9m.otel.test` no-op span to the
// configured collector. Returns 204 on success. Throws on failure so
// the UI can show "did the test reach the collector" feedback.
export async function sendOtelTestSpan(): Promise<void> {
  await apiClient.post('/otel/test')
}
