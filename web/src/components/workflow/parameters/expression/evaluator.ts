/**
 * AST interpreter for the m9m expression subset.
 *
 * The evaluator walks the AST produced by {@link ../parser} and resolves
 * it against an {@link ExpressionContext}. It never executes
 * user-provided code: there is no `eval`, no `new Function` and no
 * `with` anywhere in this file.
 *
 * The context mirrors the server-side proxies created in
 * `internal/expressions/data_proxy.go`:
 *
 * | Expression           | Server proxy                 | Browser value |
 * | -------------------- | ---------------------------- | ------------- |
 * | `$json`              | current item JSON payload    | `context.json` |
 * | `$input.all()`       | all incoming item payloads   | `context.input` |
 * | `$input.first()`     | first incoming item wrapper  | first of `context.input` |
 * | `$input.last()`      | last incoming item wrapper   | last of `context.input` |
 * | `$input.item`        | current item wrapper         | `context.input[context.itemIndex]` |
 * | `$node["Name"].json` | last output of node `Name`   | `context.node.Name` |
 * | `$workflow`          | workflow metadata            | `context.workflow` |
 * | `$execution`         | execution metadata           | `context.execution` |
 * | `$now`               | Luxon-style date proxy       | `context.now` |
 * | `$runIndex`          | current loop iteration       | `context.runIndex` |
 *
 * Like the Go engine, the m9m functions are *value first*: the value a
 * function works on is its first argument. That makes the global call
 * style and the method style equivalent:
 *
 * ```text
 * split($json.text, ',')   // global style
 * $json.text.split(',')    // method style
 * ```
 */

import {
  ExpressionEvaluationError,
  ExpressionSyntaxError,
  type ExpressionChunk,
  type ExpressionError,
  type ExpressionNode,
  type TextChunk,
} from './types'
import { parseExpression, parseValue } from './parser'
import {
  BUILTIN_FUNCTIONS,
  isTruthy,
  toNumberValue,
  toStringValue,
} from './functions'
import {
  GLOBAL_FUNCTIONS,
  JSON_OBJECT,
  MATH,
  getNativeMethod,
} from './native'

/**
 * Data an expression is evaluated against.
 *
 * Every field is optional — a missing `$json` resolves to `undefined`,
 * exactly like n8n does when no item is available.
 */
export interface ExpressionContext {
  /** Payload of the current item (`$json`). */
  json?: unknown
  /**
   * Items arriving from the previous node.
   *
   * Each entry is the raw payload of one item, i.e. what
   * `$input.all()` returns on the server.
   */
  input?: unknown[]
  /** Index of the current item inside `input` (`$input.item`). */
  itemIndex?: number
  /** Last output of every referenced node, keyed by node name. */
  node?: Record<string, unknown>
  /** Workflow metadata (`$workflow`). */
  workflow?: {
    id?: string
    name?: string
    active?: boolean
    versionId?: string
    settings?: { timezone?: string; executionOrder?: string }
  }
  /** Execution metadata (`$execution`). */
  execution?: {
    id?: string
    mode?: string
    startedAt?: number
    stoppedAt?: number
    restApiUrl?: string
    instanceBaseUrl?: string
    webhookBaseUrl?: string
  }
  /** Timestamp used by `$now` (defaults to the current time). */
  now?: Date
  /** Current loop iteration (`$runIndex`). */
  runIndex?: number
}

/** Result of evaluating an expression. */
export type EvaluationResult =
  | { ok: true; value: unknown }
  | { ok: false; error: ExpressionError }

/** Sentinel used by optional chaining when a chain is short-circuited. */
const CHAIN_ABORT = Symbol('chain-abort')

/** A value that optional chaining may short-circuit. */
type ChainValue = unknown | typeof CHAIN_ABORT

/** The n8n item envelope: `{ json, binary, pairedItem }`. */
interface ItemWrapper {
  json: unknown
  binary: unknown
  pairedItem: unknown
}

/** True when a value already has the n8n item shape. */
function isItemWrapper(value: unknown): value is ItemWrapper {
  return (
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    'json' in (value as Record<string, unknown>)
  )
}

/** Build the n8n item wrapper around a raw payload. */
function wrapItem(payload: unknown): ItemWrapper {
  if (isItemWrapper(payload)) {
    return {
      json: payload.json,
      binary: 'binary' in payload ? payload.binary : null,
      pairedItem: 'pairedItem' in payload ? payload.pairedItem : null,
    }
  }
  return { json: payload, binary: null, pairedItem: null }
}

