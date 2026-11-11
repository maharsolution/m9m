/**
 * Native JavaScript built-ins, mirroring the goja runtime.
 *
 * m9m evaluates expressions inside a real JavaScript engine (goja), so
 * besides the m9m function registry (`functions.ts`) the engine also
 * exposes the standard JavaScript built-ins. The browser preview
 * therefore supports the subset that is useful inside an expression:
 *
 * - `String` / `Array` / `Number` prototype methods
 * - the `Math` and `JSON` global objects
 * - `parseInt`, `parseFloat`, `String`, `Number`, `Boolean`
 *
 * Every method is implemented explicitly — no prototype patching and no
 * dynamic code execution.
 */

/** A native method that receives its receiver as the first argument. */
export type NativeMethod = (receiver: unknown, ...args: unknown[]) => unknown

/**
 * Map of native methods.
 *
 * `toString` and `valueOf` are declared explicitly: without them the
 * contextual type of those two properties would come from
 * `Object.prototype` (where they take no arguments), which makes the
 * `receiver` parameter implicitly `any`.
 */
interface NativeMethodMap {
  [name: string]: NativeMethod
  toString: NativeMethod
  valueOf: NativeMethod
}

/** Methods available on strings. */
const STRING_METHODS: NativeMethodMap = {
  charAt: (receiver, index) => String(receiver).charAt(Number(index)),
  charCodeAt: (receiver, index) => String(receiver).charCodeAt(Number(index)),
  codePointAt: (receiver, index) => String(receiver).codePointAt(Number(index)),
  concat: (receiver, ...args) => String(receiver).concat(...args.map(String)),
  endsWith: (receiver, search) => String(receiver).endsWith(String(search)),
  includes: (receiver, search) => String(receiver).includes(String(search)),
  indexOf: (receiver, search, position) =>
    String(receiver).indexOf(String(search), position === undefined ? 0 : Number(position)),
  lastIndexOf: (receiver, search) => String(receiver).lastIndexOf(String(search)),
  localeCompare: (receiver, other) => String(receiver).localeCompare(String(other)),
  normalize: (receiver, form) => String(receiver).normalize(form ? String(form) : undefined),
  padEnd: (receiver, length, pad) => String(receiver).padEnd(Number(length), pad ? String(pad) : ' '),
  padStart: (receiver, length, pad) =>
    String(receiver).padStart(Number(length), pad ? String(pad) : ' '),
  repeat: (receiver, count) => String(receiver).repeat(Number(count)),
  replace: (receiver, search, replacement) =>
    String(receiver).replace(String(search), String(replacement)),
  replaceAll: (receiver, search, replacement) => {
    // `String.prototype.replaceAll` is ES2021 and the project targets
    // ES2020, so the replacement is done manually.
    const source = String(receiver)
    const needle = String(search)
    if (needle === '') return source
    return source.split(needle).join(String(replacement))
  },
  slice: (receiver, start, end) =>
    String(receiver).slice(Number(start), end === undefined ? undefined : Number(end)),
  split: (receiver, separator) =>
    separator === undefined ? [String(receiver)] : String(receiver).split(String(separator)),
  startsWith: (receiver, search) => String(receiver).startsWith(String(search)),
  substr: (receiver, start, length) =>
    String(receiver).substr(Number(start), length === undefined ? undefined : Number(length)),
  substring: (receiver, start, end) =>
    String(receiver).substring(Number(start), end === undefined ? undefined : Number(end)),
  toLowerCase: (receiver) => String(receiver).toLowerCase(),
  toUpperCase: (receiver) => String(receiver).toUpperCase(),
  toString: (receiver) => String(receiver),
  trim: (receiver) => String(receiver).trim(),
  trimEnd: (receiver) => String(receiver).trimEnd(),
  trimStart: (receiver) => String(receiver).trimStart(),
  valueOf: (receiver) => String(receiver),
}

