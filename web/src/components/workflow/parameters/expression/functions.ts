/**
 * Browser-side implementations of the m9m expression functions.
 *
 * The catalogue mirrors the functions the Go engine actually registers
 * in `internal/expressions/functions.go` and
 * `internal/expressions/functions_extended.go`:
 *
 * | Extension      | Registered functions |
 * | -------------- | -------------------- |
 * | `StringExtensions` | split, join, substring, substr, replace, replaceAll, trim, trimStart, trimEnd, toLowerCase, toUpperCase, lowercase, uppercase, lower, upper, charAt, charCodeAt, indexOf, lastIndexOf, includes, startsWith, endsWith, padStart, padEnd, repeat, slice, base64Encode, base64Decode, urlEncode, urlDecode, htmlEncode, htmlDecode, md5, sha1, sha256, sha512, isEmail, isUrl, isDomain, stripTags, extractDomain, extractUrl, toTitleCase, toCamelCase, toSnakeCase, toKebabCase |
 * | `MathExtensions`   | min, max, average, sum, round, ceil, floor, abs, random, randomInt, add, subtract, multiply, divide, modulo, pow, sqrt, sin, cos, tan |
 * | `ArrayExtensions`  | first, last, unique, compact, flatten, chunk, pluck, randomItem, length, indexOf, includes, slice, sort, reverse |
 * | `DateExtensions`   | now, formatDate, toDate, addDays, subtractDays, diffDays, getTime, dateFormat, addHours, addMinutes |
 * | `LogicExtensions`  | if, and, or, not, isEmpty, isNotEmpty, equal, notEqual, greaterThan, lessThan |
 * | `ObjectExtensions` | keys, values, merge, pick, omit, has |
 * | `UtilityExtensions`| toJson, fromJson, uuid, typeOf, randomString |
 *
 * `filter`, `map`, `reduce` (array) and `sleep` (utility) are listed by
 * `GetFunctionNames()` on the Go side but are **not** registered in the
 * runtime, so they are deliberately absent here as well.
 *
 * Every function uses the Go *value-first* argument order, so the same
 * implementation serves both call styles:
 *
 * ```text
 * split($json.text, ',')   // global style (like the Go registry)
 * $json.text.split(',')    // method style (like JavaScript)
 * ```
 */

/** Categories the server groups its functions into. */
import {
  base64Decode as base64DecodeImpl,
  base64Encode as base64EncodeImpl,
  md5 as md5Impl,
  sha1 as sha1Impl,
  sha256 as sha256Impl,
  sha512 as sha512Impl,
} from './hash'

export type FunctionCategory = 'string' | 'math' | 'array' | 'date' | 'logic' | 'object' | 'utility'

/** Metadata for one function, used by the autocomplete catalogue. */
export interface FunctionDefinition {
  /** Registered function name. */
  name: string
  /** Server extension the function belongs to. */
  category: FunctionCategory
  /** Short human readable summary. */
  description: string
  /** Argument list, value-first, e.g. `split(string, separator, limit?)`. */
  signature: string
  /** Example call. */
  example: string
}

/** A callable implementation. Arguments follow the Go value-first order. */
export type BuiltinFunction = (args: unknown[]) => unknown

/* -------------------------------------------------------------------------- */
/* Coercion helpers (mirroring goja's `.String()` / `.ToFloat()` behaviour). */
/* -------------------------------------------------------------------------- */

/** Convert a value to its string form the way goja's `String()` does. */
export function toStringValue(value: unknown): string {
  if (value === null) return 'null'
  if (value === undefined) return 'undefined'
  if (typeof value === 'string') return value
  if (typeof value === 'number') return String(value)
  if (typeof value === 'boolean') return value ? 'true' : 'false'
  if (Array.isArray(value)) return value.map(toStringValue).join(',')
  if (typeof value === 'object') {
    const maybeToString = (value as { toString?: () => string }).toString
    if (typeof maybeToString === 'function' && maybeToString !== Object.prototype.toString) {
      return maybeToString.call(value)
    }
    return '[object Object]'
  }
  return String(value)
}

/** Convert a value to a number the way goja's `ToFloat()` does. */
export function toNumberValue(value: unknown): number {
  if (typeof value === 'number') return value
  if (typeof value === 'boolean') return value ? 1 : 0
  if (value === null) return 0
  if (value === undefined) return Number.NaN
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (trimmed === '') return 0
    return Number(trimmed)
  }
  return Number.NaN
}

/** Convert a value to an integer the way goja's `ToInteger()` does. */
export function toIntegerValue(value: unknown): number {
  const number = toNumberValue(value)
  if (Number.isNaN(number)) return 0
  return Math.trunc(number)
}

/** Truthiness as implemented by `LogicExtensions.isTruthy`. */
export function isTruthy(value: unknown): boolean {
  if (value === null || value === undefined) return false
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value !== 0
  if (typeof value === 'string') return value !== ''
  return true
}

/** Emptiness as implemented by `LogicExtensions.isEmptyValue`. */
export function isEmptyValue(value: unknown): boolean {
  if (value === null || value === undefined) return true
  if (typeof value === 'string') return value === ''
  if (Array.isArray(value)) return value.length === 0
  if (typeof value === 'object') return Object.keys(value as object).length === 0
  return false
}

/** Stable string key for a value, matching Go's `fmt.Sprintf("%v")` comparison. */
function valueKey(value: unknown): string {
  if (value === null) return '<nil>'
  if (value === undefined) return '<nil>'
  if (Array.isArray(value)) return `[${value.map(valueKey).join(' ')}]`
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    return `map[${entries.map(([key, item]) => `${key}:${valueKey(item)}`).join(' ')}]`
  }
  return toStringValue(value)
}

/** True when the value is a plain object (not an array, not null). */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/* -------------------------------------------------------------------------- */
/* String helpers                                                             */
/* -------------------------------------------------------------------------- */

/** Split a string into parts (Go `split`). */
function splitString(args: unknown[]): unknown[] {
  const source = toStringValue(args[0])
  const separator = toStringValue(args[1])
  const parts = separator === '' ? source.split('') : source.split(separator)
  if (args.length > 2) {
    const limit = toIntegerValue(args[2])
    if (limit > 0 && parts.length > limit) return parts.slice(0, limit)
  }
  return parts
}

/** Join an array into a string (Go `join`). */
function joinArray(args: unknown[]): string {
  const list = Array.isArray(args[0]) ? args[0] : []
  const separator = toStringValue(args[1])
  return list.map(toStringValue).join(separator)
}

/** Go `substring`: clamp the bounds and swap them when reversed. */
function substringGo(args: unknown[]): string {
  const source = toStringValue(args[0])
  let start = toIntegerValue(args[1])
  let end = args.length > 2 ? toIntegerValue(args[2]) : source.length
  if (start < 0) start = 0
  if (end > source.length) end = source.length
  if (start > end) [start, end] = [end, start]
  return source.slice(start, end)
}

/** Go `substr`: start offset plus optional length. */
function substrGo(args: unknown[]): string {
  const source = toStringValue(args[0])
  let start = toIntegerValue(args[1])
  let length = args.length > 2 ? toIntegerValue(args[2]) : source.length - start
  if (start < 0) start = 0
  if (start >= source.length) return ''
  if (length < 0) length = 0
  const end = Math.min(start + length, source.length)
  return source.slice(start, end)
}

