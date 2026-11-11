export interface NodeType {
  name: string
  displayName: string
  description: string
  /** Backend-provided category (e.g. "Core", "Transform"). Empty string when the
   *  backend does not report one. */
  category?: string
  version: number
  defaults: NodeDefaults
  inputs: string[]
  outputs: string[]
  properties?: NodeProperty[]
  /** Credential slots the node can be bound to. The backend always emits the
   *  array (empty when the node declares none). */
  credentials?: NodeCredentialType[]
  /** Icon identifiers (Heroicons-style names). "" when the node has none. */
  icon?: string
  iconColor?: string
  /** n8n-style grouping (e.g. ["transform"], ["trigger"]). [] when none. */
  group?: string[]
  usableAsTool?: boolean
  subtitle?: string
}

export interface NodeDefaults {
  name: string
  color?: string
}

export interface NodeProperty {
  displayName: string
  name: string
  type: NodePropertyType
  default?: unknown
  required?: boolean
  description?: string
  placeholder?: string
  options?: NodePropertyOption[]
  displayOptions?: NodePropertyDisplayOptions
  typeOptions?: NodePropertyTypeOptions
  /**
   * Allowed values of the property's primary sub-field.
   *
   * The backend emits this list for the composite property types:
   * `resourceMapper` (mapping modes), `filter` (comparison operators),
   * `file` (binary-property modes) and `icon` (the curated icon set).
   */
  values?: NodePropertyOptionValue[]
}

/**
 * One allowed value of a composite property's sub-field.
 *
 * The wire shape mirrors the backend's `OptionValue`:
 * `{ name, value, description }`.
 */
export interface NodePropertyOptionValue {
  name: string
  value: string
  description?: string
}

export type NodePropertyType =
  | 'string'
  | 'number'
  | 'boolean'
  | 'options'
  | 'multiOptions'
  | 'collection'
  | 'fixedCollection'
  | 'json'
  | 'code'
  | 'dateTime'
  | 'color'
  | 'resourceLocator'
  | 'resourceMapper'
  | 'filter'
  | 'file'
  | 'icon'

export interface NodePropertyOption {
  name: string
  value: string | number | boolean
  description?: string
  /** Backend action name used to fetch this option list dynamically
   *  (n8n's `loadOptions` semantics). Empty for static lists. */
  action?: string
  // n8n-style nested sub-property definitions. Only present when the
  // backend emits a `collection` / `fixedCollection` with child properties
  // (n8n's `INodePropertyOptions` / `INodePropertyCollection`).
  type?: string
  displayName?: string
  default?: unknown
  required?: boolean
  placeholder?: string
  options?: NodePropertyOption[]
  typeOptions?: NodePropertyTypeOptions
  displayOptions?: NodePropertyDisplayOptions
  /**
   * Child field definitions of a collection bucket (n8n's
   * `INodePropertyCollection.values`). When present the collection
   * renderer builds one control per declared field instead of falling
   * back to the generic name/value rows.
   */
  values?: NodePropertyOption[]
}

/**
 * n8n `displayOptions` — `{ show: { <param>: [<values>] }, hide: { <param>: [<values>] } }`.
 *
 * The field is visible when every `show` condition matches and no `hide`
 * condition matches. A property without `displayOptions` is always visible.
 */
export interface NodePropertyDisplayOptions {
  show?: Record<string, unknown[]>
  hide?: Record<string, unknown[]>
}

/**
 * Free-form per-property hints. The backend sends whatever the node author
 * declared (`multipleValues`, `rows`, `minValue`, `maxValue`,
 * `numberPrecision`, `password`, `loadOptionsMethod`, …), so unknown keys are
 * allowed and preserved.
 */
export interface NodePropertyTypeOptions {
  multipleValues?: boolean
  multipleValueButtonText?: string
  maxValue?: number
  minValue?: number
  numberPrecision?: number
  password?: boolean
  rows?: number
  alwaysOpenEditWindow?: boolean
  /** n8n resource-locator modes, when the property is a `resourceLocator`. */
  modes?: NodePropertyOption[]
  /** n8n `loadOptions` method name for dynamic option lists. */
  loadOptionsMethod?: string
  [key: string]: unknown
}

