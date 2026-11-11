/**
 * Self-contained test suite for the browser-side expression engine.
 *
 * The repository has no frontend test runner that can be relied on, so
 * this file is a plain script: it bundles itself with `esbuild` and
 * runs under Node.js.
 *
 * ```bash
 * cd web
 * npx esbuild src/components/workflow/parameters/expression/expression.test.ts \
 *   --bundle --platform=node --format=cjs --outfile="%TEMP%/m9m-expression-test.cjs"
 * node "%TEMP%/m9m-expression-test.cjs"
 * ```
 *
 * The file must not import anything from `node:*` and must not use
 * `process`, so it keeps type-checking against the DOM lib of the
 * application.
 */

import { evaluateExpression, evaluateValue, type ExpressionContext } from './evaluator'
import { parseExpression, parseValue } from './parser'
import { ExpressionSyntaxError } from './types'

/** Number of assertions that passed. */
let assertions = 0

/** One registered test case. */
interface TestCase {
  name: string
  run: () => void
}

/** All test cases, executed by {@link main}. */
const tests: TestCase[] = []

/** Register a test case. */
function test(name: string, run: () => void): void {
  tests.push({ name, run })
}

/** Format a value for a failure message. */
function format(value: unknown): string {
  if (typeof value === 'string') return JSON.stringify(value)
  if (value === undefined) return 'undefined'
  try {
    return JSON.stringify(value) ?? String(value)
  } catch {
    return String(value)
  }
}

/** True for plain objects (not arrays, not `null`). */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** Deep equality for the JSON-ish values the engine produces. */
function deepEqual(left: unknown, right: unknown): boolean {
  if (left === right) return true
  if (typeof left === 'number' && typeof right === 'number') {
    return Number.isNaN(left) && Number.isNaN(right)
  }
  if (Array.isArray(left) && Array.isArray(right)) {
    if (left.length !== right.length) return false
    return left.every((item, index) => deepEqual(item, right[index]))
  }
  if (isPlainObject(left) && isPlainObject(right)) {
    const leftKeys = Object.keys(left).sort()
    const rightKeys = Object.keys(right).sort()
    if (leftKeys.length !== rightKeys.length) return false
    return leftKeys.every(
      (key, index) => key === rightKeys[index] && deepEqual(left[key], right[key]),
    )
  }
  return false
}

/** Assert that a condition holds. */
function assertTrue(condition: boolean, message: string): void {
  if (!condition) throw new Error(message)
  assertions += 1
}

/** Assert that two values are deeply equal. */
function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (!deepEqual(actual, expected)) {
    throw new Error(
      `${message}\n  expected: ${format(expected)}\n  actual:   ${format(actual)}`,
    )
  }
  assertions += 1
}

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
): { message: string; start: number; end: number } {
  const result = evaluateExpression(source, context)
  if (result.ok) throw new Error(`evaluation of "${source}" unexpectedly succeeded`)
  return result.error
}

/* -------------------------------------------------------------------------- */
/* Parser                                                                     */
/* -------------------------------------------------------------------------- */

test('parses a member expression', () => {
  const node = parseExpression('$json.user.id')
  assertEqual(node.type, 'member', 'expected a member node')
  assertEqual(node.start, 0, 'expected the span to start at 0')
  assertEqual(node.end, 13, 'expected the span to end at 13')
})

test('parses literals and binary expressions', () => {
  assertEqual(parseExpression('42').type, 'literal', 'expected a literal node')
  assertEqual(parseExpression("'text'").type, 'literal', 'expected a literal node')
  assertEqual(parseExpression('1 + 2').type, 'binary', 'expected a binary node')
  assertEqual(parseExpression('$json.a ? 1 : 2').type, 'conditional', 'expected a conditional')
  assertEqual(parseExpression('!$json.flag').type, 'unary', 'expected a unary node')
  assertEqual(parseExpression('split($json.a, ",")').type, 'call', 'expected a call node')
})

test('rejects invalid expressions', () => {
  const invalid = ['', '$json.', '1 +', '(', '$json.a ===']
  for (const source of invalid) {
    let thrown: unknown
    try {
      parseExpression(source)
    } catch (error) {
      thrown = error
    }
    assertTrue(thrown instanceof ExpressionSyntaxError, `"${source}" should not parse`)
  }
})

test('reports the offset of a syntax error', () => {
  try {
    parseExpression('$json.')
    throw new Error('expected a syntax error')
  } catch (error) {
    assertTrue(error instanceof ExpressionSyntaxError, 'expected a syntax error')
    const syntaxError = error as ExpressionSyntaxError
    assertEqual(syntaxError.start, 6, 'expected the error to point behind the dot')
    assertEqual(syntaxError.end, 6, 'expected a zero width error span')
  }
})