/** Go `slice` for strings: supports negative indices. */
function sliceString(args: unknown[]): string {
  const source = toStringValue(args[0])
  let start = toIntegerValue(args[1])
  let end = args.length > 2 ? toIntegerValue(args[2]) : source.length
  if (start < 0) start = source.length + start
  if (end < 0) end = source.length + end
  if (start < 0) start = 0
  if (end > source.length) end = source.length
  if (start > end) return ''
  return source.slice(start, end)
}

/** Pad a string on the left. */
function padStart(args: unknown[]): string {
  const source = toStringValue(args[0])
  const targetLength = toIntegerValue(args[1])
  const padString = args.length > 2 ? toStringValue(args[2]) : ' '
  if (source.length >= targetLength) return source
  if (padString === '') return source
  const missing = targetLength - source.length
  const padding = padString.repeat(Math.ceil(missing / padString.length)).slice(0, missing)
  return padding + source
}

/** Pad a string on the right. */
function padEnd(args: unknown[]): string {
  const source = toStringValue(args[0])
  const targetLength = toIntegerValue(args[1])
  const padString = args.length > 2 ? toStringValue(args[2]) : ' '
  if (source.length >= targetLength) return source
  if (padString === '') return source
  const missing = targetLength - source.length
  const padding = padString.repeat(Math.ceil(missing / padString.length)).slice(0, missing)
  return source + padding
}

/** Replace the first occurrence of a substring. */
function replaceFirst(args: unknown[]): string {
  const source = toStringValue(args[0])
  const search = toStringValue(args[1])
  const replacement = toStringValue(args[2])
  const index = source.indexOf(search)
  if (index === -1) return source
  return source.slice(0, index) + replacement + source.slice(index + search.length)
}

/** Replace every occurrence of a substring. */
function replaceAll(args: unknown[]): string {
  const source = toStringValue(args[0])
  const search = toStringValue(args[1])
  const replacement = toStringValue(args[2])
  if (search === '') return source
  return source.split(search).join(replacement)
}

/** Lowercase a string. */
function toLowerCase(args: unknown[]): string {
  return toStringValue(args[0]).toLowerCase()
}

/** Uppercase a string. */
function toUpperCase(args: unknown[]): string {
  return toStringValue(args[0]).toUpperCase()
}

/** Trim whitespace from both ends. */
function trim(args: unknown[]): string {
  return toStringValue(args[0]).trim()
}

/** Trim leading whitespace. */
function trimStart(args: unknown[]): string {
  return toStringValue(args[0]).replace(/^\s+/, '')
}

/** Trim trailing whitespace. */
function trimEnd(args: unknown[]): string {
  return toStringValue(args[0]).replace(/\s+$/, '')
}

/** Character at an index (1-based in n8n, 0-based like Go). */
function charAt(args: unknown[]): string {
  const source = toStringValue(args[0])
  const index = toIntegerValue(args[1])
  if (index < 0 || index >= source.length) return ''
  return source[index]
}

/** UTF-16 code unit at an index. */
function charCodeAt(args: unknown[]): number {
  const source = toStringValue(args[0])
  const index = toIntegerValue(args[1])
  if (index < 0 || index >= source.length) return Number.NaN
  return source.charCodeAt(index)
}

/** Index of a substring. */
function indexOfString(args: unknown[]): number {
  const source = toStringValue(args[0])
  const search = toStringValue(args[1])
  let start = args.length > 2 ? toIntegerValue(args[2]) : 0
  if (start < 0) start = 0
  if (start >= source.length) return -1
  return source.indexOf(search, start)
}

/** Last index of a substring. */
function lastIndexOfString(args: unknown[]): number {
  const source = toStringValue(args[0])
  return source.lastIndexOf(toStringValue(args[1]))
}

/** Whether a string contains a substring. */
function includesString(args: unknown[]): boolean {
  return toStringValue(args[0]).includes(toStringValue(args[1]))
}

/** Whether a string starts with a prefix. */
function startsWith(args: unknown[]): boolean {
  return toStringValue(args[0]).startsWith(toStringValue(args[1]))
}

/** Whether a string ends with a suffix. */
function endsWith(args: unknown[]): boolean {
  return toStringValue(args[0]).endsWith(toStringValue(args[1]))
}

/** Repeat a string. */
function repeat(args: unknown[]): string {
  const source = toStringValue(args[0])
  const count = toIntegerValue(args[1])
  if (count < 0) throw new Error('RangeError: repeat count must be non-negative')
  return source.repeat(count)
}

