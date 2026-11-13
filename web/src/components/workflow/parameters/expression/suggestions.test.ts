import { describe, expect, it } from 'vitest'
import { collectJsonFieldTree, collectJsonFields } from '@/lib/expressionFields'
import { FUNCTIONS } from './functions'
import {
  applySuggestionAt,
  detectExpressionPosition,
  filterSuggestions,
  matchesLabel,
  suggestForPosition,
  MAX_SUGGESTIONS,
  type SuggestionSources,
} from './suggestions'

const sources: SuggestionSources = {
  variables: [
    { name: '$json', description: 'JSON payload of the current item', example: '$json.id' },
    { name: '$now', description: 'Current timestamp', example: "$now.format('yyyy')" },
    { name: '$node', description: 'Output of another node', example: '$node["Set"].json' },
  ],
  nodes: [
    { name: 'Webhook', type: 'n8n-nodes-base.webhook' },
    { name: 'Set', type: 'n8n-nodes-base.set', fields: ['id', 'name'] },
  ],
  functions: FUNCTIONS,
  itemFields: ['id', 'name', 'email'],
  nodeFields: { Set: ['id', 'name'] },
}

/** The payload of the current item — it has nested objects and an array. */
const itemPayload = {
  id: 7,
  name: 'Bob',
  email: 'bob@example.com',
  address: {
    city: 'Jakarta',
    zip: '12345',
    geo: { lat: -6.2, lng: 106.8 },
  },
  items: [
    { sku: 'A1', qty: 2 },
    { sku: 'B2', price: 9 },
  ],
}

/** The payload the node "Set" produced. */
const nodePayload = {
  id: 7,
  address: { city: 'Surabaya', zip: '54321' },
}

/** The sources with the nested field trees of the payloads. */
const nestedSources: SuggestionSources = {
  variables: sources.variables,
  nodes: sources.nodes,
  functions: sources.functions,
  itemFields: collectJsonFields(itemPayload),
  nodeFields: { Set: collectJsonFields(nodePayload) },
  itemTree: collectJsonFieldTree(itemPayload),
  nodeTrees: { Set: collectJsonFieldTree(nodePayload) },
}

/** The suggestions for a caret position inside a `{{ … }}` expression. */
function suggestionsAt(
  text: string,
  caret = text.length,
  list: SuggestionSources = sources,
) {
  const position = detectExpressionPosition(text, caret)
  return suggestForPosition(position, list)
}

describe('expression position', () => {
  it('is outside of an expression when there is no opener', () => {
    expect(detectExpressionPosition('plain text', 5).inExpression).toBe(false)
  })

  it('is outside of an expression when the expression was already closed', () => {
    expect(detectExpressionPosition('{{$json.id}} and more', 17).inExpression).toBe(false)
  })

  it('is inside of an expression right after the opener', () => {
    const position = detectExpressionPosition('{{', 2)

    expect(position.inExpression).toBe(true)
    expect(position.prefix).toBe('')
    expect(position.root).toBeNull()
  })

  it('reads the token typed so far as the filter', () => {
    const position = detectExpressionPosition('{{$js', 5)

    expect(position.prefix).toBe('$js')
    expect(position.replaceLength).toBe(3)
  })

  it('knows the item path behind a dot', () => {
    const position = detectExpressionPosition('{{$json.', 8)

    expect(position.afterDot).toBe(true)
    expect(position.root).toBe('$json')
    expect(position.segments).toEqual([])
  })

  it('knows the node of a node reference', () => {
    const position = detectExpressionPosition('{{$node["Set"].', 15)

    expect(position.root).toBe('$node')
    expect(position.nodeName).toBe('Set')
    expect(position.afterDot).toBe(true)
  })

  it('knows the name of a node that is still being typed', () => {
    const position = detectExpressionPosition('{{$node["We', 11)

    expect(position.nodeNamePrefix).toBe('We')
  })
})

