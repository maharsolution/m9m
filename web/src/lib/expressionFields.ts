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

/** The completion sources of the expression editor. */
export interface ExpressionFieldSources {
  /** Field names of the current item (`$json.<field>`). */
  itemFields: string[]
  /** Field names per node name (`$node["Name"].json.<field>`). */
  nodeFields: Record<string, string[]>
}

/**
 * Collects the top level field names of a JSON payload.
 *
 * Non-objects (an array, a string, `undefined`) have no fields.
 */
export function collectJsonFields(value: unknown): string[] {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return []
  }

  return Object.keys(value as Record<string, unknown>)
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

  Object.entries(outputs).forEach(([name, payload]) => {
    nodeFields[name] = collectJsonFields(payload)
  })

  return {
    itemFields: collectJsonFields(input),
    nodeFields,
  }
}