/** Escape HTML special characters. */
function htmlEncode(args: unknown[]): string {
  return toStringValue(args[0])
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&#34;')
    .replace(/'/g, '&#39;')
}

/** Reverse of {@link htmlEncode}. */
function htmlDecode(args: unknown[]): string {
  return toStringValue(args[0])
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&#34;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&amp;/g, '&')
}

/** Percent-encode a string (`url.QueryEscape` semantics). */
function urlEncode(args: unknown[]): string {
  return encodeURIComponent(toStringValue(args[0]))
    .replace(/[!'()*]/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`)
    .replace(/%20/g, '+')
}

/** Decode a percent-encoded string (`url.QueryUnescape` semantics). */
function urlDecode(args: unknown[]): string {
  const source = toStringValue(args[0]).replace(/\+/g, ' ')
  try {
    return decodeURIComponent(source)
  } catch {
    throw new Error('DecodeError: Invalid URL encoded string')
  }
}

const EMAIL_PATTERN = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/
const DOMAIN_PATTERN = /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/

/** Whether a string looks like an e-mail address. */
function isEmail(args: unknown[]): boolean {
  return EMAIL_PATTERN.test(toStringValue(args[0]))
}

/** Whether a string parses as an absolute URL. */
function isUrl(args: unknown[]): boolean {
  const source = toStringValue(args[0])
  try {
    // eslint-disable-next-line no-new
    new URL(source)
    return true
  } catch {
    return false
  }
}

/** Whether a string looks like a domain name. */
function isDomain(args: unknown[]): boolean {
  return DOMAIN_PATTERN.test(toStringValue(args[0]))
}

/** Remove HTML/XML tags. */
function stripTags(args: unknown[]): string {
  return toStringValue(args[0]).replace(/<[^>]*>/g, '')
}

/** Host name of the first URL inside a string. */
function extractDomain(args: unknown[]): string {
  const source = toStringValue(args[0])
  try {
    return new URL(source).hostname
  } catch {
    return ''
  }
}

/** Every URL found inside a string. */
function extractUrl(args: unknown[]): string[] {
  return toStringValue(args[0]).match(/https?:\/\/[^\s]+/g) ?? []
}

/** Split a string into words on whitespace, `_` and `-`. */
function words(value: string): string[] {
  return value.replace(/[_-]/g, ' ').trim().split(/\s+/).filter((word) => word !== '')
}

/** Capitalise the first letter of a word. */
function capitalise(word: string): string {
  return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase()
}

/** `hello world` → `Hello World`. */
function toTitleCase(args: unknown[]): string {
  return words(toStringValue(args[0]).toLowerCase()).map(capitalise).join(' ')
}

/** `hello world` → `helloWorld`. */
function toCamelCase(args: unknown[]): string {
  const parts = words(toStringValue(args[0]).toLowerCase())
  if (parts.length === 0) return ''
  return parts[0] + parts.slice(1).map(capitalise).join('')
}

/** `hello world` → `hello_world`. */
function toSnakeCase(args: unknown[]): string {
  return words(toStringValue(args[0]).toLowerCase()).join('_')
}

/** `hello world` → `hello-world`. */
function toKebabCase(args: unknown[]): string {
  return words(toStringValue(args[0]).toLowerCase()).join('-')
}

/* -------------------------------------------------------------------------- */
/* Math helpers                                                               */
/* -------------------------------------------------------------------------- */

/** Flatten the arguments of a math function (Go `flattenArgs`). */
function flattenNumbers(args: unknown[]): number[] {
  if (args.length === 1 && Array.isArray(args[0])) {
    return args[0].map(toNumberValue)
  }
  return args.map(toNumberValue)
}

/** Smallest value. */
function min(args: unknown[]): number {
  const values = flattenNumbers(args)
  if (values.length === 0) return Number.POSITIVE_INFINITY
  return Math.min(...values)
}

/** Largest value. */
function max(args: unknown[]): number {
  const values = flattenNumbers(args)
  if (values.length === 0) return Number.NEGATIVE_INFINITY
  return Math.max(...values)
}

/** Arithmetic mean. */
function average(args: unknown[]): number {
  const values = flattenNumbers(args)
  if (values.length === 0) return 0
  return values.reduce((total, value) => total + value, 0) / values.length
}

/** Sum of all values. */
function sum(args: unknown[]): number {
  return flattenNumbers(args).reduce((total, value) => total + value, 0)
}

/** Round to a given precision. */
function round(args: unknown[]): number {
  const value = toNumberValue(args[0])
  const precision = args.length > 1 ? toIntegerValue(args[1]) : 0
  const multiplier = 10 ** precision
  return Math.round(value * multiplier) / multiplier
}

/** Round up. */
function ceil(args: unknown[]): number {
  return Math.ceil(toNumberValue(args[0]))
}

/** Round down. */
function floor(args: unknown[]): number {
  return Math.floor(toNumberValue(args[0]))
}

/** Absolute value. */
function abs(args: unknown[]): number {
  return Math.abs(toNumberValue(args[0]))
}

/** Pseudo random float in `[0, 1)`. */
function random(): number {
  return Math.random()
}

/** Pseudo random integer in an inclusive range. */
function randomInt(args: unknown[]): number {
  let minValue = 0
  let maxValue = toIntegerValue(args[0])
  if (args.length > 1) {
    minValue = maxValue
    maxValue = toIntegerValue(args[1])
  }
  if (minValue > maxValue) [minValue, maxValue] = [maxValue, minValue]
  return Math.floor(Math.random() * (maxValue - minValue + 1)) + minValue
}

/** Difference of two numbers. */
function subtract(args: unknown[]): number {
  return toNumberValue(args[0]) - toNumberValue(args[1])
}

/** Product of all numbers. */
function multiply(args: unknown[]): number {
  return args.map(toNumberValue).reduce((total, value) => total * value, 1)
}

/** Quotient of two numbers; division by zero yields `Infinity`. */
function divide(args: unknown[]): number {
  const divisor = toNumberValue(args[1])
  if (divisor === 0) return Number.POSITIVE_INFINITY
  return toNumberValue(args[0]) / divisor
}

/** Remainder of a division. */
function modulo(args: unknown[]): number {
  return toNumberValue(args[0]) % toNumberValue(args[1])
}

/** Exponentiation. */
function pow(args: unknown[]): number {
  return toNumberValue(args[0]) ** toNumberValue(args[1])
}

/** Square root. */
function sqrt(args: unknown[]): number {
  return Math.sqrt(toNumberValue(args[0]))
}

/** Sine. */
function sin(args: unknown[]): number {
  return Math.sin(toNumberValue(args[0]))
}

/** Cosine. */
function cos(args: unknown[]): number {
  return Math.cos(toNumberValue(args[0]))
}

/** Tangent. */
function tan(args: unknown[]): number {
  return Math.tan(toNumberValue(args[0]))
}

/* -------------------------------------------------------------------------- */
/* Array helpers                                                              */
/* -------------------------------------------------------------------------- */

/** Normalise a value into an array (Go `toSlice`). */
function toArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

/** First element of an array. */
function first(args: unknown[]): unknown {
  const list = toArray(args[0])
  return list.length === 0 ? undefined : list[0]
}

/** Last element of an array. */
function last(args: unknown[]): unknown {
  const list = toArray(args[0])
  return list.length === 0 ? undefined : list[list.length - 1]
}

/** Remove duplicate entries. */
function unique(args: unknown[]): unknown[] {
  const seen = new Set<string>()
  const result: unknown[] = []
  for (const item of toArray(args[0])) {
    const key = valueKey(item)
    if (!seen.has(key)) {
      seen.add(key)
      result.push(item)
    }
  }
  return result
}

/** Remove falsy entries. */
function compact(args: unknown[]): unknown[] {
  return toArray(args[0]).filter((item) => isTruthy(item))
}

/** Flatten nested arrays. */
function flatten(args: unknown[]): unknown[] {
  const depth = args.length > 1 ? toIntegerValue(args[1]) : 1
  const flattenLevel = (list: unknown[], level: number): unknown[] => {
    if (level <= 0) return list
    const result: unknown[] = []
    for (const item of list) {
      if (Array.isArray(item)) result.push(...flattenLevel(item, level - 1))
      else result.push(item)
    }
    return result
  }
  return flattenLevel(toArray(args[0]), depth)
}

/** Split an array into chunks of a given size. */
function chunk(args: unknown[]): unknown[][] {
  const list = toArray(args[0])
  const size = toIntegerValue(args[1])
  if (size <= 0) return []
  const result: unknown[][] = []
  for (let index = 0; index < list.length; index += size) {
    result.push(list.slice(index, index + size))
  }
  return result
}

/** Collect one property from every object in an array. */
function pluck(args: unknown[]): unknown[] {
  const property = toStringValue(args[1])
  const result: unknown[] = []
  for (const item of toArray(args[0])) {
    if (isPlainObject(item) && property in item) result.push(item[property])
  }
  return result
}

/** Random element of an array. */
function randomItem(args: unknown[]): unknown {
  const list = toArray(args[0])
  if (list.length === 0) return undefined
  return list[Math.floor(Math.random() * list.length)]
}

/** Length of an array or string. */
function length(args: unknown[]): number {
  const value = args[0]
  if (typeof value === 'string') return value.length
  return toArray(value).length
}

/** Index of an item inside an array. */
function indexOfArray(args: unknown[]): number {
  const search = valueKey(args[1])
  return toArray(args[0]).findIndex((item) => valueKey(item) === search)
}

/** Whether an array contains an item. */
function includesArray(args: unknown[]): boolean {
  const search = valueKey(args[1])
  return toArray(args[0]).some((item) => valueKey(item) === search)
}

/** Slice of an array. */
function sliceArray(args: unknown[]): unknown[] {
  const list = toArray(args[0])
  let start = toIntegerValue(args[1])
  let end = args.length > 2 ? toIntegerValue(args[2]) : list.length
  if (start < 0) start = list.length + start
  if (end < 0) end = list.length + end
  if (start < 0) start = 0
  if (end > list.length) end = list.length
  if (start > end) return []
  return list.slice(start, end)
}

/** Sort an array by its string representation. */
function sortArray(args: unknown[]): unknown[] {
  return toArray(args[0])
    .slice()
    .sort((left, right) => (valueKey(left) < valueKey(right) ? -1 : valueKey(left) > valueKey(right) ? 1 : 0))
}

/** Reverse an array. */
function reverseArray(args: unknown[]): unknown[] {
  return toArray(args[0]).slice().reverse()
}

/* -------------------------------------------------------------------------- */
/* Date helpers                                                               */
/* -------------------------------------------------------------------------- */

/** Current timestamp in milliseconds (second precision, like the server). */
function now(): number {
  return Math.floor(Date.now() / 1000) * 1000
}

/** Parse a date-ish value into a `Date`, or `null` when it is invalid. */
function toDateValue(value: unknown): Date | null {
  if (value instanceof Date) return Number.isNaN(value.getTime()) ? null : value
  if (typeof value === 'number') {
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? null : date
  }
  if (typeof value === 'string') {
    const trimmed = value.trim()
    const direct = new Date(trimmed)
    if (!Number.isNaN(direct.getTime())) return direct
    const parsed = Date.parse(trimmed.replace(' ', 'T'))
    return Number.isNaN(parsed) ? null : new Date(parsed)
  }
  return null
}

/** Format a date using the Go-style tokens the server understands. */
function formatDateParts(date: Date, format: string): string {
  const pad = (value: number, size = 2) => String(value).padStart(size, '0')
  return format.replace(/yyyy|yy|MM|dd|HH|mm|ss/g, (token) => {
    switch (token) {
      case 'yyyy':
        return String(date.getFullYear())
      case 'yy':
        return pad(date.getFullYear() % 100)
      case 'MM':
        return pad(date.getMonth() + 1)
      case 'dd':
        return pad(date.getDate())
      case 'HH':
        return pad(date.getHours())
      case 'mm':
        return pad(date.getMinutes())
      case 'ss':
        return pad(date.getSeconds())
      default:
        return token
    }
  })
}

/** Format a timestamp with a Go-style pattern. */
function formatDate(args: unknown[]): string {
  const date = toDateValue(args[0])
  if (!date) throw new Error('First argument must be a date or timestamp')
  return formatDateParts(date, toStringValue(args[1]))
}

/** Convert a value into a millisecond timestamp. */
function toDate(args: unknown[]): number {
  const value = args[0]
  if (typeof value === 'number') return value
  const date = toDateValue(value)
  if (!date) throw new Error('Invalid date string')
  return date.getTime()
}

/** Add days to a timestamp. */
function addDays(args: unknown[]): number {
  const date = new Date(toNumberValue(args[0]))
  date.setDate(date.getDate() + toIntegerValue(args[1]))
  return Math.floor(date.getTime() / 1000) * 1000
}

/** Subtract days from a timestamp. */
function subtractDays(args: unknown[]): number {
  const date = new Date(toNumberValue(args[0]))
  date.setDate(date.getDate() - toIntegerValue(args[1]))
  return Math.floor(date.getTime() / 1000) * 1000
}

/** Difference between two timestamps in days. */
function diffDays(args: unknown[]): number {
  const firstDate = new Date(toNumberValue(args[0]))
  const secondDate = new Date(toNumberValue(args[1]))
  return (secondDate.getTime() - firstDate.getTime()) / (1000 * 60 * 60 * 24)
}

/** Add hours to a timestamp. */
function addHours(args: unknown[]): number {
  const date = new Date(toNumberValue(args[0]))
  date.setHours(date.getHours() + toIntegerValue(args[1]))
  return Math.floor(date.getTime() / 1000) * 1000
}

/** Add minutes to a timestamp. */
function addMinutes(args: unknown[]): number {
  const date = new Date(toNumberValue(args[0]))
  date.setMinutes(date.getMinutes() + toIntegerValue(args[1]))
  return Math.floor(date.getTime() / 1000) * 1000
}

/* -------------------------------------------------------------------------- */
/* Logic / object / utility helpers                                           */
/* -------------------------------------------------------------------------- */

/** Ternary helper (`if`). */
function ifFunction(args: unknown[]): unknown {
  return isTruthy(args[0]) ? args[1] : args[2]
}

/** Logical AND over all arguments. */
function andFunction(args: unknown[]): boolean {
  return args.every((value) => isTruthy(value))
}

/** Logical OR over all arguments. */
function orFunction(args: unknown[]): boolean {
  return args.some((value) => isTruthy(value))
}

/** Logical NOT. */
function notFunction(args: unknown[]): boolean {
  return !isTruthy(args[0])
}

/** Whether a value is empty. */
function isEmpty(args: unknown[]): boolean {
  if (args.length === 0) return true
  return isEmptyValue(args[0])
}

/** Whether a value is not empty. */
function isNotEmpty(args: unknown[]): boolean {
  if (args.length === 0) return false
  return !isEmptyValue(args[0])
}

/** Loose equality by string representation (Go `equal`). */
function equal(args: unknown[]): boolean {
  return valueKey(args[0]) === valueKey(args[1])
}

/** Loose inequality by string representation (Go `notEqual`). */
function notEqual(args: unknown[]): boolean {
  return valueKey(args[0]) !== valueKey(args[1])
}

/** Numeric comparison. */
function greaterThan(args: unknown[]): boolean {
  return toNumberValue(args[0]) > toNumberValue(args[1])
}

/** Numeric comparison. */
function lessThan(args: unknown[]): boolean {
  return toNumberValue(args[0]) < toNumberValue(args[1])
}

/** Keys of an object. */
function keys(args: unknown[]): string[] {
  if (!isPlainObject(args[0])) throw new Error('Argument must be an object')
  return Object.keys(args[0])
}

/** Values of an object. */
function values(args: unknown[]): unknown[] {
  if (!isPlainObject(args[0])) throw new Error('Argument must be an object')
  return Object.values(args[0])
}

/** Merge objects, later keys win. */
function merge(args: unknown[]): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const argument of args) {
    if (isPlainObject(argument)) Object.assign(result, argument)
  }
  return result
}

/** Keys to keep, either one key or an array of keys. */
function pick(args: unknown[]): Record<string, unknown> {
  if (!isPlainObject(args[0])) throw new Error('First argument must be an object')
  const source = args[0]
  const wanted = Array.isArray(args[1]) ? args[1].map(toStringValue) : [toStringValue(args[1])]
  const result: Record<string, unknown> = {}
  for (const key of wanted) {
    if (key in source) result[key] = source[key]
  }
  return result
}

/** Keys to remove, either one key or an array of keys. */
function omit(args: unknown[]): Record<string, unknown> {
  if (!isPlainObject(args[0])) throw new Error('First argument must be an object')
  const source = args[0]
  const unwanted = new Set(Array.isArray(args[1]) ? args[1].map(toStringValue) : [toStringValue(args[1])])
  const result: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(source)) {
    if (!unwanted.has(key)) result[key] = value
  }
  return result
}