describe('suggestion list', () => {
  it('opens right after the opener and offers the variables', () => {
    const labels = suggestionsAt('{{').map((suggestion) => suggestion.label)

    expect(labels).toContain('$json')
    expect(labels).toContain('$now')
    expect(labels).toContain('$node["Webhook"]')
  })

  it('filters the list while the user types', () => {
    const labels = suggestionsAt('{{$js').map((suggestion) => suggestion.label)

    expect(labels).toContain('$json')
    expect(labels).not.toContain('$now')
  })

  it('offers the fields of the item behind `$json.`', () => {
    const suggestions = suggestionsAt('{{$json.')

    expect(suggestions.map((suggestion) => suggestion.label)).toEqual(['id', 'name', 'email'])
    expect(suggestions[0].kind).toBe('field')
  })

  it('offers the fields of a node behind `$node["Set"].`', () => {
    const labels = suggestionsAt('{{$node["Set"].').map((suggestion) => suggestion.label)

    expect(labels).toEqual(['id', 'name'])
  })

  it('offers the nodes whose name matches', () => {
    const labels = suggestionsAt('{{$node["We').map((suggestion) => suggestion.label)

    expect(labels).toEqual(['$node["Webhook"]'])
  })

  it('offers the functions for a plain word', () => {
    const labels = suggestionsAt('{{spl').map((suggestion) => suggestion.label)

    expect(labels).toContain('split')
  })

  it('has no suggestions outside of an expression', () => {
    expect(suggestionsAt('plain')).toEqual([])
  })
})

describe('suggestion filtering', () => {
  it('matches a prefix', () => {
    expect(matchesLabel('$json', '$js')).toBe(true)
    expect(matchesLabel('$json', '$zz')).toBe(false)
  })

  it('matches a label that starts with a dollar sign loosely', () => {
    expect(matchesLabel('$node["Set"]', '$nod')).toBe(true)
  })

  it('keeps the list short', () => {
    const many = Array.from({ length: 100 }, (_, index) => ({
      kind: 'variable' as const,
      label: `$v${index}`,
      detail: '',
      insertText: `$v${index}`,
    }))

    expect(filterSuggestions(many, '')).toHaveLength(25)
    expect(filterSuggestions(many, '$v9').length).toBeLessThanOrEqual(25)
  })
})

describe('nested field completion', () => {
  it('offers the fields of a nested object behind `$json.address.`', () => {
    const labels = suggestionsAt('{{$json.address.', undefined, nestedSources).map(
      (suggestion) => suggestion.label,
    )

    expect(labels).toEqual(['city', 'zip', 'geo'])
  })

  it('offers the fields of a deeply nested object', () => {
    const labels = suggestionsAt('{{$json.address.geo.', undefined, nestedSources).map(
      (suggestion) => suggestion.label,
    )

    expect(labels).toEqual(['lat', 'lng'])
  })

  it('offers the fields of the objects inside an array', () => {
    const labels = suggestionsAt('{{$json.items.', undefined, nestedSources).map(
      (suggestion) => suggestion.label,
    )

    expect(labels).toEqual(['sku', 'qty', 'price'])
  })

  it('offers nothing for a path that does not resolve', () => {
    expect(suggestionsAt('{{$json.missing.', undefined, nestedSources)).toEqual([])
    expect(suggestionsAt('{{$json.address.missing.', undefined, nestedSources)).toEqual([])
    expect(suggestionsAt('{{$json.address.city.zip.', undefined, nestedSources)).toEqual([])
  })

  it('offers the nested fields of a node behind `$node["Set"].json.address.`', () => {
    const labels = suggestionsAt(
      '{{$node["Set"].json.address.',
      undefined,
      nestedSources,
    ).map((suggestion) => suggestion.label)

    expect(labels).toEqual(['city', 'zip'])
  })

  it('offers the nested fields of a node without the `json` segment', () => {
    const labels = suggestionsAt('{{$node["Set"].address.', undefined, nestedSources).map(
      (suggestion) => suggestion.label,
    )

    expect(labels).toEqual(['city', 'zip'])
  })

  it('offers the fields of the node payload behind `$node["Set"].json.`', () => {
    const labels = suggestionsAt('{{$node["Set"].json.', undefined, nestedSources).map(
      (suggestion) => suggestion.label,
    )

    expect(labels).toEqual(['id', 'address'])
  })

  it('offers nothing for a node path that does not resolve', () => {
    expect(suggestionsAt('{{$node["Set"].json.missing.', undefined, nestedSources)).toEqual([])
    expect(suggestionsAt('{{$node["Set"].json.address.city.', undefined, nestedSources)).toEqual([])
  })

  it('filters the fields of a nested object by the typed name', () => {
    const labels = suggestionsAt('{{$json.address.ci', undefined, nestedSources).map(
      (suggestion) => suggestion.label,
    )

    expect(labels).toEqual(['city'])
  })

  it('does not offer a field for a fuzzy name', () => {
    expect(suggestionsAt('{{$json.cty', undefined, nestedSources)).toEqual([])
  })

  it('knows the path segment that is being typed', () => {
    expect(detectExpressionPosition('{{$json.address.ci', 18).partialSegment).toBe('ci')
    expect(detectExpressionPosition('{{$json', 7).partialSegment).toBeNull()
  })

  it('keeps the field list short', () => {
    const wide = collectJsonFieldTree(
      Object.fromEntries(Array.from({ length: 40 }, (_, index) => [`f${index}`, index])),
    )
    const list = suggestionsAt('{{$json.', undefined, {
      ...nestedSources,
      itemTree: wide,
    })

    expect(list).toHaveLength(MAX_SUGGESTIONS)
  })
})