/**
 * Convert a Luxon/Moment format pattern into the Go time layout the
 * server uses.
 *
 * Mirrors `luxonToGoFormat` in `internal/expressions/data_proxy.go`.
 */
function luxonToGoFormat(pattern: string): string {
  const replacements: Array<[string, string]> = [
    ['yyyy', '2006'],
    ['SSS', '000'],
    ['MM', '01'],
    ['dd', '02'],
    ['HH', '15'],
    ['mm', '04'],
    ['ss', '05'],
    ['yy', '06'],
  ]
  let out = pattern
  for (const [oldToken, newToken] of replacements) {
    out = out.split(oldToken).join(newToken)
  }
  return out
}

/** Format a date with Go layout tokens (`2006`, `01`, `02`, `15`, `04`, `05`). */
function formatGoLayout(date: Date, layout: string): string {
  const pad = (value: number, size = 2) => String(value).padStart(size, '0')
  const offsetMinutes = -date.getTimezoneOffset()
  const sign = offsetMinutes >= 0 ? '+' : '-'
  const absolute = Math.abs(offsetMinutes)
  return layout
    .replace(/2006/g, String(date.getFullYear()))
    .replace(/06/g, pad(date.getFullYear() % 100))
    .replace(/01/g, pad(date.getMonth() + 1))
    .replace(/02/g, pad(date.getDate()))
    .replace(/15/g, pad(date.getHours()))
    .replace(/04/g, pad(date.getMinutes()))
    .replace(/05/g, pad(date.getSeconds()))
    .replace(/000/g, pad(date.getMilliseconds(), 3))
    .replace(/Z07:00/g, `${sign}${pad(Math.floor(absolute / 60))}:${pad(absolute % 60)}`)
    .replace(/Z07/g, `${sign}${pad(Math.floor(absolute / 60))}`)
}

/** The Luxon-style `$now` proxy. */
interface NowProxy {
  toISOString(): string
  toMillis(): number
  toString(): string
  format(pattern: string): string
  getFullYear(): number
  getMonth(): number
  getDate(): number
  getHours(): number
  getMinutes(): number
  getSeconds(): number
  getTime(): number
}

/** Build the `$now` proxy for a given date. */
function createNowProxy(date: Date): NowProxy {
  return {
    toISOString: () => date.toISOString(),
    toMillis: () => date.getTime(),
    toString: () => date.toISOString(),
    format: (pattern: string) => formatGoLayout(date, luxonToGoFormat(pattern)),
    getFullYear: () => date.getFullYear(),
    getMonth: () => date.getMonth(),
    getDate: () => date.getDate(),
    getHours: () => date.getHours(),
    getMinutes: () => date.getMinutes(),
    getSeconds: () => date.getSeconds(),
    getTime: () => date.getTime(),
  }
}

/** Read a property from an object, function, array or string. */
function readProperty(target: unknown, key: string): unknown {
  if (target === null || target === undefined) return undefined
  if (typeof target === 'string') {
    if (key === 'length') return target.length
    const index = Number(key)
    return Number.isInteger(index) ? target[index] : undefined
  }
  if (typeof target === 'object' || typeof target === 'function') {
    const record = target as Record<string, unknown>
    if (key === 'length' && Array.isArray(record)) return record.length
    return record[key]
  }
  return undefined
}

/** Mutable state for a single evaluation run. */
interface EvaluatorState {
  context: ExpressionContext
  now: NowProxy
}

/**
 * Resolve a member expression to a value.
 *
 * Three sources are consulted, in this order:
 *
 * 1. native JavaScript methods (`"abc".toUpperCase()`),
 * 2. m9m functions in method style (`$json.text.split(',')`),
 * 3. plain properties (`$json.user.id`).
 */
function resolveMember(
  receiver: unknown,
  key: string,
  node: Extract<ExpressionNode, { type: 'member' }>,
): unknown {
  const isPrimitive = typeof receiver === 'string' || typeof receiver === 'number'

  if (isPrimitive || Array.isArray(receiver)) {
    if (key === 'length' && typeof receiver !== 'number') {
      return (receiver as string | unknown[]).length
    }

    const native = getNativeMethod(receiver, key)
    if (native) {
      return (...args: unknown[]) => native(receiver, ...args)
    }

    const builtin = BUILTIN_FUNCTIONS[key]
    if (builtin) {
      // m9m functions are value-first: the receiver is the first argument.
      return (...args: unknown[]) => builtin([receiver, ...args])
    }
  }

  const value = readProperty(receiver, key)
  if (value === undefined && node.computed) {
    return undefined
  }
  return value
}