/** Whether an object has a key. */
function has(args: unknown[]): boolean {
  if (!isPlainObject(args[0])) return false
  return toStringValue(args[1]) in args[0]
}

/** Serialise a value to JSON. */
function toJson(args: unknown[]): string {
  try {
    return JSON.stringify(args[0]) ?? 'null'
  } catch {
    throw new Error('Failed to stringify object')
  }
}

/** Parse a JSON string. */
function fromJson(args: unknown[]): unknown {
  try {
    return JSON.parse(toStringValue(args[0]))
  } catch {
    throw new Error('Failed to parse JSON string')
  }
}

/** Random UUID v4. */
function uuid(): string {
  const cryptoRef = globalThis.crypto
  if (cryptoRef && typeof cryptoRef.randomUUID === 'function') return cryptoRef.randomUUID()
  const bytes = new Uint8Array(16)
  if (cryptoRef && typeof cryptoRef.getRandomValues === 'function') {
    cryptoRef.getRandomValues(bytes)
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256)
    }
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

/** Runtime type name of a value. */
function typeOf(args: unknown[]): string {
  if (args.length === 0) return 'undefined'
  const value = args[0]
  if (value === null) return 'null'
  if (Array.isArray(value)) return 'array'
  if (typeof value === 'object') return 'object'
  return typeof value
}

