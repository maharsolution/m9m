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

// knownSchemas is the registry the "New" form uses. Add
// a type here to enable a structured form for it.
export const knownSchemas: Record<string, CredentialSchema> = {
  oauth2Api: OAuth2Schema,
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
