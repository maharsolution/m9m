// Credential form helpers for BUG-CAP-2026-09-09-003.
//
// The credentials view in m9m v0.3.x was read-only (the
// "New" form was a JSON textarea). This module provides
// the typed form helpers that drive a per-type "New"
// form: validate the input, build the wire payload, and
// map server-side validation errors back to fields.
//
// `OAuth2Schema` is the canonical n8n-shape schema
// (matches `internal/credentials/oauth2.go`). Other
// types (httpBasicAuth, httpHeaderAuth, kafka, mongoDb,
// redis, postgres, etc.) follow the same shape — the
// m9m credential envelope is `{name, type, data: {…}}`
// where `data` keys match the field name in the schema.

import { OAuth2Schema, type CredentialSchema, type CredentialSchemaField } from '@/types/credentials'
import { getNodeCredentialTypes } from '@/types/node'
import type { NodeType } from '@/types/node'

/**
 * Fallback schema for the `oAuth2Api` credential.
 *
 * `OAuth2Schema` (internal/credentials/oauth2.go) is the base shape;
 * the `authentication` field (header vs. body token transport) is
 * added here because the API registry
 * (internal/api/credentials.go → `oAuth2Api`) declares it and the form
 * must be able to persist it.
 */
const OAUTH2_API_SCHEMA: CredentialSchema = {
  ...OAuth2Schema,
  type: 'oAuth2Api',
  properties: [
    ...OAuth2Schema.properties,
    {
      name: 'authentication',
      display: 'Authentication',
      type: 'options',
      options: ['header', 'body'],
      required: false,
      default: 'header',
    },
  ],
}

// knownSchemas is the registry the "New" form uses. Add
// a type here to enable a structured form for it.
//
// The keys are the credential-type ids of the backend registry
// (internal/api/credentials.go) — a key that does not exist there can
// never be created, so the form would end in a 404.
export const knownSchemas: Record<string, CredentialSchema> = {
  // The credential-type id used by the API registry is `oAuth2Api`
  // (internal/api/credentials.go). The key MUST match the id stored in
  // the `type` field of a credential — the old `oauth2Api` spelling
  // (internal/credentials/oauth2.go) is not a type the credential API
  // accepts, so a form built from it would 404 on the schema lookup.
  oAuth2Api: OAUTH2_API_SCHEMA,
  // Future: httpBasicAuth, httpHeaderAuth, apiKey, kafka, …
}

// validate runs the schema's required-field gate. Returns
// a `{field: message}` map (empty if everything is OK).
export function validate(schema: CredentialSchema, data: Record<string, unknown>): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const field of schema.properties) {
    if (field.required) {
      const v = data[field.name]
      if (v === undefined || v === null || (typeof v === 'string' && v.trim() === '')) {
        errors[field.name] = `${field.display} is required`
      }
    }
  }
  return errors
}

// toPayload is the wire shape `POST /api/v1/credentials`
// expects. The engine re-encrypts `data` and persists it
// under the credential id.
export function toPayload(name: string, type: string, data: Record<string, unknown>): {
  name: string
  type: string
  data: Record<string, unknown>
} {
  // Drop any field that's not in the schema. The schema
  // is the source of truth — extra fields in the form
  // (e.g. the user typed into a debug textarea) are
  // ignored, not persisted.
  const schema = knownSchemas[type]
  const cleaned: Record<string, unknown> = {}
  if (schema) {
    for (const field of schema.properties) {
      const v = data[field.name]
      if (v !== undefined && v !== null && v !== '') {
        cleaned[field.name] = v
      }
    }
  } else {
    Object.assign(cleaned, data)
  }
  return { name, type, data: cleaned }
}

// fieldFor returns the schema field for a given property
// name, or undefined if the schema doesn't have that
// field. Used by the form's per-field error rendering.
export function fieldFor(schema: CredentialSchema, name: string): CredentialSchemaField | undefined {
  return schema.properties.find((f) => f.name === name)
}

// isKnownType reports whether the type has a structured
// schema. Unknown types still work via the JSON fallback
// (the "Advanced" textarea), so this is purely a UI hint.
export function isKnownType(type: string): boolean {
  return type in knownSchemas
}

// ---------------------------------------------------------------------------
// Node credential slots (metadata-driven)
// ---------------------------------------------------------------------------

/**
 * A credential slot a node type can be bound to.
 *
 * `fromFallback` marks slots that came from the static fallback table
 * rather than the node-type metadata — the UI shows them as such so a
 * missing metadata entry is visible instead of silent.
 */
