/**
 * The data the expression editor offers as completions.
 *
 * n8n knows the payload of the current item and the output of every
 * other node of the workflow, so it can complete `$json.<field>` and
 * `$node["Name"].json.<field>`. m9m knows the same data after the
 * workflow ran at least once — the node details view shows it in its
 * Input / Output columns.
 *
 * The helpers are pure: they only look at the JSON-ish values the
 * execution API returned.
 */

/**
 * One level of the nested field tree of a payload.
 *
 * The tree mirrors the shape of the payload: `fields` holds the keys
 * of the level, `children` holds the sub-tree of every key. The
 * autocomplete walks it, so `$json.address.city` resolves to the keys
 * of `city`.
 */
export interface FieldTree {
  /** The field names of this level. */
  fields: string[]
  /** The sub-tree of every field, keyed by the field name. */
  children: Record<string, FieldTree>
}

/** The completion sources of the expression editor. */
export interface ExpressionFieldSources {
  /** Field names of the current item (`$json.<field>`). */
  itemFields: string[]
  /** Field names per node name (`$node["Name"].json.<field>`). */
  nodeFields: Record<string, string[]>
  /** The nested field tree of the current item (`$json.address.city`). */
  itemTree: FieldTree
  /** The nested field tree per node name (`$node["Name"].json.address`). */
  nodeTrees: Record<string, FieldTree>
}

/**
 * How deep the field tree is built.
 *
 * A path that is deeper than this resolves to nothing — the editor
 * never guesses.
 */
export const MAX_FIELD_DEPTH = 5

/**
 * Collects the top level field names of a JSON payload.
 *
 * Non-objects (an array, a string, `undefined`) have no fields.
 */
export function collectJsonFields(value: unknown): string[] {
  if (!isPlainObject(value)) {
    return []
  }

  return Object.keys(value as Record<string, unknown>)
}

/**
 * Collects the nested field names of a JSON payload.
 *
 * Every object level becomes a {@link FieldTree} node; the tree stops
 * after {@link MAX_FIELD_DEPTH} levels. An array is resolved through
 * its object elements — the keys of the array are the keys of the
 * objects it holds.
 *
 * @param value - the JSON payload.
 * @param depth - the current level (0 is the payload itself).
 */
export function collectJsonFieldTree(value: unknown, depth = 0): FieldTree {
  const fields: string[] = []
  const children: Record<string, FieldTree> = {}

  if (depth >= MAX_FIELD_DEPTH) {
    return { fields, children }
  }

  for (const [key, child] of payloadEntries(value)) {
    if (!fields.includes(key)) {
      fields.push(key)
    }
    children[key] = collectJsonFieldTree(child, depth + 1)
  }

  return { fields, children }
}

/**
 * Builds the completion sources from the data the editor knows.
 *
 * @param input - the JSON payload of the current item.
 * @param outputs - the JSON payload of every other node, by node name.
 */
export function buildExpressionFieldSources(
  input: unknown,
  outputs: Record<string, unknown> = {},
): ExpressionFieldSources {
  const nodeFields: Record<string, string[]> = {}
  const nodeTrees: Record<string, FieldTree> = {}

  Object.entries(outputs).forEach(([name, payload]) => {
    nodeFields[name] = collectJsonFields(payload)
    nodeTrees[name] = collectJsonFieldTree(payload)
  })

  return {
    itemFields: collectJsonFields(input),
    nodeFields,
    itemTree: collectJsonFieldTree(input),
    nodeTrees,
  }
}

/** Whether a value is a plain JSON object. */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

/**
 * The entries of a payload.
 *
 * An array contributes the entries of the objects it holds — the first
 * occurrence of a key wins, so the order stays deterministic.
 */
function payloadEntries(value: unknown): [string, unknown][] {
  if (isPlainObject(value)) {
    return Object.entries(value)
  }

  if (!Array.isArray(value)) {
    return []
  }

  const entries: [string, unknown][] = []
  const seen = new Set<string>()

  for (const element of value) {
    if (!isPlainObject(element)) continue
    for (const [key, child] of Object.entries(element)) {
      if (seen.has(key)) continue
      seen.add(key)
      entries.push([key, child])
    }
  }

  return entries
}