/** Random alphanumeric string. */
function randomString(args: unknown[]): string {
  let size = args.length > 0 ? toIntegerValue(args[0]) : 8
  if (size <= 0) size = 8
  const charset = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
  let out = ''
  for (let index = 0; index < size; index += 1) {
    out += charset[Math.floor(Math.random() * charset.length)]
  }
  return out
}

/* -------------------------------------------------------------------------- */
/* Registry                                                                   */
/* -------------------------------------------------------------------------- */

/**
 * The functions the m9m server registers, keyed by name.
 *
 * `indexOf`, `includes`, `slice` and `length` exist in both the string
 * and the array extension on the server. The browser dispatches them on
 * the type of the first argument so both spellings behave like the
 * server.
 */
export const BUILTIN_FUNCTIONS: Record<string, BuiltinFunction> = {
  // --- string ---
  split: splitString,
  join: joinArray,
  substring: substringGo,
  substr: substrGo,
  replace: replaceFirst,
  replaceAll,
  trim,
  trimStart,
  trimEnd,
  toLowerCase,
  toUpperCase,
  lowercase: toLowerCase,
  uppercase: toUpperCase,
  lower: toLowerCase,
  upper: toUpperCase,
  charAt,
  charCodeAt,
  indexOf: (args) =>
    typeof args[0] === 'string' ? indexOfString(args) : indexOfArray(args),
  lastIndexOf: lastIndexOfString,
  includes: (args) =>
    typeof args[0] === 'string' ? includesString(args) : includesArray(args),
  startsWith,
  endsWith,
  padStart,
  padEnd,
  repeat,
  slice: (args) => (typeof args[0] === 'string' ? sliceString(args) : sliceArray(args)),
  base64Encode: (args) => base64EncodeImpl(toStringValue(args[0])),
  base64Decode: (args) => base64DecodeImpl(toStringValue(args[0])),
  urlEncode,
  urlDecode,
  htmlEncode,
  htmlDecode,
  md5: (args) => md5Impl(toStringValue(args[0])),
  sha1: (args) => sha1Impl(toStringValue(args[0])),
  sha256: (args) => sha256Impl(toStringValue(args[0])),
  sha512: (args) => sha512Impl(toStringValue(args[0])),
  isEmail,
  isUrl,
  isDomain,
  stripTags,
  extractDomain,
  extractUrl,
  toTitleCase,
  toCamelCase,
  toSnakeCase,
  toKebabCase,

  // --- math ---
  min,
  max,
  average,
  sum,
  add: sum,
  round,
  ceil,
  floor,
  abs,
  random,
  randomInt,
  subtract,
  multiply,
  divide,
  modulo,
  pow,
  sqrt,
  sin,
  cos,
  tan,

  // --- array ---
  first,
  last,
  unique,
  compact,
  flatten,
  chunk,
  pluck,
  randomItem,
  length,
  sort: sortArray,
  reverse: reverseArray,

  // --- date ---
  now,
  formatDate,
  dateFormat: formatDate,
  toDate,
  getTime: toDate,
  addDays,
  subtractDays,
  diffDays,
  addHours,
  addMinutes,

  // --- logic ---
  if: ifFunction,
  and: andFunction,
  or: orFunction,
  not: notFunction,
  isEmpty,
  isNotEmpty,
  equal,
  notEqual,
  greaterThan,
  lessThan,

  // --- object ---
  keys,
  values,
  merge,
  pick,
  omit,
  has,

  // --- utility ---
  toJson,
  fromJson,
  uuid,
  typeOf,
  randomString,
}

/* -------------------------------------------------------------------------- */
/* Member-style dispatch                                                      */
/* -------------------------------------------------------------------------- */

/**
 * Minimum argument count of every builtin.
 *
 * The Go runtime validates the arity of its functions and aborts with
 * `split requires 2 arguments` and friends. The browser preview raises
 * the same messages so a workflow that fails on the server fails in the
 * editor with the same wording.
 */
const BUILTIN_MIN_ARGS: Record<string, number> = {
  // --- string ---
  split: 2,
  join: 2,
  substring: 2,
  substr: 2,
  replace: 3,
  replaceAll: 3,
  trim: 1,
  trimStart: 1,
  trimEnd: 1,
  toLowerCase: 1,
  toUpperCase: 1,
  lowercase: 1,
  uppercase: 1,
  lower: 1,
  upper: 1,
  charAt: 2,
  charCodeAt: 2,
  indexOf: 2,
  lastIndexOf: 2,
  includes: 2,
  startsWith: 2,
  endsWith: 2,
  padStart: 2,
  padEnd: 2,
  repeat: 2,
  slice: 2,

  // --- math ---
  round: 1,
  ceil: 1,
  floor: 1,
  abs: 1,
  sqrt: 1,
  sin: 1,
  cos: 1,
  tan: 1,
  subtract: 2,
  multiply: 1,
  divide: 2,
  modulo: 2,
  pow: 2,

  // --- array ---
  first: 1,
  last: 1,
  unique: 1,
  compact: 1,
  flatten: 1,
  chunk: 2,
  pluck: 2,
  randomItem: 1,
  length: 1,
  sort: 1,
  reverse: 1,

  // --- date ---
  formatDate: 2,
  dateFormat: 2,
  toDate: 1,
  getTime: 1,
  addDays: 2,
  subtractDays: 2,
  diffDays: 2,
  addHours: 2,
  addMinutes: 2,

  // --- logic ---
  if: 3,
  not: 1,
  isEmpty: 1,
  isNotEmpty: 1,
  equal: 2,
  notEqual: 2,
  greaterThan: 2,
  lessThan: 2,

  // --- object ---
  keys: 1,
  values: 1,
  pick: 2,
  omit: 2,
  has: 2,

  // --- utility ---
  toJson: 1,
  fromJson: 1,
}

