/**
 * The autocomplete engine of the expression editor.
 *
 * n8n opens a suggestion list as soon as the user types `{{` and
 * keeps it open while the expression is being written:
 *
 *   `{{`              → the variables, the nodes and the functions
 *   `{{$js`           → the same list, filtered
 *   `{{$json.`        → the fields of the current item
 *   `{{$node["Set"].` → the fields the node "Set" produced
 *
 * The engine is pure: it reads the text and the caret, and returns
 * the suggestions. Everything it needs (the catalogue, the fields of
 * the items) is handed in, so the logic can be unit tested without
 * a DOM.
 */

import type { FunctionDefinition } from './functions'
import type { AvailableNode, VariableDefinition } from './variables'

/** Kind of an autocomplete suggestion. */
export type SuggestionKind = 'variable' | 'node' | 'function' | 'field'

/** One autocomplete suggestion. */
export interface Suggestion {
  /** What kind of entry this is. */
  kind: SuggestionKind
  /** Text shown in the dropdown. */
  label: string
  /** Secondary text shown next to the label. */
  detail: string
  /** Text inserted into the editor when the entry is selected. */
  insertText: string
}

/** Maximum number of suggestions returned for one position. */
export const MAX_SUGGESTIONS = 25

/** Where the caret sits inside an expression. */
export interface ExpressionPosition {
  /** True when the caret is inside an expression. */
  inExpression: boolean
  /** The token the list is filtered with (e.g. `$js`). */
  prefix: string
  /** Number of characters at the caret that a suggestion replaces. */
  replaceLength: number
  /** The variable the path starts with (`$json`, `$node`, …). */
  root: string | null
  /** The node name of a `$node["Name"]` reference. */
  nodeName: string | null
  /** The segments after the root (`json`, `item`, …). */
  segments: string[]
  /** The name typed inside `$node["…"]` while it is still open. */
  nodeNamePrefix: string | null
  /** True when the caret sits directly behind a dot. */
  afterDot: boolean
}

/** Everything the engine needs to build the suggestion list. */
export interface SuggestionSources {
  /** The context variables (`$json`, `$now`, …). */
  variables: VariableDefinition[]
  /** The nodes of the workflow (`$node["Name"]`). */
  nodes: AvailableNode[]
  /** The functions of the expression engine. */
  functions: FunctionDefinition[]
  /** The fields of the current item (`$json.<field>`). */
  itemFields: string[]
  /** The fields per node name (`$node["Name"].json.<field>`). */
  nodeFields: Record<string, string[]>
}

/** A position that is not inside an expression. */
const OUTSIDE_EXPRESSION: ExpressionPosition = {
  inExpression: false,
  prefix: '',
  replaceLength: 0,
  root: null,
  nodeName: null,
  segments: [],
  nodeNamePrefix: null,
  afterDot: false,
}

/** The identifier token at the end of a string. */
const TOKEN_PATTERN = /([$A-Za-z_][$A-Za-z0-9_]*)$/

/** A node reference whose name is still being typed. */
const OPEN_NODE_PATTERN = /\$(?:node)?\["?([^"\]]*)$/

/** A node reference in the `$("Name")` shorthand. */
const OPEN_SHORTHAND_PATTERN = /\$\("([^"]*)$/

/**
 * Detects where the caret sits inside an expression.
 *
 * @param text - the full editor content.
 * @param caret - the caret offset inside `text`.
 * @param options - `insideExpression` is set by the expression mode,
 *   where the whole content is the expression body.
 */
export function detectExpressionPosition(
  text: string,
  caret: number,
  options: { insideExpression?: boolean } = {},
): ExpressionPosition {
  const before = text.slice(0, Math.max(0, caret))

  let body: string
  if (options.insideExpression) {
    body = before
  } else {
    const open = before.lastIndexOf('{{')
    if (open === -1) {
      return { ...OUTSIDE_EXPRESSION }
    }
    // The expression was closed before the caret — the caret is in
    // the plain text around it.
    if (before.indexOf('}}', open + 2) !== -1) {
      return { ...OUTSIDE_EXPRESSION }
    }
    body = before.slice(open + 2)
  }

  return parseBody(body)
}

