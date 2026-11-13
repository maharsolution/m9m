import { describe, expect, it } from 'vitest'
import { buildExpressionFieldSources, collectJsonFields } from './expressionFields'

describe('expression fields — the completion data of the editor', () => {
  it('collects the field names of a JSON payload', () => {
    expect(collectJsonFields({ id: 7, name: 'Bob' })).toEqual(['id', 'name'])
  })

  it('has no fields for a value that is not an object', () => {
    expect(collectJsonFields(undefined)).toEqual([])
    expect(collectJsonFields(null)).toEqual([])
    expect(collectJsonFields('text')).toEqual([])
    expect(collectJsonFields([1, 2])).toEqual([])
  })

  it('builds the sources of the current item and of every node', () => {
    const sources = buildExpressionFieldSources(
      { id: 7, name: 'Bob' },
      { Set: { ok: true }, Webhook: { headers: {} } },
    )

    expect(sources.itemFields).toEqual(['id', 'name'])
    expect(sources.nodeFields.Set).toEqual(['ok'])
    expect(sources.nodeFields.Webhook).toEqual(['headers'])
  })
})