/**
 * A credential slot declared by a node type.
 *
 * `type` is the canonical credential-type identifier; `name` is the
 * deprecated alias the backend still emits for backwards compatibility.
 */
export interface NodeCredentialType {
  /** Canonical credential type id (e.g. "httpBasicAuth"). */
  type?: string
  /** Human-readable label shown in the credential picker. */
  displayName?: string
  required?: boolean
  /** @deprecated alias of `type`, kept for backwards compatibility. */
  name?: string
  displayOptions?: NodePropertyDisplayOptions
}

/** A parameter value as stored in `node.parameters`. */
export type NodeParameterValue =
  | string
  | number
  | boolean
  | null
  | NodeParameterValue[]
  | { [key: string]: NodeParameterValue }
  | undefined

/** Map of parameter name → value, as stored on a workflow node. */
export type NodeParameters = Record<string, NodeParameterValue>

export type NodeCategory =
  | 'trigger'
  | 'action'
  | 'transform'
  | 'flow'
  | 'core'
  | 'data'
  | 'communication'
  | 'marketing'
  | 'productivity'
  | 'sales'
  | 'development'
  | 'utility'

export interface NodeCategoryInfo {
  name: NodeCategory
  displayName: string
  icon: string
  color: string
}

export const NODE_CATEGORIES: NodeCategoryInfo[] = [
  { name: 'trigger', displayName: 'Triggers', icon: 'bolt', color: 'green' },
  { name: 'action', displayName: 'Actions', icon: 'play', color: 'indigo' },
  { name: 'transform', displayName: 'Transform', icon: 'arrows-right-left', color: 'amber' },
  { name: 'flow', displayName: 'Flow', icon: 'share', color: 'purple' },
  { name: 'core', displayName: 'Core', icon: 'cube', color: 'slate' },
  { name: 'data', displayName: 'Data', icon: 'database', color: 'blue' },
  { name: 'communication', displayName: 'Communication', icon: 'chat-bubble-left-right', color: 'pink' },
  { name: 'utility', displayName: 'Utility', icon: 'wrench', color: 'gray' },
]

export function getNodeCategory(nodeType: string): NodeCategory {
  // Triggers
  if (nodeType.includes('trigger') || nodeType.includes('webhook') || nodeType.includes('cron') || nodeType.includes('errorTrigger')) {
    return 'trigger'
  }
  // Transform
  if (nodeType.includes('function') || nodeType.includes('code') || nodeType.includes('set') || nodeType.includes('filter')) {
    return 'transform'
  }
  // Flow control
  if (nodeType.includes('if') || nodeType.includes('switch') || nodeType.includes('merge') || nodeType.includes('split') || nodeType.includes('wait') || nodeType.includes('loop') || nodeType.includes('noOp')) {
    return 'flow'
  }
  // Core
  if (nodeType.includes('start') || nodeType.includes('executeWorkflow')) {
    return 'core'
  }
  // Data / Database. Case-insensitive so `n8n-nodes-base.mySql`
  // (camelCase n8n convention) lands in the Data section alongside
  // `n8n-nodes-base.mongoDb` — previously the lowercase substring
  // check missed MySQL and it fell through to the action fallback,
  // making it the only DB node rendered under "Actions".
  const lc = nodeType.toLowerCase()
  if (lc.includes('postgres') || lc.includes('mysql') || lc.includes('sqlite') || lc.includes('mongo') || lc.includes('redis') || lc.includes('elastic')) {
    return 'data'
  }
  // Communication
  if (nodeType.includes('slack') || nodeType.includes('discord') || nodeType.includes('twilio') || nodeType.includes('sendGrid') || nodeType.includes('teams') || nodeType.includes('email')) {
    return 'communication'
  }
  // Productivity
  if (nodeType.includes('notion') || nodeType.includes('stripe') || nodeType.includes('googleSheets')) {
    return 'productivity'
  }
  return 'action'
}