test('splits a value into text and expression chunks', () => {
  const chunks = parseValue('Hello {{ $json.name }}!')
  assertEqual(chunks.length, 3, 'expected three chunks')
  assertEqual(chunks[0].type, 'text', 'expected a text chunk')
  assertEqual(chunks[1].type, 'expression', 'expected an expression chunk')
  assertEqual(chunks[2].type, 'text', 'expected a text chunk')
  const expression = chunks[1]
  assertTrue(expression.type === 'expression', 'expected an expression chunk')
  if (expression.type !== 'expression') throw new Error('expected an expression chunk')
  // The span of the parsed node is absolute inside the value.
  assertEqual(expression.node.start, 9, 'expected an absolute span')
  assertEqual(expression.node.end, 19, 'expected an absolute span')
})

/* -------------------------------------------------------------------------- */
/* Evaluator                                                                  */
/* -------------------------------------------------------------------------- */

test('evaluates ternaries and comparisons', () => {
  const context: ExpressionContext = { json: { age: 20, name: 'Bob' } }
  assertEqual(evaluate('$json.age >= 18 ? "adult" : "minor"', context), 'adult', 'ternary')
  assertEqual(evaluate('$json.age > 30 ? "adult" : "minor"', context), 'minor', 'ternary')
  assertEqual(evaluate('$json.age == 20', context), true, 'loose equality')
  assertEqual(evaluate('$json.age === "20"', context), false, 'strict equality')
  assertEqual(evaluate('$json.name !== "Alice"', context), true, 'strict inequality')
  assertEqual(evaluate('1 + 2 * 3', context), 7, 'precedence')
})

test('reads $json', () => {
  const context: ExpressionContext = { json: { user: { id: 7, name: 'Bob' }, tags: ['a', 'b'] } }
  assertEqual(evaluate('$json.user.id', context), 7, 'nested property')
  assertEqual(evaluate('$json.user.name', context), 'Bob', 'nested property')
  assertEqual(evaluate('$json.tags[1]', context), 'b', 'computed member')
  assertEqual(evaluate('$json.missing', context), undefined, 'missing property')
})

test('reads $input', () => {
  const context: ExpressionContext = {
    json: { id: 2 },
    input: [{ id: 1 }, { id: 2 }, { id: 3 }],
    itemIndex: 1,
  }
  assertEqual(evaluate('$input.all()', context), [{ id: 1 }, { id: 2 }, { id: 3 }], '$input.all()')
  assertEqual(
    evaluate('$input.first()', context),
    { json: { id: 1 }, binary: null, pairedItem: null },
    '$input.first()',
  )
  assertEqual(
    evaluate('$input.last()', context),
    { json: { id: 3 }, binary: null, pairedItem: null },
    '$input.last()',
  )
  assertEqual(
    evaluate('$input.item', context),
    { json: { id: 2 }, binary: null, pairedItem: null },
    '$input.item',
  )
  assertEqual(evaluate('$input.item.json.id', context), 2, '$input.item.json.id')
  assertEqual(evaluate('$input.first().json.id', context), 1, '$input.first().json.id')
})

test('reads $node and the $() shorthand', () => {
  const context: ExpressionContext = {
    node: { Set: { value: 5 }, Other: { value: 6 } },
  }
  assertEqual(evaluate('$node["Set"].json', context), { value: 5 }, '$node["Set"].json')
  assertEqual(evaluate('$("Set").json', context), { value: 5 }, '$("Set").json')
  assertEqual(evaluate('$node["Set"].json.value', context), 5, '$node["Set"].json.value')
  assertEqual(evaluate('$("Set").item.json.value', context), 5, '$("Set").item.json.value')
  assertEqual(evaluate('$("Other").first().json.value', context), 6, '$("Other").first()')
})

test('reads $now, $runIndex, $workflow and $execution', () => {
  const now = new Date(2026, 0, 2, 3, 4, 5, 678)
  const context: ExpressionContext = {
    now,
    runIndex: 3,
    workflow: { id: 'wf-1', name: 'My workflow', active: true },
    execution: { id: 'exec-1', mode: 'manual' },
  }
  assertEqual(evaluate('$now.toMillis()', context), now.getTime(), '$now.toMillis()')
  assertEqual(evaluate('$now.format("yyyy-MM-dd HH:mm:ss")', context), '2026-01-02 03:04:05', '$now.format()')
  assertEqual(evaluate('$runIndex', context), 3, '$runIndex')
  assertEqual(evaluate('$workflow.name', context), 'My workflow', '$workflow.name')
  assertEqual(evaluate('$workflow.active', context), true, '$workflow.active')
  assertEqual(evaluate('$execution.id', context), 'exec-1', '$execution.id')
  assertEqual(evaluate('$execution.mode', context), 'manual', '$execution.mode')
})

