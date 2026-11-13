import { describe, expect, it } from 'vitest'
import {
  buildExpressionFieldSources,
  collectJsonFieldTree,
  collectJsonFields,
  MAX_FIELD_DEPTH,
} from './expressionFields'

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

  it('builds the nested field tree of a payload', () => {
    const tree = collectJsonFieldTree({
      id: 7,
      address: { city: 'Jakarta', geo: { lat: -6.2 } },
    })

    expect(tree.fields).toEqual(['id', 'address'])
    expect(tree.children.address.fields).toEqual(['city', 'geo'])
    expect(tree.children.address.children.geo.fields).toEqual(['lat'])
  })

  it('resolves an array of objects through its elements', () => {
    const tree = collectJsonFieldTree({
      items: [
        { sku: 'A1', qty: 2 },
        { sku: 'B2', price: 9 },
      ],
    })

    expect(tree.children.items.fields).toEqual(['sku', 'qty', 'price'])
  })

  it('stops the tree at the maximum depth', () => {
    let payload: Record<string, unknown> = { leaf: true }
    for (let level = 0; level < MAX_FIELD_DEPTH + 2; level += 1) {
      payload = { next: payload }
    }

    const tree = collectJsonFieldTree(payload)
    let current = tree
    let depth = 0
    while (current.children.next) {
      current = current.children.next
      depth += 1
    }

    expect(depth).toBeLessThanOrEqual(MAX_FIELD_DEPTH)
    expect(current.fields).toEqual([])
  })

  it('builds the trees of the current item and of every node', () => {
    const sources = buildExpressionFieldSources(
      { id: 7, address: { city: 'Jakarta' } },
      { Set: { ok: true, meta: { count: 1 } } },
    )

    expect(sources.itemTree.fields).toEqual(['id', 'address'])
    expect(sources.itemTree.children.address.fields).toEqual(['city'])
    expect(sources.nodeTrees.Set.fields).toEqual(['ok', 'meta'])
    expect(sources.nodeTrees.Set.children.meta.fields).toEqual(['count'])
  })
})