/**
 * FALLBACK credential-type table.
 *
 * This mirrors n8n's shipped nodes so the UI can render the credential bind
 * UI even when the backend metadata does not declare any credential slot.
 *
 * It is a fallback only: the node-type metadata (`credentials[]`) is
 * authoritative whenever it declares at least one credential slot. See
 * `resolveNodeCredentialTypes` in `@/lib/credentialSchema`.
 *
 * The map is keyed by the long n8n node-type name (e.g.
 * `n8n-nodes-base.httpRequest`) but the lookup also accepts the short name
 * (`httpRequest`) for convenience.
 */
const NODE_CREDENTIAL_TYPE_MAP: Record<string, string[]> = {
  // HTTP
  'n8n-nodes-base.httpRequest': ['httpBasicAuth', 'httpHeaderAuth', 'oAuth2Api'],
  // Databases
  'n8n-nodes-base.postgres': ['postgres'],
  'n8n-nodes-base.mySql': ['mySql'],
  'n8n-nodes-base.sqlite': [],
  'n8n-nodes-base.mongoDb': ['mongoDb'],
  'n8n-nodes-base.redis': [],
  // Messaging
  'n8n-nodes-base.kafka': ['kafka'],
  'n8n-nodes-base.kafkaTrigger': ['kafka'],
  // AI / LLM
  '@n8n/n8n-nodes-langchain.openAi': ['openAi'],
  '@n8n/n8n-nodes-langchain.anthropic': ['anthropic'],
  // Communication
  'n8n-nodes-base.slack': ['slackApi', 'oAuth2Api'],
  'n8n-nodes-base.discord': ['discord'],
  'n8n-nodes-base.sendEmail': ['smtp'],
  'n8n-nodes-base.sendGrid': ['sendGrid'],
  'n8n-nodes-base.twilio': ['twilio'],
  'n8n-nodes-base.microsoftTeams': ['microsoftTeams'],
  // Productivity
  'n8n-nodes-base.notion': ['notion'],
  'n8n-nodes-base.stripe': ['stripe'],
  'n8n-nodes-base.googleSheets': ['googleApi', 'oAuth2Api'],
  // Webhook / trigger nodes
  'n8n-nodes-base.webhook': ['httpBasicAuth', 'httpHeaderAuth', 'jwtAuth'],
  // Other
  'n8n-nodes-base.awsLambda': ['aws'],
  'n8n-nodes-base.s3': ['aws'],
  'n8n-nodes-base.gitlab': ['gitlab'],
  'n8n-nodes-base.github': ['github'],
  'n8n-nodes-base.elasticsearch': [],
}

/**
 * FALLBACK lookup of the credential-type names a node can be bound to.
 *
 * Prefer the node-type metadata (`credentials[]`) — see
 * `resolveNodeCredentialTypes` in `@/lib/credentialSchema`. This helper only
 * mirrors n8n's shipped nodes so the UI still works for node types whose
 * metadata declares no credential slots.
 */
export function getNodeCredentialTypes(nodeType: string): string[] {
  // 1. Static map (fallback — the backend metadata is authoritative).
  if (NODE_CREDENTIAL_TYPE_MAP[nodeType]) {
    return NODE_CREDENTIAL_TYPE_MAP[nodeType]
  }
  // 2. Allow short-name lookup (e.g. `httpRequest` -> `n8n-nodes-base.httpRequest`).
  const longForm = `n8n-nodes-base.${nodeType}`
  if (NODE_CREDENTIAL_TYPE_MAP[longForm]) {
    return NODE_CREDENTIAL_TYPE_MAP[longForm]
  }
  return []
}

/**
 * Returns the canonical long-form node-type name (e.g. `httpRequest` ->
 * `n8n-nodes-base.httpRequest`). Used when we need to write back to the
 * workflow JSON and the user typed a short name in some other tool.
 */
export function toLongNodeType(nodeType: string): string {
  if (nodeType.startsWith('n8n-nodes-base.') || nodeType.startsWith('@n8n/')) {
    return nodeType
  }
  return `n8n-nodes-base.${nodeType}`
}