/**
 * Throw when a builtin is called with too few arguments.
 *
 * @param name - name of the builtin.
 * @param args - arguments the builtin was called with.
 * @throws {Error} with the same wording the Go runtime uses.
 */
export function assertBuiltinArity(name: string, args: unknown[]): void {
  const minimum = BUILTIN_MIN_ARGS[name]
  if (minimum === undefined || args.length >= minimum) return
  const suffix = minimum === 1 ? 'argument' : 'arguments'
  throw new Error(`${name} requires ${minimum} ${suffix}`)
}

/** Builtins that operate on a string receiver (`$json.name.split(',')`). */
const STRING_MEMBER_BUILTINS = new Set([
  'split',
  'substring',
  'substr',
  'replace',
  'replaceAll',
  'trim',
  'trimStart',
  'trimEnd',
  'toLowerCase',
  'toUpperCase',
  'lowercase',
  'uppercase',
  'lower',
  'upper',
  'charAt',
  'charCodeAt',
  'indexOf',
  'lastIndexOf',
  'includes',
  'startsWith',
  'endsWith',
  'padStart',
  'padEnd',
  'repeat',
  'slice',
  'base64Encode',
  'base64Decode',
  'urlEncode',
  'urlDecode',
  'htmlEncode',
  'htmlDecode',
  'md5',
  'sha1',
  'sha256',
  'sha512',
  'isEmail',
  'isUrl',
  'isDomain',
  'stripTags',
  'extractDomain',
  'extractUrl',
  'toTitleCase',
  'toCamelCase',
  'toSnakeCase',
  'toKebabCase',
  'formatDate',
  'dateFormat',
  'toDate',
  'getTime',
  'addDays',
  'subtractDays',
  'diffDays',
  'addHours',
  'addMinutes',
])

/** Builtins that operate on an array receiver (`$json.items.first()`). */
const ARRAY_MEMBER_BUILTINS = new Set([
  'join',
  'first',
  'last',
  'unique',
  'compact',
  'flatten',
  'chunk',
  'pluck',
  'randomItem',
  'indexOf',
  'includes',
  'slice',
  'sort',
  'reverse',
  'sum',
  'min',
  'max',
  'average',
])

/** Builtins that operate on a number receiver (`$json.price.round(2)`). */
const NUMBER_MEMBER_BUILTINS = new Set([
  'min',
  'max',
  'average',
  'sum',
  'round',
  'ceil',
  'floor',
  'abs',
  'sqrt',
  'sin',
  'cos',
  'tan',
  'subtract',
  'multiply',
  'divide',
  'modulo',
  'pow',
  'randomInt',
  'toDate',
  'getTime',
  'formatDate',
  'dateFormat',
  'addDays',
  'subtractDays',
  'diffDays',
  'addHours',
  'addMinutes',
])

/**
 * A builtin that can be called in member style, plus the number of
 * arguments it needs.
 */
export interface MemberBuiltin {
  /** The builtin implementation (value-first argument order). */
  fn: BuiltinFunction
  /** Minimum number of arguments the builtin accepts. */
  minArgs: number
}

/**
 * Look up the m9m builtin that matches a member-style call.
 *
 * The Go functions are value-first, so `$json.name.split(',')` has to
 * call `split($json.name, ',')`. The native JavaScript method is only
 * used when no builtin with that name exists.
 *
 * @param receiver - the value the member is read from.
 * @param name - the member name.
 * @returns the builtin, or `undefined` when the native method applies.
 */
export function getMemberBuiltin(receiver: unknown, name: string): MemberBuiltin | undefined {
  const builtin = Object.prototype.hasOwnProperty.call(BUILTIN_FUNCTIONS, name)
    ? BUILTIN_FUNCTIONS[name]
    : undefined
  if (!builtin) return undefined

  const applies =
    (typeof receiver === 'string' && STRING_MEMBER_BUILTINS.has(name)) ||
    (Array.isArray(receiver) && ARRAY_MEMBER_BUILTINS.has(name)) ||
    (typeof receiver === 'number' && NUMBER_MEMBER_BUILTINS.has(name))
  if (!applies) return undefined

  return { fn: builtin, minArgs: BUILTIN_MIN_ARGS[name] ?? 0 }
}

/**
 * Catalogue of the registered functions, used by the autocomplete.
 *
 * The list is intentionally restricted to the functions the server
 * registers at runtime — `filter`, `map`, `reduce` and `sleep` are
 * intentionally missing because the Go registry never installs them.
 */
