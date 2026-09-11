// AI config types — mirror internal/ai/config.go + providers.go on the
// Go side. Field name convention is camelCase to match the JSON the
// backend returns from GET /api/v1/ai. Optional fields (the override
// layer) are marked `?` because they are absent / null when the user
// has not overridden the env default.

/**
 * The set of providers the UI can pick from. Keep this list in sync
 * with `Providers` in internal/ai/providers.go — the API also sends
 * a full provider catalog on every GET, so the UI is not strictly
 * required to maintain this, but it lets us render the Settings page
 * even before the first backend call returns.
 */
export type AIProviderId = 'openai' | 'anthropic' | 'minimax' | 'ollama'

export interface AIProviderInfo {
  id: AIProviderId
  label: string
  description: string
  defaultBaseUrl: string
  defaultModel: string
  models: string[]
  requiresKey: boolean
}

/**
 * The effective AI configuration the runtime uses. Every field is
 * populated (no nulls) because it is computed by merging env defaults
 * with the persisted override.
 */
export interface AIConfig {
  enabled: boolean
  provider: AIProviderId
  apiKey: string
  baseUrl: string
  model: string
  maxTokens: number
  temperature: number
  /** timeout in milliseconds */
  timeoutMs: number
}

/**
 * Override layer. Each field is tri-state:
 *   - omitted / null  → "follow env default" (no override)
 *   - explicit value  → override the env default
 *
 * Matches the Go side's `*bool` / `*string` / `*float64` pointer fields
 * in `ConfigOverride`. The UI never has to send a value just to leave
 * it unchanged.
 */
export interface AIOverride {
  enabled?: boolean | null
  provider?: AIProviderId | null
  apiKey?: string | null
  baseUrl?: string | null
  model?: string | null
  maxTokens?: number | null
  temperature?: number | null
  timeoutMs?: number | null
  source?: string | null
}

/**
 * The view the UI receives from GET /api/v1/ai: three config layers
 * (env, override, effective) plus the full provider catalog so the
 * dropdowns can be rendered without a second round-trip.
 */
export interface AIConfigView {
  env: AIConfig
  override: AIOverride
  effective: AIConfig
  providers: AIProviderInfo[]
}