/** Evaluate an expression AST against the current state. */
function evaluateNode(node: ExpressionNode, state: EvaluatorState): ChainValue {
  switch (node.type) {
    case 'literal':
      return node.value

    case 'identifier':
      return resolveIdentifier(node.name, state, node.start, node.end)

    case 'member': {
      const receiver = evaluateNode(node.object, state)
      if (receiver === CHAIN_ABORT) return CHAIN_ABORT
      if (receiver === null || receiver === undefined) {
        if (node.optional) return CHAIN_ABORT
        throw new ExpressionEvaluationError(
          `Cannot read property '${describeProperty(node)}' of ${String(receiver)}`,
          node.start,
          node.end,
        )
      }

      let key: string
      if (node.computed) {
        const keyValue = evaluateNode(node.property as ExpressionNode, state)
        if (keyValue === CHAIN_ABORT) return CHAIN_ABORT
        key = toStringValue(keyValue)
      } else {
        key = node.property as string
      }
      return resolveMember(receiver, key, node)
    }

    case 'call': {
      const callee = evaluateNode(node.callee, state)
      if (callee === CHAIN_ABORT) return CHAIN_ABORT
      if (callee === null || callee === undefined) {
        if (node.optional) return CHAIN_ABORT
        throw new ExpressionEvaluationError(
          'Cannot call a null or undefined value',
          node.start,
          node.end,
        )
      }

      const args: unknown[] = []
      for (const argument of node.args) {
        const value = evaluateNode(argument, state)
        if (value === CHAIN_ABORT) return CHAIN_ABORT
        args.push(value)
      }

      if (typeof callee === 'function') {
        return (callee as (...callArgs: unknown[]) => unknown)(...args)
      }

      throw new ExpressionEvaluationError('Value is not a function', node.start, node.end)
    }

    case 'unary': {
      const argument = evaluateNode(node.argument, state)
      if (argument === CHAIN_ABORT) return CHAIN_ABORT
      if (node.operator === '!') return !isTruthy(argument)
      return -toNumberValue(argument)
    }

    case 'binary': {
      const left = evaluateNode(node.left, state)
      if (left === CHAIN_ABORT) return CHAIN_ABORT
      const right = evaluateNode(node.right, state)
      if (right === CHAIN_ABORT) return CHAIN_ABORT
      return applyBinary(node.operator, left, right, node.start, node.end)
    }

    case 'conditional': {
      const test = evaluateNode(node.test, state)
      if (test === CHAIN_ABORT) return CHAIN_ABORT
      return isTruthy(test)
        ? evaluateNode(node.consequent, state)
        : evaluateNode(node.alternate, state)
    }

    default: {
      // Every node type is handled above, so `node` narrows to `never` here.
      // The cast keeps the error span available for the editor highlight.
      const unsupported = node as unknown as { start: number; end: number }
      throw new ExpressionEvaluationError(
        'Unsupported expression node',
        unsupported.start,
        unsupported.end,
      )
    }
  }
}

/** Resolve a bare identifier. */
function resolveIdentifier(
  name: string,
  state: EvaluatorState,
  start: number,
  end: number,
): unknown {
  switch (name) {
    case '$json':
      return state.context.json
    case '$input':
      return createInputProxy(state.context)
    case '$node':
      return createNodeRegistry(state.context)
    case '$':
      return createNodeLookup(state.context)
    case '$workflow':
      return state.context.workflow ?? {}
    case '$execution':
      return state.context.execution ?? {}
    case '$now':
      return state.now
    case '$runIndex':
      return state.context.runIndex ?? 0
    case 'Math':
      return MATH
    case 'JSON':
      return JSON_OBJECT
    default:
      if (Object.prototype.hasOwnProperty.call(BUILTIN_FUNCTIONS, name)) {
        const builtin = BUILTIN_FUNCTIONS[name]
        return (...args: unknown[]) => builtin(args)
      }
      if (Object.prototype.hasOwnProperty.call(GLOBAL_FUNCTIONS, name)) {
        const global = GLOBAL_FUNCTIONS[name]
        return (...args: unknown[]) => global(...args)
      }
      throw new ExpressionEvaluationError(`Unknown identifier '${name}'`, start, end)
  }
}