/** Parses the expression body in front of the caret. */
function parseBody(body: string): ExpressionPosition {
  // A node reference whose name is still being typed:
  // `$node["Set` or `$("Set`.
  const openNode = body.match(OPEN_NODE_PATTERN)
  if (openNode) {
    return {
      inExpression: true,
      prefix: openNode[1] ?? '',
      replaceLength: openNode[0].length,
      root: '$node',
      nodeName: null,
      segments: [],
      nodeNamePrefix: openNode[1] ?? '',
      afterDot: false,
    }
  }

  const openShorthand = body.match(OPEN_SHORTHAND_PATTERN)
  if (openShorthand) {
    return {
      inExpression: true,
      prefix: openShorthand[1] ?? '',
      replaceLength: openShorthand[0].length,
      root: '$',
      nodeName: null,
      segments: [],
      nodeNamePrefix: openShorthand[1] ?? '',
      afterDot: false,
    }
  }

  const endsWithDot = body.endsWith('.')
  const core = endsWithDot ? body.slice(0, -1) : body

  let prefix = ''
  let replaceLength = 0
  if (!endsWithDot) {
    const token = core.match(TOKEN_PATTERN)
    if (token) {
      prefix = token[1]
      replaceLength = token[1].length
    }
  }

  const pathCore = replaceLength > 0 ? core.slice(0, core.length - replaceLength) : core
  const parsed = parsePath(pathCore)

  return {
    inExpression: true,
    prefix,
    replaceLength,
    root: parsed.root,
    nodeName: parsed.nodeName,
    segments: parsed.segments,
    nodeNamePrefix: null,
    afterDot: endsWithDot,
  }
}

/** Splits the path in front of the caret into root, node and segments. */
function parsePath(path: string): {
  root: string | null
  nodeName: string | null
  segments: string[]
} {
  const trimmed = path.trim()
  if (trimmed === '') {
    return { root: null, nodeName: null, segments: [] }
  }

  const nodeRef = trimmed.match(/^\$node\["([^"]+)"\](?:\.(.*))?$/)
  if (nodeRef) {
    return {
      root: '$node',
      nodeName: nodeRef[1],
      segments: splitSegments(nodeRef[2] ?? ''),
    }
  }

  const shorthand = trimmed.match(/^\$\("([^"]+)"\)(?:\.(.*))?$/)
  if (shorthand) {
    return {
      root: '$',
      nodeName: shorthand[1],
      segments: splitSegments(shorthand[2] ?? ''),
    }
  }

  const parts = splitSegments(trimmed)
  if (parts.length === 0) {
    return { root: null, nodeName: null, segments: [] }
  }

  return { root: parts[0], nodeName: null, segments: parts.slice(1) }
}

/** Splits a dotted path into its segments. */
function splitSegments(path: string): string[] {
  return path
    .split('.')
    .map((segment) => segment.trim())
    .filter((segment) => segment !== '')
}

/**
 * Whether a label matches the needle.
 *
 * A prefix match wins; a needle that starts with `$` also matches a
 * label whose non-word characters are skipped, so `$nod` matches
 * `$node["Set"]`.
 */
export function matchesLabel(label: string, needle: string): boolean {
  const haystack = label.toLowerCase()
  const lowerNeedle = needle.toLowerCase()
  if (lowerNeedle === '') return true
  if (haystack.startsWith(lowerNeedle)) return true
  if (lowerNeedle.startsWith('$')) {
    return haystack.replace(/[^a-z]/g, '').startsWith(lowerNeedle.replace(/[^a-z]/g, ''))
  }
  return haystack.includes(lowerNeedle)
}

/** Filters a list of suggestions by the needle. */
export function filterSuggestions(
  suggestions: Suggestion[],
  needle: string,
): Suggestion[] {
  if (needle === '') {
    return suggestions.slice(0, MAX_SUGGESTIONS)
  }

  return suggestions
    .filter((suggestion) => matchesLabel(suggestion.label, needle))
    .slice(0, MAX_SUGGESTIONS)
}

/**
 * Builds the suggestion list for a caret position.
 *
 * The list depends on what the user typed:
 *
 *   * right after `{{` — the variables, the nodes and the functions,
 *   * a variable name — the same list, filtered,
 *   * a node name — the nodes whose name matches,
 *   * behind a dot — the fields of the item or of the node.
 */
