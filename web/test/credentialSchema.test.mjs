// Tests for BUG-CAP-2026-09-09-003 — credential form helpers.

import { describe, it, expect } from 'vitest'
import {
  knownSchemas,
  validate,
  toPayload,
  fieldFor,
  isKnownType,
} from '../src/lib/credentialSchema'

describe('knownSchemas', () => {
  it('includes oAuth2Api', () => {
    expect(knownSchemas.oAuth2Api).toBeDefined()
    expect(knownSchemas.oAuth2Api.type).toBe('oAuth2Api')
  })
})

describe('validate', () => {
  it('returns no errors on a valid OAuth2 form', () => {
    const schema = knownSchemas.oAuth2Api
    const errors = validate(schema, {
      clientId: 'cid',
      clientSecret: 'csecret',
      authUrl: 'https://x/oauth/authorize',
      accessTokenUrl: 'https://x/oauth/token',
    })
    expect(errors).toEqual({})
  })

  it('reports missing required fields', () => {
    const errors = validate(knownSchemas.oAuth2Api, {})
    expect(errors.clientId).toBeTruthy()
    expect(errors.clientSecret).toBeTruthy()
    expect(errors.authUrl).toBeTruthy()
    expect(errors.accessTokenUrl).toBeTruthy()
  })

  it('treats empty-string as missing', () => {
    const errors = validate(knownSchemas.oAuth2Api, { clientId: '   ' })
    expect(errors.clientId).toBeTruthy()
  })

  it('treats non-string required fields as present', () => {
    const errors = validate(knownSchemas.oAuth2Api, { expiresIn: 3600 })
    expect(errors.expiresIn).toBeUndefined()
  })
})

describe('toPayload', () => {
  it('builds the wire shape', () => {
    const p = toPayload('My cred', 'oAuth2Api', {
      clientId: 'cid',
      clientSecret: 'cs',
      authUrl: 'https://x/auth',
      accessTokenUrl: 'https://x/token',
    })
    expect(p.name).toBe('My cred')
    expect(p.type).toBe('oAuth2Api')
    expect(p.data.clientId).toBe('cid')
  })

  it('strips fields not in the schema', () => {
    const p = toPayload('My cred', 'oAuth2Api', {
      clientId: 'cid',
      authUrl: 'https://x/auth',
      accessTokenUrl: 'https://x/token',
      rogueField: 'should be dropped',
    })
    expect(p.data.rogueField).toBeUndefined()
  })

  it('drops empty fields', () => {
    const p = toPayload('c', 'oAuth2Api', {
      clientId: 'cid',
      authUrl: 'https://x/auth',
      accessTokenUrl: 'https://x/token',
      scope: '',
    })
    expect('scope' in p.data).toBe(false)
  })

  it('passes unknown types through (JSON fallback path)', () => {
    const p = toPayload('c', 'mongoDb', { connectionString: 'mongodb://x', db: 'admin' })
    expect(p.type).toBe('mongoDb')
    expect(p.data.connectionString).toBe('mongodb://x')
  })
})

describe('fieldFor', () => {
  it('returns the field for a known name', () => {
    const f = fieldFor(knownSchemas.oAuth2Api, 'clientId')
    expect(f).toBeDefined()
    expect(f.required).toBe(true)
  })
  it('returns undefined for an unknown name', () => {
    expect(fieldFor(knownSchemas.oAuth2Api, 'rogue')).toBeUndefined()
  })
})

describe('isKnownType', () => {
  it('returns true for known types', () => {
    expect(isKnownType('oAuth2Api')).toBe(true)
  })
  it('returns false for unknown types', () => {
    expect(isKnownType('mongoDb')).toBe(false)
  })
})