export interface ResolvedCredentialSlot {
  /** Credential type id (the key used in `node.credentials`). */
  type: string
  /** Human-readable label for the picker. */
  displayName: string
  /** The node refuses to run without this credential. */
  required: boolean
  /** True when the slot came from the static fallback table. */
  fromFallback: boolean
}

/**
 * The credential-type ids the backend can create.
 *
 * Mirrors the keys of `credentialSchemas` in
 * `internal/api/credentials.go` — 13 base types plus the `mySql` and
 * `openAiApi` aliases and the `rabbitmq` / `redis` schemas that
 * `init()` registers.
 *
 * This registry is authoritative for the UI: a type that is missing
 * here makes `GET /api/v1/credentials/schema/{type}` answer 404, so
 * the picker must never offer it.
 */
export const BACKEND_CREDENTIAL_TYPES = [
  'httpBasicAuth',
  'httpHeaderAuth',
  'oAuth2Api',
  'apiKey',
  'postgres',
  'mysql',
  'mySql',
  'mongoDb',
  'kafka',
  'slack',
  'discord',
  'openai',
  'openAiApi',
  'anthropic',
  'jwtAuth',
  'rabbitmq',
  'redis',
] as const

/** One of the credential-type ids the backend registry knows. */
export type BackendCredentialType = (typeof BACKEND_CREDENTIAL_TYPES)[number]

const BACKEND_CREDENTIAL_TYPE_SET: ReadonlySet<string> = new Set(
  BACKEND_CREDENTIAL_TYPES,
)

/**
 * True when the backend registry can create a credential of this type.
 *
 * Used to keep the static fallback table from offering a type the API
 * would reject.
 */
export function isBackendCredentialType(type: string): boolean {
  return BACKEND_CREDENTIAL_TYPE_SET.has(type)
}

/**
 * FALLBACK label table.
 *
 * Only used when the node-type metadata does not provide a
 * `displayName` for a credential slot (or when the slot comes from the
 * static fallback list of node types).
 *
 * Every key is a credential type the backend registry defines and every
 * value is that schema's `DisplayName` — the map is typed against
 * `BackendCredentialType` so a drifting id fails the type check.
 */
const CREDENTIAL_TYPE_LABELS: Record<BackendCredentialType, string> = {
  httpBasicAuth: 'Basic Auth',
  httpHeaderAuth: 'Header Auth',
  oAuth2Api: 'OAuth2 API',
  apiKey: 'API Key',
  postgres: 'Postgres',
  mysql: 'MySQL',
  mySql: 'MySQL',
  mongoDb: 'MongoDB',
  kafka: 'Kafka',
  slack: 'Slack',
  discord: 'Discord',
  openai: 'OpenAI',
  openAiApi: 'OpenAI',
  anthropic: 'Anthropic',
  jwtAuth: 'JWT Auth',
  rabbitmq: 'RabbitMQ',
  redis: 'Redis',
}

/** Display label for a credential type id. */
export function credentialTypeLabel(type: string): string {
  return isBackendCredentialType(type)
    ? CREDENTIAL_TYPE_LABELS[type as BackendCredentialType]
    : type
}

/**
 * Resolve the credential slots of a node type.
 *
 * The node-type metadata (`NodeType.credentials`) is authoritative: it
 * carries the credential type, its display name and whether it is
 * required. Only node types whose metadata declares no credentials fall
 * back to the static table in `@/types/node`.
 */
export function resolveNodeCredentialTypes(
  nodeType?: NodeType | null,
): ResolvedCredentialSlot[] {
  const declared = nodeType?.credentials ?? []
  if (declared.length > 0) {
    return declared
      .map((credential) => {
        // `type` is the canonical key; `name` is the deprecated alias.
        const type = credential.type ?? credential.name ?? ''
        return {
          type,
          displayName: credential.displayName || credentialTypeLabel(type),
          required: credential.required === true,
          fromFallback: false,
        }
      })
      .filter((credential) => credential.type.length > 0)
  }

  // FALLBACK — node types whose metadata declares no credential slots.
  //
  // The static table mirrors n8n's shipped nodes, so it also lists
  // types m9m's credential registry does not know (`smtp`, `twilio`,
  // `slackApi`, `openAi`, …). Offering one of those would let the
  // user pick a credential the API rejects with 404 on save, so the
  // list is filtered against the backend registry here.
  const fallbackTypes = getNodeCredentialTypes(nodeType?.name ?? '').filter(
    (type) => isBackendCredentialType(type),
  )
  return fallbackTypes.map((type) => ({
    type,
    displayName: credentialTypeLabel(type),
    required: false,
    fromFallback: true,
  }))
}
