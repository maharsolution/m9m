export interface ApiResponse<T> {
  data: T
  total?: number
  offset?: number
  limit?: number
}

export interface ApiError {
  error: boolean
  message: string
  code: number
  details?: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  offset: number
  limit: number
}

export interface HealthResponse {
  status: string
  service: string
  version: string
  time: string
}

export interface VersionResponse {
  n8nVersion: string
  serverVersion: string
  implementation: string
  compatibility: {
    workflows: boolean
    nodes: boolean
    expressions: boolean
    credentials: boolean
  }
}

export interface SettingsResponse {
  timezone: string
  executionMode: string
  saveDataSuccessExecution: string
  saveDataErrorExecution: string
  saveExecutionProgress: boolean
  saveManualExecutions: boolean
  communityNodesEnabled: boolean
  versionNotifications: {
    enabled: boolean
  }
  instanceId: string
  telemetry: {
    enabled: boolean
  }
}

export interface Credential {
  id: string
  name: string
  type: string
  /**
   * The credential's secret fields. Returned by the single-credential
   * endpoint (GET /credentials/:id) so the edit form can be pre-filled
   * with the stored values; the list endpoint leaves it undefined.
   */
  data?: Record<string, unknown>
  isManaged?: boolean
  isGlobal?: boolean
  isResolvable?: boolean
  resolvableAllowFallback?: boolean
  resolverId?: string
  createdAt: string
  updatedAt: string
}

export interface CredentialTypeSummary {
  type: string
  displayName: string
}

export interface CredentialCreate {
  name: string
  type: string
  data: Record<string, unknown>
  isGlobal?: boolean
}

export interface CredentialTestResult {
  status: 'OK' | 'Error'
  message: string
}

export interface CredentialSchemaPropertyOption {
  name: string
  value: string | number | boolean
}

export interface CredentialSchemaProperty {
  name: string
  displayName: string
  type: 'string' | 'password' | 'number' | 'boolean' | 'options' | 'json'
  required: boolean
  default?: string | number | boolean | Record<string, unknown>
  placeholder?: string
  description?: string
  options?: CredentialSchemaPropertyOption[]
}

export interface CredentialSchema {
  type: string
  displayName: string
  properties: CredentialSchemaProperty[]
  required: string[]
  displayOptions?: Record<string, boolean>
}

export interface Tag {
  id: string
  name: string
  createdAt: string
  updatedAt: string
}

export interface WebSocketMessage {
  type: WebSocketMessageType
  data: Record<string, unknown>
  timestamp?: number
}

export type WebSocketMessageType =
  | 'connected'
  | 'executionUpdate'
  | 'nodeExecution'
  | 'workflowActivated'
  | 'workflowDeactivated'
  | 'response'
  | 'error'

export interface ExecutionUpdateMessage {
  executionId: string
  workflowId: string
  status: string
  startedAt?: string
  finishedAt?: string
  error?: string
}

export interface NodeExecutionMessage {
  executionId: string
  nodeId: string
  status: string
  data?: Record<string, unknown>
}

/**
 * One workflow that was imported successfully.
 */
export interface WorkflowImportItem {
  index: number
  id: string
  name: string
  /** The workflow id carried by the source document (traceability). */
  sourceId?: string
  /** The fresh version id m9m assigned to the import. */
  versionId?: string
  active: boolean
  /** Non-fatal problems found while converting the workflow. */
  warnings?: string[]
}

/**
 * One workflow of an import batch that was rejected. A bad entry
 * never aborts the rest of the batch.
 */
export interface WorkflowImportError {
  index: number
  name?: string
  message: string
}

/**
 * Response body of `POST /workflows/import`.
 */
export interface WorkflowImportSummary {
  imported: number
  failed: number
  /** "fromJson" when the JSON active flag was honoured, else "false". */
  activeState: string
  workflows: WorkflowImportItem[]
  errors: WorkflowImportError[]
}

/**
 * Response body of a rejected `POST /workflows/import`.
 *
 * The endpoint answers 400 when no workflow of the batch could be
 * imported. It repeats the summary fields at the top level so the
 * per-item errors are readable without digging into `summary`.
 */
export interface WorkflowImportFailure {
  error: boolean
  message: string
  code: number
  imported: number
  failed: number
  activeState: string
  workflows: WorkflowImportItem[]
  errors: WorkflowImportError[]
  summary?: WorkflowImportSummary
}

/**
 * A downloadable export: the JSON payload plus the file name the
 * server suggested through the Content-Disposition header.
 */
export interface WorkflowExportFile {
  blob: Blob
  filename: string
}
