import type { NodeProperty, NodePropertyDisplayOptions } from '@/types/node'
import { getParameterValue, isExpressionValue } from './parameterPath'

/**
 * n8n-style `displayOptions` evaluation.
 *
 * Rules (mirroring n8n's `displayParameter`):
 *
 *  - A property without `displayOptions` is always visible.
 *  - `show`: every listed controlling parameter must match one of the
 *    listed values — all `show` rules are AND-ed. When a controlling value
 *    is an expression (`={{ … }}`) the rule is treated as satisfied
 *    (n8n cannot evaluate it at edit time, so the field stays visible).
 *  - `hide`: any listed controlling parameter matching one of its values
 *    hides the field — `hide` rules are OR-ed. A controlling parameter
 *    that has no value never hides anything.
 *
 * The controlling value is read from the current parameter map; when the
 * parameter has not been set yet the referenced property's `default` is
 * used, so a freshly added node renders the same fields n8n shows.
 *
 * Multi-value controlling parameters (`multiOptions`, collections) are
 * treated as a set: the rule matches when *any* of the current values is
 * listed.
 */

/** Loose value comparison — option values are strings, parameters may be numbers/booleans. */
function valuesMatch(actual: unknown, expected: unknown): boolean {
  if (actual === expected) return true
  if (actual === null || actual === undefined) return false
  if (expected === null || expected === undefined) return false
  if (typeof actual === 'object' || typeof expected === 'object') return false
  return String(actual) === String(expected)
}

/** Flatten a controlling value into the list of values to test. */
function toValueList(value: unknown): unknown[] {
  if (Array.isArray(value)) return value.flatMap((entry) => toValueList(entry))
  return [value]
}

/** Resolve the value of a controlling parameter (current value, else its default). */
function resolveControllingValue(
  name: string,
  parameters: Record<string, unknown>,
  properties: NodeProperty[],
): unknown {
  const current = getParameterValue(parameters, name)
  if (current !== undefined) return current
  const property = properties.find((p) => p.name === name)
  return property?.default
}

export function evaluateDisplayOptions(
  displayOptions: NodePropertyDisplayOptions | undefined,
  parameters: Record<string, unknown>,
  properties: NodeProperty[] = [],
): boolean {
  if (!displayOptions) return true

  const show = displayOptions.show ?? {}
  for (const [name, allowed] of Object.entries(show)) {
    const value = resolveControllingValue(name, parameters, properties)
    const candidates = toValueList(value)
    // An expression cannot be evaluated in the editor — assume the
    // condition holds so the field does not disappear at edit time.
    if (candidates.some((candidate) => isExpressionValue(candidate))) continue
    const allowedValues = Array.isArray(allowed) ? allowed : [allowed]
    const matched = candidates.some((candidate) =>
      allowedValues.some((expected) => valuesMatch(candidate, expected)),
    )
    if (!matched) return false
  }

  const hide = displayOptions.hide ?? {}
  for (const [name, allowed] of Object.entries(hide)) {
    const value = resolveControllingValue(name, parameters, properties)
    const candidates = toValueList(value)
    if (candidates.length === 0) continue
    if (candidates.every((candidate) => candidate === undefined || candidate === null || candidate === '')) {
      continue
    }
    const allowedValues = Array.isArray(allowed) ? allowed : [allowed]
    const matched = candidates.some((candidate) =>
      allowedValues.some((expected) => valuesMatch(candidate, expected)),
    )
    if (matched) return false
  }

  return true
}