/** Human readable name of a member expression's property. */
function describeProperty(node: Extract<ExpressionNode, { type: 'member' }>): string {
  return node.computed ? 'computed' : (node.property as string)
}

/** Apply a binary operator using JavaScript semantics. */
function applyBinary(
  operator: string,
  left: unknown,
  right: unknown,
  start: number,
  end: number,
): unknown {
  switch (operator) {
    case '+':
      if (typeof left === 'string' || typeof right === 'string') {
        return toStringValue(left) + toStringValue(right)
      }
      return toNumberValue(left) + toNumberValue(right)
    case '-':
      return toNumberValue(left) - toNumberValue(right)
    case '*':
      return toNumberValue(left) * toNumberValue(right)
    case '/':
      return toNumberValue(left) / toNumberValue(right)
    case '%':
      return toNumberValue(left) % toNumberValue(right)
    case '==':
      return looseEquals(left, right)
    case '!=':
      return !looseEquals(left, right)
    case '===':
      return left === right
    case '!==':
      return left !== right
    case '<':
      return compare(left, right) < 0
    case '<=':
      return compare(left, right) <= 0
    case '>':
      return compare(left, right) > 0
    case '>=':
      return compare(left, right) >= 0
    case '&&':
      return isTruthy(left) ? right : left
    case '||':
      return isTruthy(left) ? left : right
    default:
      throw new ExpressionEvaluationError(`Unsupported operator '${operator}'`, start, end)
  }
}

/** Loose equality (`==`) following JavaScript rules. */
function looseEquals(left: unknown, right: unknown): boolean {
  if (left === null || left === undefined) return right === null || right === undefined
  if (right === null || right === undefined) return false
  if (typeof left === 'object' || typeof right === 'object') {
    return toStringValue(left) === toStringValue(right)
  }
  if (typeof left === 'number' && typeof right === 'string') {
    return left === toNumberValue(right)
  }
  if (typeof left === 'string' && typeof right === 'number') {
    return toNumberValue(left) === right
  }
  return left === right
}

/** Ordering comparison used by `<`, `<=`, `>` and `>=`. */
function compare(left: unknown, right: unknown): number {
  if (typeof left === 'string' && typeof right === 'string') {
    return left < right ? -1 : left > right ? 1 : 0
  }
  const leftNumber = toNumberValue(left)
  const rightNumber = toNumberValue(right)
  if (leftNumber < rightNumber) return -1
  if (leftNumber > rightNumber) return 1
  return 0
}

/** Build the `$input` proxy. */
function createInputProxy(context: ExpressionContext): Record<string, unknown> {
  const items = context.input ?? []
  const currentIndex = context.itemIndex ?? 0

  return {
    all: () => items.slice(),
    first: () => (items.length > 0 ? wrapItem(items[0]) : undefined),
    last: () => (items.length > 0 ? wrapItem(items[items.length - 1]) : undefined),
    item: items.length > 0 ? wrapItem(items[currentIndex] ?? items[0]) : undefined,
  }
}

/** Build the `$node` registry: callable *and* indexable by node name. */
function createNodeRegistry(context: ExpressionContext): (...args: unknown[]) => unknown {
  const nodes = context.node ?? {}
  const registry = ((name: unknown) => createNodeProxy(nodes[toStringValue(name)])) as
    ((...args: unknown[]) => unknown) & Record<string, unknown>

  for (const [name, output] of Object.entries(nodes)) {
    registry[name] = createNodeProxy(output)
  }
  return registry
}

/** Build the `$("Name")` lookup function. */
function createNodeLookup(context: ExpressionContext): (name: unknown) => unknown {
  const nodes = context.node ?? {}
  return (name: unknown) => createNodeProxy(nodes[toStringValue(name)])
}

/** Build the proxy returned by `$node["Name"]` / `$("Name")`. */
function createNodeProxy(output: unknown): Record<string, unknown> {
  const items = Array.isArray(output) ? output : output === undefined ? [] : [output]
  const last = items.length > 0 ? items[items.length - 1] : undefined
  return {
    json: last,
    first: () => (items.length > 0 ? wrapItem(items[0]) : undefined),
    last: () => (items.length > 0 ? wrapItem(items[items.length - 1]) : undefined),
    all: () => items.map((item) => wrapItem(item)),
    item: last === undefined ? undefined : wrapItem(last),
  }
}