/** Methods available on arrays. */
const ARRAY_METHODS: NativeMethodMap = {
  at: (receiver, index) => (receiver as unknown[]).at(Number(index)),
  concat: (receiver, ...args) => (receiver as unknown[]).concat(...args),
  every: (receiver, predicate) => {
    const list = receiver as unknown[]
    if (typeof predicate !== 'function') return true
    return list.every((item, index) => Boolean((predicate as (v: unknown, i: number) => unknown)(item, index)))
  },
  find: (receiver, predicate) => {
    const list = receiver as unknown[]
    if (typeof predicate !== 'function') return undefined
    return list.find((item, index) => Boolean((predicate as (v: unknown, i: number) => unknown)(item, index)))
  },
  findIndex: (receiver, predicate) => {
    const list = receiver as unknown[]
    if (typeof predicate !== 'function') return -1
    return list.findIndex((item, index) =>
      Boolean((predicate as (v: unknown, i: number) => unknown)(item, index)),
    )
  },
  flat: (receiver, depth) => (receiver as unknown[]).flat(depth === undefined ? 1 : Number(depth)),
  flatMap: (receiver, callback) => {
    const list = receiver as unknown[]
    if (typeof callback !== 'function') return list
    return list.flatMap((item, index) => {
      const result = (callback as (v: unknown, i: number) => unknown)(item, index)
      return Array.isArray(result) ? result : [result]
    })
  },
  includes: (receiver, search) => (receiver as unknown[]).includes(search),
  indexOf: (receiver, search) => (receiver as unknown[]).indexOf(search),
  join: (receiver, separator) =>
    (receiver as unknown[]).join(separator === undefined ? ',' : String(separator)),
  lastIndexOf: (receiver, search) => (receiver as unknown[]).lastIndexOf(search),
  reverse: (receiver) => (receiver as unknown[]).slice().reverse(),
  slice: (receiver, start, end) =>
    (receiver as unknown[]).slice(Number(start), end === undefined ? undefined : Number(end)),
  some: (receiver, predicate) => {
    const list = receiver as unknown[]
    if (typeof predicate !== 'function') return false
    return list.some((item, index) => Boolean((predicate as (v: unknown, i: number) => unknown)(item, index)))
  },
  sort: (receiver) => (receiver as unknown[]).slice().sort(),
  toString: (receiver) => (receiver as unknown[]).join(','),
  valueOf: (receiver) => receiver,
}

/** Methods available on numbers. */
const NUMBER_METHODS: NativeMethodMap = {
  toFixed: (receiver, digits) => Number(receiver).toFixed(Number(digits)),
  toPrecision: (receiver, digits) => Number(receiver).toPrecision(Number(digits)),
  toString: (receiver, radix) =>
    radix === undefined ? String(receiver) : Number(receiver).toString(Number(radix)),
  valueOf: (receiver) => Number(receiver),
}

/**
 * Look up a native method for a receiver.
 *
 * @param receiver - the value the method is called on.
 * @param name - the method name.
 * @returns the method, or `undefined` when the receiver has no such method.
 */
export function getNativeMethod(receiver: unknown, name: string): NativeMethod | undefined {
  if (typeof receiver === 'string') return STRING_METHODS[name]
  if (Array.isArray(receiver)) return ARRAY_METHODS[name]
  if (typeof receiver === 'number') return NUMBER_METHODS[name]
  return undefined
}

/** The `Math` global object. */
export const MATH: Record<string, unknown> = {
  abs: Math.abs,
  ceil: Math.ceil,
  floor: Math.floor,
  max: Math.max,
  min: Math.min,
  pow: Math.pow,
  round: Math.round,
  sign: Math.sign,
  sqrt: Math.sqrt,
  trunc: Math.trunc,
  random: Math.random,
  PI: Math.PI,
  E: Math.E,
}

/** The `JSON` global object. */
export const JSON_OBJECT: Record<string, unknown> = {
  stringify: (value: unknown, replacer?: unknown, space?: unknown) =>
    JSON.stringify(value, replacer as (key: string, value: unknown) => unknown, space as number),
  parse: (value: unknown) => JSON.parse(String(value)),
}

/** Global functions the JavaScript runtime provides. */
export const GLOBAL_FUNCTIONS: Record<string, (...args: unknown[]) => unknown> = {
  parseInt: (value: unknown, radix?: unknown) =>
    Number.parseInt(String(value), radix === undefined ? undefined : Number(radix)),
  parseFloat: (value: unknown) => Number.parseFloat(String(value)),
  String: (value: unknown) => String(value),
  Number: (value: unknown) => Number(value),
  Boolean: (value: unknown) => Boolean(value),
  isNaN: (value: unknown) => Number.isNaN(Number(value)),
  isFinite: (value: unknown) => Number.isFinite(Number(value)),
  encodeURIComponent: (value: unknown) => encodeURIComponent(String(value)),
  decodeURIComponent: (value: unknown) => decodeURIComponent(String(value)),
  encodeURI: (value: unknown) => encodeURI(String(value)),
  decodeURI: (value: unknown) => decodeURI(String(value)),
}