/**
 * Test suite for the browser-side expression engine.
 *
 * The file used to be a plain script that bundled itself with
 * `esbuild`; Vitest reported it with "No test suite found". It is a
 * proper Vitest suite now, covering the parser, the evaluator and the
 * function catalogue of the expression editor.
 */

import { describe, expect, it } from 'vitest'
import { evaluateExpression, evaluateValue, type ExpressionContext } from './evaluator'
import { parseExpression, parseValue } from './parser'
import { ExpressionSyntaxError, type ExpressionError } from './types'

/** Evaluate an expression and return its value, failing on errors. */
function evaluate(source: string, context: ExpressionContext = {}): unknown {
  const result = evaluateExpression(source, context)
  if (!result.ok) {
    throw new Error(
      `evaluation of "${source}" failed: ${result.error.message} ` +
        `(${result.error.start}-${result.error.end})`,
    )
  }
  return result.value
}

/** Evaluate a parameter value and return its value, failing on errors. */
function evaluateParameter(source: string, context: ExpressionContext = {}): unknown {
  const result = evaluateValue(source, context)
  if (!result.ok) {
    throw new Error(
      `evaluation of "${source}" failed: ${result.error.message} ` +
        `(${result.error.start}-${result.error.end})`,
    )
  }
  return result.value
}

/** Evaluate an expression and return its error, failing when it succeeds. */
function evaluateError(
  source: string,
  context: ExpressionContext = {},
): ExpressionError {
  const result = evaluateExpression(source, context)
  if (result.ok) throw new Error(`evaluation of "${source}" unexpectedly succeeded`)
  return result.error
}

describe('expression parser', () => {
  it('parses a member expression', () => {
    const node = parseExpression('$json.user.id')
    expect(node.type).toBe('member')
    expect(node.start).toBe(0)
    expect(node.end).toBe(13)
  })

  it('parses literals and binary expressions', () => {
    expect(parseExpression('42').type).toBe('literal')
    expect(parseExpression("'text'").type).toBe('literal')
    expect(parseExpression('1 + 2').type).toBe('binary')
    expect(parseExpression('$json.a ? 1 : 2').type).toBe('conditional')
    expect(parseExpression('!$json.flag').type).toBe('unary')
    expect(parseExpression('split($json.a, ",")').type).toBe('call')
  })

  it('rejects invalid expressions', () => {
    const invalid = ['', '$json.', '1 +', '(', '$json.a ===']
    for (const source of invalid) {
      expect(() => parseExpression(source)).toThrow(ExpressionSyntaxError)
    }
  })

  it('reports the offset of a syntax error', () => {
    try {
      parseExpression('$json.')
      throw new Error('expected a syntax error')
    } catch (error) {
      expect(error).toBeInstanceOf(ExpressionSyntaxError)
      const syntaxError = error as ExpressionSyntaxError
      expect(syntaxError.start).toBe(6)
      expect(syntaxError.end).toBe(6)
    }
  })

  it('splits a value into text and expression chunks', () => {
    const chunks = parseValue('Hello {{ $json.name }}!')
    expect(chunks).toHaveLength(3)
    expect(chunks[0].type).toBe('text')
    expect(chunks[1].type).toBe('expression')
    expect(chunks[2].type).toBe('text')

    const expression = chunks[1]
    if (expression.type !== 'expression') throw new Error('expected an expression chunk')
    // The span of the parsed node is absolute inside the value.
    expect(expression.node.start).toBe(9)
    expect(expression.node.end).toBe(19)
  })
})

