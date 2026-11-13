/**
 * Test suite of the error banner rules of the expression editor.
 *
 * The banner is the red hint under the field. It must stay away while
 * the user is still typing — an expression that is not closed yet is
 * unfinished, not broken — but it must show every real syntax error
 * of a closed expression.
 */

import { describe, expect, it } from 'vitest'
import { evaluateValue } from './evaluator'
import { isExpressionOpenAtCaret, selectDisplayError } from './errors'
import { detectExpressionPosition, suggestForPosition, type SuggestionSources } from './suggestions'
import type { ExpressionError } from './types'

/** The sources of the suggestions — an item with a single `body` field. */
const sources: SuggestionSources = {
  variables: [],
  nodes: [],
  functions: [],
  itemFields: ['body'],
  nodeFields: {},
  itemTree: { fields: ['body'], children: {} },
}

/** The error of a parameter value, failing when the value parses. */
function errorOf(source: string): ExpressionError {
  const result = evaluateValue(source)
  if (result.ok) throw new Error(`evaluation of "${source}" unexpectedly succeeded`)
  return result.error
}

/** The error the editor shows for a value, with the caret at its end. */
function displayErrorOf(
  source: string,
  options: { suggestionsOpen?: boolean; hasSampleInput?: boolean } = {},
): ExpressionError | null {
  return selectDisplayError(errorOf(source), {
    text: source,
    caret: source.length,
    suggestionsOpen: options.suggestionsOpen ?? false,
    hasSampleInput: options.hasSampleInput ?? false,
  })
}

describe('open expressions', () => {
  it('detects an expression that is still open', () => {
    expect(isExpressionOpenAtCaret('{{$json.bdy', 10)).toBe(true)
    expect(isExpressionOpenAtCaret('{{$json.nope.', 12)).toBe(true)
  })

  it('detects an expression that was closed', () => {
    expect(isExpressionOpenAtCaret('{{$json.id}}', 12)).toBe(false)
  })

  it('detects no expression at all', () => {
    expect(isExpressionOpenAtCaret('plain text', 10)).toBe(false)
  })

  it('reports an open expression as an unterminated error', () => {
    const error = errorOf('{{$json.bdy')

    expect(error.kind).toBe('unterminated')
    expect(error.message).toBe("Unterminated expression: missing '}}'")
  })
})

describe('the error banner', () => {
  it('suppresses the unterminated error while the expression is still open', () => {
    expect(displayErrorOf('{{$json.bdy')).toBeNull()
    expect(displayErrorOf('{{$json.nope.')).toBeNull()
  })

  it('suppresses the unterminated error when the suggestion list is empty', () => {
    const text = '{{$json.nope.'
    const position = detectExpressionPosition(text, text.length)
    const list = suggestForPosition(position, sources)

    expect(list).toEqual([])
    expect(displayErrorOf(text)).toBeNull()
  })

  it('shows the error of a closed but broken expression', () => {
    const error = displayErrorOf('{{$json..}}')

    expect(error?.kind).toBe('syntax')
    expect(error?.message).toBe("Expected a name but found '.'")
  })

  it('shows the error of a closed expression that ends with a dot', () => {
    const error = displayErrorOf('{{$json.body. }}')

    expect(error?.kind).toBe('syntax')
    expect(error?.message).toBe('Expected a name but found end of expression')
  })

  it('shows the unterminated error when the caret is outside of the expression', () => {
    const text = '{{$json.bdy'
    const error = selectDisplayError(errorOf(text), {
      text,
      caret: 0,
      suggestionsOpen: false,
      hasSampleInput: false,
    })

    expect(error?.kind).toBe('unterminated')
  })

  it('shows the unterminated error of an expression behind the caret', () => {
    const text = 'hello {{$json.bdy'
    const error = selectDisplayError(errorOf(text), {
      text,
      caret: 0,
      suggestionsOpen: false,
      hasSampleInput: false,
    })

    // The `{{` stands behind the caret, so the expression under the
    // caret is not open — the error is a real one.
    expect(error?.kind).toBe('unterminated')
  })

  it('hides the error while the suggestion list is open', () => {
    expect(displayErrorOf('{{$json..}}', { suggestionsOpen: true })).toBeNull()
  })

  it('hides a runtime error when the editor has no data to evaluate against', () => {
    expect(displayErrorOf('{{$json.body}}')).toBeNull()
    expect(displayErrorOf('{{$json.body}}', { hasSampleInput: true })?.kind).toBe('runtime')
  })
})