describe('fuzzy matching', () => {
  it('matches a scattered subsequence', () => {
    expect(matchesLabel('$json', 'jsn')).toBe(true)
    expect(matchesLabel('$workflow', 'wnf')).toBe(true)
    expect(matchesLabel('$node["Set"]', 'jn["st')).toBe(true)
  })

  it('does not match a needle that shares nothing with the label', () => {
    expect(matchesLabel('$json', 'zz')).toBe(false)
    expect(matchesLabel('$node["Set"]', 'We')).toBe(false)
  })

  it('offers a fuzzy match in the root list', () => {
    const labels = suggestionsAt('{{jsn').map((suggestion) => suggestion.label)

    expect(labels).toContain('$json')
    expect(labels).not.toContain('$now')
  })

  it('offers a fuzzy match in the node name list', () => {
    const labels = suggestionsAt('{{$node["webhok').map((suggestion) => suggestion.label)

    expect(labels).toEqual(['$node["Webhook"]'])
  })

  it('ranks a prefix match above a scattered match', () => {
    const list = [
      { kind: 'variable' as const, label: '$a_js_on', detail: '', insertText: '$a_js_on' },
      { kind: 'variable' as const, label: '$json', detail: '', insertText: '$json' },
    ]

    expect(filterSuggestions(list, '$js').map((suggestion) => suggestion.label)).toEqual([
      '$json',
      '$a_js_on',
    ])
  })

  it('sorts the list alphabetically inside a group', () => {
    const list = [
      { kind: 'variable' as const, label: '$beta', detail: '', insertText: '$beta' },
      { kind: 'variable' as const, label: '$alpha', detail: '', insertText: '$alpha' },
    ]

    expect(filterSuggestions(list, '').map((suggestion) => suggestion.label)).toEqual([
      '$alpha',
      '$beta',
    ])
  })

  it('keeps the order of the list for equal labels', () => {
    const list = [
      { kind: 'variable' as const, label: '$same', detail: 'first', insertText: '$same' },
      { kind: 'variable' as const, label: '$same', detail: 'second', insertText: '$same' },
    ]

    expect(filterSuggestions(list, '').map((suggestion) => suggestion.detail)).toEqual([
      'first',
      'second',
    ])
  })
})

describe('inserting a suggestion', () => {
  it('replaces the token in front of the caret', () => {
    const text = '{{$js'
    const position = detectExpressionPosition(text, text.length)
    const result = applySuggestionAt(text, text.length, position, '$json')

    expect(result.text).toBe('{{$json')
    expect(result.caret).toBe(7)
  })

  it('completes an expression that was only opened', () => {
    const text = '{{'
    const position = detectExpressionPosition(text, text.length)
    const result = applySuggestionAt(text, text.length, position, '$json.id')

    expect(result.text).toBe('{{$json.id')
    expect(result.caret).toBe(10)
  })

  it('completes a path behind a dot', () => {
    const text = '{{$json.'
    const position = detectExpressionPosition(text, text.length)
    const result = applySuggestionAt(text, text.length, position, 'name')

    expect(result.text).toBe('{{$json.name')
    expect(result.caret).toBe(12)
  })

  it('keeps the text behind the caret', () => {
    const text = '{{$js}}'
    const position = detectExpressionPosition(text, 5)
    const result = applySuggestionAt(text, 5, position, '$json')

    expect(result.text).toBe('{{$json}}')
  })
})
