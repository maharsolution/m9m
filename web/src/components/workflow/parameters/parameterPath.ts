/**
 * Parameter path helpers.
 *
 * m9m's node metadata uses dotted parameter names the same way n8n does
 * (`options.caseSensitive`, `conditions.conditions`, `rules.values`). n8n
 * resolves such a name as a *path* into the parameter object, so the value
 * lives at `parameters.options.caseSensitive` — not at a flat key literally
 * called `"options.caseSensitive"`.
 *
 * These helpers read/write the nested shape while still understanding the
 * flat dotted keys older m9m builds wrote, so a workflow saved by an older
 * editor keeps rendering until the user edits the field.
 */

/** Split a parameter name into its path segments. */
export function parameterPath(name: string): string[] {
  return name.split('.').filter((segment) => segment.length > 0)
}

/**
 * Read a parameter value by path. Falls back to the legacy flat key when
 * the nested object does not carry the value.
 */
export function getParameterValue(
  params: Record<string, unknown>,
  name: string,
): unknown {
  const segments = parameterPath(name)
  if (segments.length === 0) return undefined

  let current: unknown = params
  for (const segment of segments) {
    if (current === null || typeof current !== 'object') {
      current = undefined
      break
    }
    current = (current as Record<string, unknown>)[segment]
  }
  if (current !== undefined) return current

  // Legacy: a flat key literally named "a.b".
  return Object.prototype.hasOwnProperty.call(params, name) ? params[name] : undefined
}

/**
 * Write a parameter value by path, creating intermediate objects. The
 * legacy flat key (if any) is removed so both shapes never disagree.
 */
export function setParameterValue(
  params: Record<string, unknown>,
  name: string,
  value: unknown,
): Record<string, unknown> {
  const next: Record<string, unknown> = { ...params }
  const segments = parameterPath(name)
  if (segments.length === 0) return next

  if (segments.length === 1) {
    next[segments[0]] = value
  } else {
    const root: Record<string, unknown> = { ...next }
    let cursor: Record<string, unknown> = root
    for (let i = 0; i < segments.length - 1; i += 1) {
      const segment = segments[i]
      const child = cursor[segment]
      cursor[segment] =
        child !== null && typeof child === 'object' && !Array.isArray(child)
          ? { ...(child as Record<string, unknown>) }
          : {}
      cursor = cursor[segment] as Record<string, unknown>
    }
    cursor[segments[segments.length - 1]] = value
    Object.assign(next, root)
  }

  if (segments.length > 1 && Object.prototype.hasOwnProperty.call(next, name)) {
    delete next[name]
  }
  return next
}

/** Remove a parameter value by path (nested and legacy flat key). */
export function deleteParameterValue(
  params: Record<string, unknown>,
  name: string,
): Record<string, unknown> {
  const next: Record<string, unknown> = { ...params }
  const segments = parameterPath(name)
  if (segments.length === 0) return next

  if (segments.length > 1) {
    const root: Record<string, unknown> = { ...next }
    let cursor: Record<string, unknown> = root
    const trail: string[] = []
    for (let i = 0; i < segments.length - 1; i += 1) {
      const segment = segments[i]
      const child = cursor[segment]
      if (child === null || typeof child !== 'object' || Array.isArray(child)) {
        return next
      }
      trail.push(segment)
      cursor = child as Record<string, unknown>
    }
    const last = segments[segments.length - 1]
    if (Object.prototype.hasOwnProperty.call(cursor, last)) {
      delete cursor[last]
    }
    // Drop containers that became empty so the tree stays tidy.
    for (let i = trail.length - 1; i >= 0; i -= 1) {
      const segment = trail[i]
      const child = next[segment]
      if (
        child !== null &&
        typeof child === 'object' &&
        !Array.isArray(child) &&
        Object.keys(child as Record<string, unknown>).length === 0
      ) {
        delete next[segment]
      } else {
        break
      }
    }
  }

  if (Object.prototype.hasOwnProperty.call(next, name)) {
    delete next[name]
  }
  return next
}

/** True when the value looks like an m9m/n8n expression (`={{ … }}` or `{{ … }}`). */
export function isExpressionValue(value: unknown): boolean {
  if (typeof value !== 'string') return false
  return value.startsWith('=') || (value.includes('{{') && value.includes('}}'))
}
