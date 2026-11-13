import { describe, expect, it } from 'vitest'
import { FUNCTIONS } from './functions'
import {
  applySuggestionAt,
  detectExpressionPosition,
  filterSuggestions,
  matchesLabel,
  suggestForPosition,
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

/** The suggestions for a caret position inside a `{{ … }}` expression. */
function suggestionsAt(text: string, caret = text.length) {
  const position = detectExpressionPosition(text, caret)
  return suggestForPosition(position, sources)
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
