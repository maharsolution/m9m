// OTEL config types — mirror internal/otel/config.go on the Go side.
// Field name convention is camelCase to match the JSON the backend
// returns from GET /api/v1/otel. Optional fields (the override layer)
// are marked `?` because they are absent / null when the user has not
// overridden the env default.

export type OtelProtocol = 'grpc' | 'http/protobuf'

export interface OtelConfig {
  enabled: boolean
  endpoint: string
  protocol: OtelProtocol
  headers: string
  headersFile: string
  sampleRate: number
  productionOnly: boolean
  includeNodeSpans: boolean
  injectOutbound: boolean
  serviceName: string
  serviceVersion: string
  agentsEnabled: boolean
  agentsRecordInputs: boolean
  agentsRecordOutputs: boolean
}

// Override layer. Each field is tri-state:
//   - omitted / null  → "follow env default" (no override)
//   - explicit value  → override the env default
// This matches the Go side's `*bool` / `*string` / `*float64` pointer
// fields. The UI never has to send a value just to leave it unchanged.
export interface OtelOverride {
  enabled?: boolean | null
  endpoint?: string | null
  protocol?: OtelProtocol | null
  headers?: string | null
  headersFile?: string | null
  sampleRate?: number | null
  serviceName?: string | null
  serviceVersion?: string | null
  productionOnly?: boolean | null
  includeNodeSpans?: boolean | null
  injectOutbound?: boolean | null
  agentsEnabled?: boolean | null
  agentsRecordInputs?: boolean | null
  agentsRecordOutputs?: boolean | null
}

// The view the UI receives from GET /api/v1/otel: three layers so
// the UI can show "what would happen with no override" alongside the
// merged effective value.
export interface OtelConfigView {
  env: OtelConfig
  override: OtelOverride
  effective: OtelConfig
}