/** Normalise a thrown error into an {@link ExpressionError}. */
function toExpressionError(error: unknown, offset: number): ExpressionError {
  if (error instanceof ExpressionSyntaxError || error instanceof ExpressionEvaluationError) {
    return {
      message: error.message,
      start: error.start + offset,
      end: error.end + offset,
    }
  }
  if (error instanceof Error) {
    return { message: error.message, start: offset, end: offset }
  }
  return { message: String(error), start: offset, end: offset }
}

/** Strip the `=` / `{{ … }}` envelope and return the body plus its offset. */
function stripEnvelope(source: string): { body: string; offset: number } {
  const match = source.match(/^(\s*)(=?)(\s*)\{\{([\s\S]*)\}\}(\s*)$/)
  if (!match) {
    const leading = source.match(/^\s*/)
    const leadingLength = leading ? leading[0].length : 0
    return { body: source.slice(leadingLength), offset: leadingLength }
  }
  const offset = match[1].length + match[2].length + match[3].length + 2
  return { body: match[4], offset }
}

/**
 * Evaluate an expression against a context.
 *
 * The source may be a bare expression (`$json.id`) or a full m9m
 * parameter value (`={{ $json.id }}`). Error offsets always refer to
 * the *original* source so the editor can highlight the offending span.
 *
 * @param source - expression source to evaluate.
 * @param context - data the expression is evaluated against.
 * @returns `{ ok: true, value }` or `{ ok: false, error }`.
 */
export function evaluateExpression(
  source: string,
  context: ExpressionContext = {},
): EvaluationResult {
  const state: EvaluatorState = {
    context,
    now: createNowProxy(context.now ?? new Date()),
  }

  try {
    const { body } = stripEnvelope(source)
    const node = parseExpression(body)
    const value = evaluateNode(node, state)
    return { ok: true, value: value === CHAIN_ABORT ? undefined : value }
  } catch (error) {
    const { offset } = stripEnvelope(source)
    return { ok: false, error: toExpressionError(error, offset) }
  }
}

/**
 * Evaluate a full parameter value that may mix text and expressions.
 *
 * A value that consists of a single expression resolves to its raw
 * value; a mixed value resolves to the concatenated string. That is the
 * same rule the Go engine applies in `buildJavaScriptCode`.
 */
export function evaluateValue(
  source: string,
  context: ExpressionContext = {},
): EvaluationResult {
  const state: EvaluatorState = {
    context,
    now: createNowProxy(context.now ?? new Date()),
  }

  try {
    // The server strips a leading `=` before splitting the value into chunks.
    const withoutPrefix = source.startsWith('=') ? source.slice(1) : source
    const chunks = parseValue(withoutPrefix)
    const expressionChunks = chunks.filter(
      (chunk): chunk is ExpressionChunk => chunk.type === 'expression',
    )
    const textChunks = chunks.filter((chunk): chunk is TextChunk => chunk.type === 'text')

    if (expressionChunks.length === 1 && textChunks.length === 0) {
      const value = evaluateNode(expressionChunks[0].node, state)
      return { ok: true, value: value === CHAIN_ABORT ? undefined : value }
    }

    let output = ''
    for (const chunk of chunks) {
      if (chunk.type === 'text') {
        output += chunk.value
        continue
      }
      const value = evaluateNode(chunk.node, state)
      if (value === CHAIN_ABORT || value === undefined || value === null) continue
      output += toStringValue(value)
    }
    return { ok: true, value: output }
  } catch (error) {
    const offset = source.startsWith('=') ? 1 : 0
    return { ok: false, error: toExpressionError(error, offset) }
  }
}

/**
 * Build the preview context from the `sampleInput` prop.
 *
 * When the sample is an array it is treated as the list of incoming
 * items, otherwise it is treated as the payload of the current item.
 */
export function createPreviewContext(sampleInput: unknown): ExpressionContext {
  if (Array.isArray(sampleInput)) {
    return {
      json: sampleInput[0],
      input: sampleInput.slice(),
      itemIndex: 0,
    }
  }
  return {
    json: sampleInput,
    input: sampleInput === undefined ? [] : [sampleInput],
    itemIndex: 0,
  }
}

/** Format a resolved value for the preview row. */
export function formatPreview(value: unknown): string {
  if (value === undefined) return 'undefined'
  if (value === null) return 'null'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return toStringValue(value)
  }
}