test('calls builtins in global style', () => {
  const context: ExpressionContext = {
    json: { name: 'ada,grace', tags: ['x', 'y'], values: [1, 2, 3], flag: true },
  }
  assertEqual(evaluate('split($json.name, ",")', context), ['ada', 'grace'], 'split')
  assertEqual(evaluate('toUpperCase($json.name)', context), 'ADA,GRACE', 'toUpperCase')
  assertEqual(evaluate('trim("  x  ")', context), 'x', 'trim')
  assertEqual(evaluate('join($json.tags, "-")', context), 'x-y', 'join')
  assertEqual(evaluate('sum($json.values)', context), 6, 'sum')
  const error = evaluateError('keys($json.values)', context)
  assertEqual(error.message, 'Argument must be an object', 'keys() on an array throws')
})

test('calls builtins in member style', () => {
  const context: ExpressionContext = {
    json: { name: 'ada,grace', tags: ['x', 'y'], values: [1, 2, 3], text: 'abc' },
  }
  assertEqual(evaluate('$json.name.split(",")', context), ['ada', 'grace'], '$json.name.split()')
  assertEqual(evaluate('$json.name.toUpperCase()', context), 'ADA,GRACE', '$json.name.toUpperCase()')
  assertEqual(evaluate('$json.text.trim()', context), 'abc', '$json.text.trim()')
  assertEqual(evaluate('$json.tags.join("-")', context), 'x-y', '$json.tags.join()')
  assertEqual(evaluate('$json.values.first()', context), 1, '$json.values.first()')
  assertEqual(evaluate('$json.values.last()', context), 3, '$json.values.last()')
  assertEqual(evaluate('$json.values.slice(0, 2)', context), [1, 2], '$json.values.slice()')
  assertEqual(evaluate('$json.values.sum()', context), 6, '$json.values.sum()')
  assertEqual(evaluate('$json.text.length', context), 3, '$json.text.length')
  assertEqual(evaluate('$json.values.length', context), 3, '$json.values.length')
})

test('falls back to the native method when a builtin cannot be called', () => {
  const context: ExpressionContext = { json: { name: 'ada', values: [1, 2] } }
  // `split` needs two arguments on the server, the native method does not.
  assertEqual(evaluate('$json.name.split()', context), ['ada'], 'native split fallback')
  assertEqual(evaluate('$json.values.indexOf(2)', context), 1, 'array indexOf')
})

test('short-circuits optional chains', () => {
  const context: ExpressionContext = { json: { user: { name: 'Bob' } } }
  assertEqual(evaluate('$json.user?.name', context), 'Bob', 'optional chain with a value')
  assertEqual(evaluate('$json.missing?.name', context), undefined, 'optional chain short-circuits')
  assertEqual(evaluate('$json.missing?.deep.value', context), undefined, 'short-circuits the chain')
  assertEqual(evaluate('$json.missing?.name || "fallback"', context), 'fallback', 'chain in a binary')
})

test('reports runtime errors with a span', () => {
  const source = '={{ $json.a.b }}'
  const error = evaluateError(source, { json: {} })
  assertTrue(error.message.includes("Cannot read property 'b'"), 'expected a property error')
  assertEqual(source.slice(error.start, error.end), '$json.a.b', 'expected the span of the member')
})

test('reports the arity of a builtin like the server', () => {
  const source = 'split($json.name)'
  const error = evaluateError(source, { json: { name: 'ada' } })
  assertEqual(error.message, 'split requires 2 arguments', 'expected the server wording')
  assertEqual(source.slice(error.start, error.end), source, 'expected the span of the call')
})

test('reports unknown identifiers with a span', () => {
  const source = 'unknownName'
  const error = evaluateError(source)
  assertEqual(error.message, "Unknown identifier 'unknownName'", 'expected an unknown identifier error')
  assertEqual(source.slice(error.start, error.end), source, 'expected the span of the identifier')
})

test('evaluates a full parameter value', () => {
  const context: ExpressionContext = { json: { name: 'Bob', id: 7 } }
  assertEqual(evaluateParameter('Hello {{ $json.name }}!', context), 'Hello Bob!', 'mixed value')
  assertEqual(evaluateParameter('={{ $json.id }}', context), 7, 'single expression')
  assertEqual(
    evaluateParameter('{{ $json.id }}-{{ $json.name }}', context),
    '7-Bob',
    'two expressions',
  )
  assertEqual(evaluateParameter('plain text', context), 'plain text', 'plain text')
})

/* -------------------------------------------------------------------------- */
/* Runner                                                                     */
/* -------------------------------------------------------------------------- */

/** Run every registered test case and report the result. */
function main(): void {
  const failures: string[] = []

  for (const item of tests) {
    try {
      item.run()
      console.log(`ok - ${item.name}`)
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      failures.push(`${item.name}: ${message}`)
      console.log(`not ok - ${item.name}`)
    }
  }

  if (failures.length > 0) {
    console.log('')
    for (const failure of failures) console.log(`  ${failure}`)
    throw new Error(`${failures.length} of ${tests.length} tests failed`)
  }

  console.log('')
  console.log(`${tests.length} tests passed (${assertions} assertions)`)
}

main()