export const FUNCTIONS: FunctionDefinition[] = [
  { name: 'split', category: 'string', description: 'Splits a string into an array of substrings', signature: 'split(string, separator, limit?)', example: "split($json.text, ',')" },
  { name: 'join', category: 'string', description: 'Joins array items into a string', signature: 'join(array, separator)', example: "join($json.tags, ', ')" },
  { name: 'substring', category: 'string', description: 'Extracts a substring between two indexes', signature: 'substring(string, start, end?)', example: 'substring($json.name, 0, 3)' },
  { name: 'substr', category: 'string', description: 'Extracts a substring by start offset and length', signature: 'substr(string, start, length?)', example: 'substr($json.name, 0, 3)' },
  { name: 'replace', category: 'string', description: 'Replaces the first occurrence of a substring', signature: 'replace(string, search, replacement)', example: "replace($json.name, 'a', 'b')" },
  { name: 'replaceAll', category: 'string', description: 'Replaces every occurrence of a substring', signature: 'replaceAll(string, search, replacement)', example: "replaceAll($json.name, 'a', 'b')" },
  { name: 'trim', category: 'string', description: 'Removes whitespace from both ends', signature: 'trim(string)', example: 'trim($json.name)' },
  { name: 'trimStart', category: 'string', description: 'Removes leading whitespace', signature: 'trimStart(string)', example: 'trimStart($json.name)' },
  { name: 'trimEnd', category: 'string', description: 'Removes trailing whitespace', signature: 'trimEnd(string)', example: 'trimEnd($json.name)' },
  { name: 'toLowerCase', category: 'string', description: 'Lowercases a string', signature: 'toLowerCase(string)', example: 'toLowerCase($json.name)' },
  { name: 'toUpperCase', category: 'string', description: 'Uppercases a string', signature: 'toUpperCase(string)', example: 'toUpperCase($json.name)' },
  { name: 'lowercase', category: 'string', description: 'Alias of toLowerCase', signature: 'lowercase(string)', example: 'lowercase($json.name)' },
  { name: 'uppercase', category: 'string', description: 'Alias of toUpperCase', signature: 'uppercase(string)', example: 'uppercase($json.name)' },
  { name: 'lower', category: 'string', description: 'Alias of toLowerCase', signature: 'lower(string)', example: 'lower($json.name)' },
  { name: 'upper', category: 'string', description: 'Alias of toUpperCase', signature: 'upper(string)', example: 'upper($json.name)' },
  { name: 'charAt', category: 'string', description: 'Returns the character at an index', signature: 'charAt(string, index)', example: 'charAt($json.code, 0)' },
  { name: 'charCodeAt', category: 'string', description: 'Returns the character code at an index', signature: 'charCodeAt(string, index)', example: 'charCodeAt($json.code, 0)' },
  { name: 'indexOf', category: 'string', description: 'Index of a substring or array item', signature: 'indexOf(string|array, search)', example: "indexOf($json.tags, 'a')" },
  { name: 'lastIndexOf', category: 'string', description: 'Last index of a substring', signature: 'lastIndexOf(string, search)', example: "lastIndexOf($json.text, 'a')" },
  { name: 'includes', category: 'string', description: 'Whether a string or array contains a value', signature: 'includes(string|array, search)', example: "includes($json.tags, 'a')" },
  { name: 'startsWith', category: 'string', description: 'Whether a string starts with a prefix', signature: 'startsWith(string, prefix)', example: "startsWith($json.url, 'https')" },
  { name: 'endsWith', category: 'string', description: 'Whether a string ends with a suffix', signature: 'endsWith(string, suffix)', example: "endsWith($json.file, '.csv')" },
  { name: 'padStart', category: 'string', description: 'Pads a string on the left', signature: 'padStart(string, length, pad?)', example: "padStart($json.id, 4, '0')" },
  { name: 'padEnd', category: 'string', description: 'Pads a string on the right', signature: 'padEnd(string, length, pad?)', example: "padEnd($json.id, 4, '0')" },
  { name: 'repeat', category: 'string', description: 'Repeats a string', signature: 'repeat(string, count)', example: 'repeat($json.sep, 3)' },
  { name: 'slice', category: 'string', description: 'Slices a string or array', signature: 'slice(string|array, start, end?)', example: 'slice($json.items, 0, 2)' },
  { name: 'base64Encode', category: 'string', description: 'Base64-encodes a string', signature: 'base64Encode(string)', example: 'base64Encode($json.data)' },
  { name: 'base64Decode', category: 'string', description: 'Decodes a Base64 string', signature: 'base64Decode(string)', example: 'base64Decode($json.data)' },
  { name: 'urlEncode', category: 'string', description: 'Percent-encodes a string', signature: 'urlEncode(string)', example: 'urlEncode($json.query)' },
  { name: 'urlDecode', category: 'string', description: 'Decodes a percent-encoded string', signature: 'urlDecode(string)', example: 'urlDecode($json.query)' },
  { name: 'htmlEncode', category: 'string', description: 'Escapes HTML entities', signature: 'htmlEncode(string)', example: 'htmlEncode($json.body)' },
  { name: 'htmlDecode', category: 'string', description: 'Decodes HTML entities', signature: 'htmlDecode(string)', example: 'htmlDecode($json.body)' },
  { name: 'md5', category: 'string', description: 'MD5 hash of a string', signature: 'md5(string)', example: 'md5($json.password)' },
  { name: 'sha1', category: 'string', description: 'SHA-1 hash of a string', signature: 'sha1(string)', example: 'sha1($json.password)' },
  { name: 'sha256', category: 'string', description: 'SHA-256 hash of a string', signature: 'sha256(string)', example: 'sha256($json.password)' },
  { name: 'sha512', category: 'string', description: 'SHA-512 hash of a string', signature: 'sha512(string)', example: 'sha512($json.password)' },
  { name: 'isEmail', category: 'string', description: 'Whether a string is an e-mail address', signature: 'isEmail(string)', example: 'isEmail($json.email)' },
  { name: 'isUrl', category: 'string', description: 'Whether a string is a valid URL', signature: 'isUrl(string)', example: 'isUrl($json.url)' },
  { name: 'isDomain', category: 'string', description: 'Whether a string is a valid domain', signature: 'isDomain(string)', example: 'isDomain($json.host)' },
  { name: 'stripTags', category: 'string', description: 'Removes HTML tags', signature: 'stripTags(string)', example: 'stripTags($json.body)' },
  { name: 'extractDomain', category: 'string', description: 'Extracts the domain from a URL', signature: 'extractDomain(string)', example: 'extractDomain($json.url)' },
  { name: 'extractUrl', category: 'string', description: 'Extracts every URL from a string', signature: 'extractUrl(string)', example: 'extractUrl($json.body)' },
  { name: 'toTitleCase', category: 'string', description: 'Converts a string to Title Case', signature: 'toTitleCase(string)', example: 'toTitleCase($json.name)' },
  { name: 'toCamelCase', category: 'string', description: 'Converts a string to camelCase', signature: 'toCamelCase(string)', example: 'toCamelCase($json.label)' },
  { name: 'toSnakeCase', category: 'string', description: 'Converts a string to snake_case', signature: 'toSnakeCase(string)', example: 'toSnakeCase($json.label)' },
  { name: 'toKebabCase', category: 'string', description: 'Converts a string to kebab-case', signature: 'toKebabCase(string)', example: 'toKebabCase($json.label)' },

  { name: 'min', category: 'math', description: 'Smallest of the given numbers', signature: 'min(number, ...)', example: 'min($json.values)' },
  { name: 'max', category: 'math', description: 'Largest of the given numbers', signature: 'max(number, ...)', example: 'max($json.values)' },
  { name: 'average', category: 'math', description: 'Arithmetic mean of the given numbers', signature: 'average(number, ...)', example: 'average($json.values)' },
  { name: 'sum', category: 'math', description: 'Sum of the given numbers', signature: 'sum(number, ...)', example: 'sum($json.values)' },
  { name: 'add', category: 'math', description: 'Alias of sum', signature: 'add(number, ...)', example: 'add($json.a, $json.b)' },
  { name: 'round', category: 'math', description: 'Rounds a number to a precision', signature: 'round(number, precision?)', example: 'round($json.price, 2)' },
  { name: 'ceil', category: 'math', description: 'Rounds a number up', signature: 'ceil(number)', example: 'ceil($json.price)' },
  { name: 'floor', category: 'math', description: 'Rounds a number down', signature: 'floor(number)', example: 'floor($json.price)' },
  { name: 'abs', category: 'math', description: 'Absolute value', signature: 'abs(number)', example: 'abs($json.delta)' },
  { name: 'random', category: 'math', description: 'Random number between 0 and 1', signature: 'random()', example: 'random()' },
  { name: 'randomInt', category: 'math', description: 'Random integer in a range', signature: 'randomInt(max) | randomInt(min, max)', example: 'randomInt(1, 6)' },
  { name: 'subtract', category: 'math', description: 'Subtracts two numbers', signature: 'subtract(number, number)', example: 'subtract($json.total, $json.discount)' },
  { name: 'multiply', category: 'math', description: 'Multiplies numbers', signature: 'multiply(number, ...)', example: 'multiply($json.price, $json.qty)' },
  { name: 'divide', category: 'math', description: 'Divides two numbers', signature: 'divide(number, number)', example: 'divide($json.total, $json.count)' },
  { name: 'modulo', category: 'math', description: 'Remainder of a division', signature: 'modulo(number, number)', example: 'modulo($json.count, 2)' },
  { name: 'pow', category: 'math', description: 'Raises a number to a power', signature: 'pow(number, number)', example: 'pow($json.base, 2)' },
  { name: 'sqrt', category: 'math', description: 'Square root', signature: 'sqrt(number)', example: 'sqrt($json.area)' },
  { name: 'sin', category: 'math', description: 'Sine', signature: 'sin(number)', example: 'sin($json.angle)' },
  { name: 'cos', category: 'math', description: 'Cosine', signature: 'cos(number)', example: 'cos($json.angle)' },
  { name: 'tan', category: 'math', description: 'Tangent', signature: 'tan(number)', example: 'tan($json.angle)' },

  { name: 'first', category: 'array', description: 'First element of an array', signature: 'first(array)', example: 'first($json.items)' },
  { name: 'last', category: 'array', description: 'Last element of an array', signature: 'last(array)', example: 'last($json.items)' },
  { name: 'unique', category: 'array', description: 'Removes duplicate array entries', signature: 'unique(array)', example: 'unique($json.tags)' },
  { name: 'compact', category: 'array', description: 'Removes falsy array entries', signature: 'compact(array)', example: 'compact($json.items)' },
  { name: 'flatten', category: 'array', description: 'Flattens nested arrays', signature: 'flatten(array, depth?)', example: 'flatten($json.groups)' },
  { name: 'chunk', category: 'array', description: 'Splits an array into chunks', signature: 'chunk(array, size)', example: 'chunk($json.items, 10)' },
  { name: 'pluck', category: 'array', description: 'Collects one property from every item', signature: 'pluck(array, property)', example: "pluck($json.items, 'id')" },
  { name: 'randomItem', category: 'array', description: 'Random element of an array', signature: 'randomItem(array)', example: 'randomItem($json.items)' },
  { name: 'length', category: 'array', description: 'Length of an array or string', signature: 'length(array|string)', example: 'length($json.items)' },
  { name: 'sort', category: 'array', description: 'Sorts an array', signature: 'sort(array)', example: 'sort($json.items)' },
  { name: 'reverse', category: 'array', description: 'Reverses an array', signature: 'reverse(array)', example: 'reverse($json.items)' },

  { name: 'now', category: 'date', description: 'Current timestamp in milliseconds', signature: 'now()', example: 'now()' },
  { name: 'formatDate', category: 'date', description: 'Formats a date (yyyy-MM-dd HH:mm:ss)', signature: 'formatDate(date, format)', example: "formatDate($json.date, 'yyyy-MM-dd')" },
  { name: 'dateFormat', category: 'date', description: 'Alias of formatDate', signature: 'dateFormat(date, format)', example: "dateFormat($json.date, 'yyyy-MM-dd')" },
  { name: 'toDate', category: 'date', description: 'Converts a value to a timestamp', signature: 'toDate(string|number)', example: 'toDate($json.date)' },
  { name: 'getTime', category: 'date', description: 'Timestamp of a date value', signature: 'getTime(string|number)', example: 'getTime($json.date)' },
  { name: 'addDays', category: 'date', description: 'Adds days to a timestamp', signature: 'addDays(timestamp, days)', example: 'addDays($json.date, 7)' },
  { name: 'subtractDays', category: 'date', description: 'Subtracts days from a timestamp', signature: 'subtractDays(timestamp, days)', example: 'subtractDays($json.date, 7)' },
  { name: 'diffDays', category: 'date', description: 'Difference between two timestamps in days', signature: 'diffDays(timestamp, timestamp)', example: 'diffDays($json.from, $json.to)' },
  { name: 'addHours', category: 'date', description: 'Adds hours to a timestamp', signature: 'addHours(timestamp, hours)', example: 'addHours($json.date, 2)' },
  { name: 'addMinutes', category: 'date', description: 'Adds minutes to a timestamp', signature: 'addMinutes(timestamp, minutes)', example: 'addMinutes($json.date, 30)' },

  { name: 'if', category: 'logic', description: 'Returns one of two values depending on a condition', signature: 'if(condition, trueValue, falseValue)', example: "if($json.age > 18, 'adult', 'minor')" },
  { name: 'and', category: 'logic', description: 'True when every argument is truthy', signature: 'and(value, ...)', example: 'and($json.a, $json.b)' },
  { name: 'or', category: 'logic', description: 'True when any argument is truthy', signature: 'or(value, ...)', example: 'or($json.a, $json.b)' },
  { name: 'not', category: 'logic', description: 'Negates a value', signature: 'not(value)', example: 'not($json.flag)' },
  { name: 'isEmpty', category: 'logic', description: 'Whether a value is empty', signature: 'isEmpty(value)', example: 'isEmpty($json.name)' },
  { name: 'isNotEmpty', category: 'logic', description: 'Whether a value is not empty', signature: 'isNotEmpty(value)', example: 'isNotEmpty($json.name)' },
  { name: 'equal', category: 'logic', description: 'Whether two values are equal', signature: 'equal(value, value)', example: 'equal($json.a, $json.b)' },
  { name: 'notEqual', category: 'logic', description: 'Whether two values differ', signature: 'notEqual(value, value)', example: 'notEqual($json.a, $json.b)' },
  { name: 'greaterThan', category: 'logic', description: 'Whether the first number is greater', signature: 'greaterThan(number, number)', example: 'greaterThan($json.count, 10)' },
  { name: 'lessThan', category: 'logic', description: 'Whether the first number is smaller', signature: 'lessThan(number, number)', example: 'lessThan($json.count, 10)' },

  { name: 'keys', category: 'object', description: 'Keys of an object', signature: 'keys(object)', example: 'keys($json.data)' },
  { name: 'values', category: 'object', description: 'Values of an object', signature: 'values(object)', example: 'values($json.data)' },
  { name: 'merge', category: 'object', description: 'Merges objects', signature: 'merge(object, ...)', example: 'merge($json.a, $json.b)' },
  { name: 'pick', category: 'object', description: 'Keeps only the given keys', signature: 'pick(object, key|keys)', example: "pick($json.data, 'id')" },
  { name: 'omit', category: 'object', description: 'Removes the given keys', signature: 'omit(object, key|keys)', example: "omit($json.data, 'secret')" },
  { name: 'has', category: 'object', description: 'Whether an object has a key', signature: 'has(object, key)', example: "has($json.data, 'id')" },

  { name: 'toJson', category: 'utility', description: 'Serialises a value to JSON', signature: 'toJson(value)', example: 'toJson($json.data)' },
  { name: 'fromJson', category: 'utility', description: 'Parses a JSON string', signature: 'fromJson(string)', example: 'fromJson($json.payload)' },
  { name: 'uuid', category: 'utility', description: 'Random UUID v4', signature: 'uuid()', example: 'uuid()' },
  { name: 'typeOf', category: 'utility', description: 'Runtime type of a value', signature: 'typeOf(value)', example: 'typeOf($json.data)' },
  { name: 'randomString', category: 'utility', description: 'Random alphanumeric string', signature: 'randomString(length?)', example: 'randomString(12)' },
]

/** Look up a registered function by name. */
export function getBuiltinFunction(name: string): BuiltinFunction | undefined {
  return Object.prototype.hasOwnProperty.call(BUILTIN_FUNCTIONS, name)
    ? BUILTIN_FUNCTIONS[name]
    : undefined
}