describe('expression evaluator', () => {
  it('evaluates ternaries and comparisons', () => {
    const context: ExpressionContext = { json: { age: 20, name: 'Bob' } }
    expect(evaluate('$json.age >= 18 ? "adult" : "minor"', context)).toBe('adult')
    expect(evaluate('$json.age > 30 ? "adult" : "minor"', context)).toBe('minor')
    expect(evaluate('$json.age == 20', context)).toBe(true)
    expect(evaluate('$json.age === "20"', context)).toBe(false)
    expect(evaluate('$json.name !== "Alice"', context)).toBe(true)
    expect(evaluate('1 + 2 * 3', context)).toBe(7)
  })

  it('reads $json', () => {
    const context: ExpressionContext = {
      json: { user: { id: 7, name: 'Bob' }, tags: ['a', 'b'] },
    }
    expect(evaluate('$json.user.id', context)).toBe(7)
    expect(evaluate('$json.user.name', context)).toBe('Bob')
    expect(evaluate('$json.tags[1]', context)).toBe('b')
    expect(evaluate('$json.missing', context)).toBeUndefined()
  })

  it('reads $input', () => {
    const context: ExpressionContext = {
      json: { id: 2 },
      input: [{ id: 1 }, { id: 2 }, { id: 3 }],
      itemIndex: 1,
    }
    expect(evaluate('$input.all()', context)).toEqual([{ id: 1 }, { id: 2 }, { id: 3 }])
    expect(evaluate('$input.first()', context)).toEqual({
      json: { id: 1 },
      binary: null,
      pairedItem: null,
    })
    expect(evaluate('$input.last()', context)).toEqual({
      json: { id: 3 },
      binary: null,
      pairedItem: null,
    })
    expect(evaluate('$input.item', context)).toEqual({
      json: { id: 2 },
      binary: null,
      pairedItem: null,
    })
    expect(evaluate('$input.item.json.id', context)).toBe(2)
    expect(evaluate('$input.first().json.id', context)).toBe(1)
  })

  it('reads $node and the $() shorthand', () => {
    const context: ExpressionContext = {
      node: { Set: { value: 5 }, Other: { value: 6 } },
    }
    expect(evaluate('$node["Set"].json', context)).toEqual({ value: 5 })
    expect(evaluate('$("Set").json', context)).toEqual({ value: 5 })
    expect(evaluate('$node["Set"].json.value', context)).toBe(5)
    expect(evaluate('$("Set").item.json.value', context)).toBe(5)
    expect(evaluate('$("Other").first().json.value', context)).toBe(6)
  })

  it('reads $now, $runIndex, $workflow and $execution', () => {
    const now = new Date(2026, 0, 2, 3, 4, 5, 678)
    const context: ExpressionContext = {
      now,
      runIndex: 3,
      workflow: { id: 'wf-1', name: 'My workflow', active: true },
      execution: { id: 'exec-1', mode: 'manual' },
    }
    expect(evaluate('$now.toMillis()', context)).toBe(now.getTime())
    expect(evaluate('$now.format("yyyy-MM-dd HH:mm:ss")', context)).toBe('2026-01-02 03:04:05')
    expect(evaluate('$runIndex', context)).toBe(3)
    expect(evaluate('$workflow.name', context)).toBe('My workflow')
    expect(evaluate('$workflow.active', context)).toBe(true)
    expect(evaluate('$execution.id', context)).toBe('exec-1')
    expect(evaluate('$execution.mode', context)).toBe('manual')
  })

  it('calls builtins in global style', () => {
    const context: ExpressionContext = {
      json: { name: 'ada,grace', tags: ['x', 'y'], values: [1, 2, 3], flag: true },
    }
    expect(evaluate('split($json.name, ",")', context)).toEqual(['ada', 'grace'])
    expect(evaluate('toUpperCase($json.name)', context)).toBe('ADA,GRACE')
    expect(evaluate('trim("  x  ")', context)).toBe('x')
    expect(evaluate('join($json.tags, "-")', context)).toBe('x-y')
    expect(evaluate('sum($json.values)', context)).toBe(6)
    expect(evaluateError('keys($json.values)', context).message).toBe(
      'Argument must be an object',
    )
  })

  it('calls builtins in member style', () => {
    const context: ExpressionContext = {
      json: { name: 'ada,grace', tags: ['x', 'y'], values: [1, 2, 3], text: 'abc' },
    }
    expect(evaluate('$json.name.split(",")', context)).toEqual(['ada', 'grace'])
    expect(evaluate('$json.name.toUpperCase()', context)).toBe('ADA,GRACE')
    expect(evaluate('$json.text.trim()', context)).toBe('abc')
    expect(evaluate('$json.tags.join("-")', context)).toBe('x-y')
    expect(evaluate('$json.values.first()', context)).toBe(1)
    expect(evaluate('$json.values.last()', context)).toBe(3)
    expect(evaluate('$json.values.slice(0, 2)', context)).toEqual([1, 2])
    expect(evaluate('$json.values.sum()', context)).toBe(6)
    expect(evaluate('$json.text.length', context)).toBe(3)
    expect(evaluate('$json.values.length', context)).toBe(3)
  })

  it('falls back to the native method when a builtin cannot be called', () => {
    const context: ExpressionContext = { json: { name: 'ada', values: [1, 2] } }
    // `split` needs two arguments on the server, the native method does not.
    expect(evaluate('$json.name.split()', context)).toEqual(['ada'])
    expect(evaluate('$json.values.indexOf(2)', context)).toBe(1)
  })

  it('short-circuits optional chains', () => {
    const context: ExpressionContext = { json: { user: { name: 'Bob' } } }
    expect(evaluate('$json.user?.name', context)).toBe('Bob')
    expect(evaluate('$json.missing?.name', context)).toBeUndefined()
    expect(evaluate('$json.missing?.deep.value', context)).toBeUndefined()
    expect(evaluate('$json.missing?.name || "fallback"', context)).toBe('fallback')
  })

  it('reports runtime errors with a span', () => {
    const source = '={{ $json.a.b }}'
    const error = evaluateError(source, { json: {} })
    expect(error.message).toContain("Cannot read property 'b'")
    expect(source.slice(error.start, error.end)).toBe('$json.a.b')
  })

  it('reports the arity of a builtin like the server', () => {
    const source = 'split($json.name)'
    const error = evaluateError(source, { json: { name: 'ada' } })
    expect(error.message).toBe('split requires 2 arguments')
    expect(source.slice(error.start, error.end)).toBe(source)
  })

  it('reports unknown identifiers with a span', () => {
    const source = 'unknownName'
    const error = evaluateError(source)
    expect(error.message).toBe("Unknown identifier 'unknownName'")
    expect(source.slice(error.start, error.end)).toBe(source)
  })

  it('tells a syntax error from a runtime error', () => {
    expect(evaluateError('$json.').kind).toBe('syntax')
    expect(evaluateError('unknownName').kind).toBe('runtime')
    expect(evaluateError('$json.a.b', { json: {} }).kind).toBe('runtime')
  })

  it('evaluates a full parameter value', () => {
    const context: ExpressionContext = { json: { name: 'Bob', id: 7 } }
    expect(evaluateParameter('Hello {{ $json.name }}!', context)).toBe('Hello Bob!')
    expect(evaluateParameter('={{ $json.id }}', context)).toBe(7)
    expect(evaluateParameter('{{ $json.id }}-{{ $json.name }}', context)).toBe('7-Bob')
    expect(evaluateParameter('plain text', context)).toBe('plain text')
  })
})
