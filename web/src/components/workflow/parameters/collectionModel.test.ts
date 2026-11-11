import { describe, expect, it } from 'vitest'
import type { NodeProperty } from '@/types/node'
import {
  collectionFieldDefs,
  declaredFieldDefs,
  defaultRow,
  toRows,
} from './collectionModel'

describe('collectionModel typed metadata', () => {
  const property: NodeProperty = {
    displayName: 'Header Parameters',
    name: 'headerParameters',
    type: 'fixedCollection',
    options: [
      {
        name: 'parameters',
        value: 'parameters',
        values: [
          { name: 'name', value: 'name', displayName: 'Name', type: 'string', required: true, default: '' },
          { name: 'value', value: 'value', displayName: 'Value', type: 'string', default: '' },
        ],
      },
    ],
  }

  it('uses typed child fields before stored-row inference', () => {
    const defs = declaredFieldDefs(property)
    expect(defs.map((field) => field.name)).toEqual(['name', 'value'])
    expect(defs[0]).toMatchObject({ displayName: 'Name', type: 'string', required: true })
    expect(collectionFieldDefs(property, { inferred: true })).toEqual(defs)
  })

  it('creates defaults from child field metadata', () => {
    expect(defaultRow(declaredFieldDefs(property))).toEqual({ name: '', value: '' })
  })

  it('preserves fixedCollection bucket shape', () => {
    expect(toRows({ parameters: { name: 'X', value: 'Y' } })).toEqual([{ name: 'X', value: 'Y' }])
  })
})