export function suggestForPosition(
  position: ExpressionPosition,
  sources: SuggestionSources,
): Suggestion[] {
  if (!position.inExpression) {
    return []
  }

  if (position.nodeNamePrefix !== null) {
    return filterSuggestions(nodeSuggestions(sources.nodes), position.nodeNamePrefix)
  }

  if (position.afterDot) {
    return fieldSuggestions(position, sources)
  }

  const root = filterSuggestions(rootSuggestions(sources), position.prefix)

  if (position.prefix !== '' && !position.prefix.startsWith('$')) {
    return filterSuggestions(
      [...root, ...functionSuggestions(sources.functions)],
      position.prefix,
    )
  }

  return root
}

/** The variables, the nodes and the functions — the root list. */
function rootSuggestions(sources: SuggestionSources): Suggestion[] {
  return [
    ...sources.variables.map(variableSuggestion),
    ...nodeSuggestions(sources.nodes),
  ]
}

/** The `$node["Name"]` entries of the given nodes. */
function nodeSuggestions(nodes: AvailableNode[]): Suggestion[] {
  return nodes.map((node) => ({
    kind: 'node' as const,
    label: `$node["${node.name}"]`,
    detail: node.type,
    insertText: `$node["${node.name}"].json`,
  }))
}

/** The suggestion entry of a context variable. */
function variableSuggestion(variable: VariableDefinition): Suggestion {
  return {
    kind: 'variable',
    label: variable.name,
    detail: variable.description,
    insertText: variable.name,
  }
}

/** The suggestion entry of a function. */
function functionSuggestion(fn: FunctionDefinition): Suggestion {
  return {
    kind: 'function',
    label: fn.name,
    detail: `${fn.signature} — ${fn.description}`,
    insertText: `${fn.name}(`,
  }
}

/** The functions of the engine. */
function functionSuggestions(functions: FunctionDefinition[]): Suggestion[] {
  return functions.map(functionSuggestion)
}

/**
 * The field suggestions for a position behind a dot.
 *
 * `$json.` offers the fields of the current item, `$node["Set"].`
 * the fields the node produced.
 */
function fieldSuggestions(
  position: ExpressionPosition,
  sources: SuggestionSources,
): Suggestion[] {
  const root = position.root
  const segments = position.segments

  if (root === '$json' || root === 'json') {
    if (segments.length === 0) {
      return itemFieldSuggestions(sources.itemFields, '$json')
    }
    return []
  }

  if ((root === '$node' || root === '$') && position.nodeName) {
    const fields = nodeFieldNames(position.nodeName, sources)
    const known = segments.every((segment) => segment === 'json' || segment === 'item')
    if (fields.length > 0 && known) {
      return itemFieldSuggestions(fields, `$node["${position.nodeName}"]`)
    }
    return []
  }

  return []
}

/** The field names of a node. */
function nodeFieldNames(
  nodeName: string,
  sources: SuggestionSources,
): string[] {
  const fromContext = sources.nodeFields[nodeName]
  if (fromContext && fromContext.length > 0) {
    return fromContext
  }

  const node = sources.nodes.find((candidate) => candidate.name === nodeName)
  return node?.fields ?? []
}

/** The suggestion entries of a set of field names. */
function itemFieldSuggestions(fields: string[], owner: string): Suggestion[] {
  return fields.map((field) => ({
    kind: 'field' as const,
    label: field,
    detail: `field of ${owner}`,
    insertText: field,
  }))
}

/**
 * Replaces the text at the caret with a suggestion.
 *
 * @param text - the full editor content.
 * @param caret - the caret offset inside `text`.
 * @param position - the position the suggestion was chosen for.
 * @param insertText - the text to insert.
 */
export function applySuggestionAt(
  text: string,
  caret: number,
  position: ExpressionPosition,
  insertText: string,
): { text: string; caret: number } {
  const start = Math.max(0, caret - position.replaceLength)
  const next = `${text.slice(0, start)}${insertText}${text.slice(caret)}`
  return { text: next, caret: start + insertText.length }
}
