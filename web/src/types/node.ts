export interface NodeType {
  name: string
  displayName: string
  description: string
  version: number
  defaults: NodeDefaults
  inputs: string[]
  outputs: string[]
  properties?: NodeProperty[]
  credentials?: NodeCredentialType[]
  icon?: string
  iconColor?: string
  group?: string[]
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
  | 'dateTime'
  | 'color'

export interface NodePropertyOption {
  name: string
  value: string | number | boolean
  description?: string
  action?: string
}

export interface NodePropertyDisplayOptions {
  show?: Record<string, unknown[]>
  hide?: Record<string, unknown[]>
}

export interface NodePropertyTypeOptions {
  multipleValues?: boolean
  multipleValueButtonText?: string
  maxValue?: number
  minValue?: number
  numberPrecision?: number
  password?: boolean
  rows?: number
  alwaysOpenEditWindow?: boolean
}

export interface NodeCredentialType {
  name: string
  required?: boolean
  displayOptions?: NodePropertyDisplayOptions
}

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
  // Data / Database
  if (nodeType.includes('postgres') || nodeType.includes('mysql') || nodeType.includes('sqlite') || nodeType.includes('mongo') || nodeType.includes('redis') || nodeType.includes('elastic')) {
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
 * getNodeCredentialTypes returns the credential-type names a node can be
 * bound to. If the NodeType metadata exposed by the backend includes a
 * `credentials` array, use that. Otherwise, fall back to a static map
 * mirroring n8n's shipped nodes so the UI can render the bind UI without
 * waiting for the backend to start returning the metadata.
 *
 * The map is keyed by the long n8n node-type name (e.g.
 * `n8n-nodes-base.httpRequest`) but `getNodeCredentialTypes` also accepts
 * the short name (`httpRequest`) for convenience.
 */
const NODE_CREDENTIAL_TYPE_MAP: Record<string, string[]> = {
  // HTTP
  'n8n-nodes-base.httpRequest': ['httpBasicAuth', 'httpHeaderAuth', 'oAuth2Api'],
  // Databases
  'n8n-nodes-base.postgres': ['postgres'],
  'n8n-nodes-base.mySql': ['mySql'],
  'n8n-nodes-base.sqlite': [],
  'n8n-nodes-base.mongoDb': [],
  'n8n-nodes-base.redis': [],
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

export function getNodeCredentialTypes(nodeType: string): string[] {
  // 1. Static map (most reliable in m9m since metadata is sparse).
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
