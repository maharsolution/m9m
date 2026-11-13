/**
 * The autocomplete engine of the expression editor.
 *
 * n8n opens a suggestion list as soon as the user types `{{` and
 * keeps it open while the expression is being written:
 *
 *   `{{`              → the variables, the nodes and the functions
 *   `{{$js`           → the same list, filtered
 *   `{{$json.`        → the fields of the current item
 *   `{{$json.address.` → the fields of `address` (nested paths)
 *   `{{$node["Set"].` → the fields the node "Set" produced
 *
 * The engine is pure: it reads the text and the caret, and returns
 * the suggestions. Everything it needs (the catalogue, the fields of
 * the items) is handed in, so the logic can be unit tested without
 * a DOM.
 */

import type { FieldTree } from '@/lib/expressionFields'
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

/** The needle matches the label exactly. */
export const MATCH_EXACT = 0
/** The needle is a prefix of the label. */
export const MATCH_PREFIX = 1
/** The needle is a prefix of the label, ignoring punctuation. */
export const MATCH_LOOSE_PREFIX = 2
/** The label contains the needle. */
export const MATCH_SUBSTRING = 3
/** The needle matches the label as a scattered subsequence. */
export const MATCH_FUZZY = 4
/** The needle does not match the label at all. */
export const MATCH_NONE = -1

/** The order the kinds are listed in (variables, nodes, functions). */
const KIND_ORDER: Record<SuggestionKind, number> = {
  variable: 0,
  node: 1,
  function: 2,
  field: 3,
}

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
  /** The path segment that is currently being typed (`$json.add`). */
  partialSegment: string | null
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
  /** The nested field tree of the current item (`$json.address.city`). */
  itemTree?: FieldTree
  /** The nested field tree per node name (`$node["Name"].json.address`). */
  nodeTrees?: Record<string, FieldTree>
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
  partialSegment: null,
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
      partialSegment: null,
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
      partialSegment: null,
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

  // The token belongs to the path (and is not the root itself) when
  // the path in front of it ends with a dot: `$json.add` types the
  // segment `add` of the path `$json.`.
  const partialSegment =
    !endsWithDot && replaceLength > 0 && pathCore.endsWith('.') ? prefix : null

  return {
    inExpression: true,
    prefix,
    replaceLength,
    root: parsed.root,
    nodeName: parsed.nodeName,
    segments: parsed.segments,
    nodeNamePrefix: null,
    partialSegment,
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
 * The quality of the match between a label and a needle.
 *
 * `0` is an exact match, `1` a prefix, `2` a loose prefix (a needle
 * that starts with `$`, matched without the punctuation), `3` a
 * substring and `4` a fuzzy match — the characters of the needle occur
 * in the label, in order, but scattered. `-1` means the needle does
 * not match the label at all.
 */
export function matchRank(label: string, needle: string): number {
  if (needle === '') {
    return MATCH_EXACT
  }

  const lowerLabel = label.toLowerCase()
  const lowerNeedle = needle.toLowerCase()

  if (lowerLabel === lowerNeedle) return MATCH_EXACT
  if (lowerLabel.startsWith(lowerNeedle)) return MATCH_PREFIX

  if (lowerNeedle.startsWith('$')) {
    const plainLabel = plainCharacters(label)
    const plainNeedle = plainCharacters(needle)
    if (plainNeedle !== '' && plainLabel.startsWith(plainNeedle)) {
      return MATCH_LOOSE_PREFIX
    }
  }

  if (lowerLabel.includes(lowerNeedle)) return MATCH_SUBSTRING

  return fuzzyRank(label, needle)
}

/**
 * Whether a label matches the needle.
 *
 * A prefix match wins; a needle that starts with `$` also matches a
 * label whose non-word characters are skipped, so `$nod` matches
 * `$node["Set"]`. A needle whose characters are scattered over the
 * label matches as well — `jsn` finds `$json`.
 */
export function matchesLabel(label: string, needle: string): boolean {
  return matchRank(label, needle) !== MATCH_NONE
}

/**
 * The rank of a scattered (subsequence) match.
 *
 * The characters of the needle have to occur in the label in order,
 * but they do not have to be adjacent. Up to a third of the needle may
 * be missing from the label — `wnf` still finds `$workflow` and
 * `jn["st` still finds `$node["Set"]`.
 */
function fuzzyRank(label: string, needle: string): number {
  const haystack = plainCharacters(label)
  const chars = plainCharacters(needle)

  if (chars === '' || haystack === '') return MATCH_NONE

  let position = 0
  let missing = 0

  for (const char of chars) {
    const found = haystack.indexOf(char, position)
    if (found === -1) {
      missing += 1
      continue
    }
    position = found + 1
  }

  if (missing > Math.floor(chars.length / 3)) return MATCH_NONE

  return MATCH_FUZZY
}

/** The letters and digits of a string, lower cased. */
function plainCharacters(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]/g, '')
}

/**
 * Filters a list of suggestions by the needle.
 *
 * The list is sorted: the best match first (exact, prefix, substring,
 * fuzzy) and alphabetical inside a group. It never grows beyond
 * {@link MAX_SUGGESTIONS} entries.
 */
export function filterSuggestions(
  suggestions: Suggestion[],
  needle: string,
): Suggestion[] {
  return suggestions
    .map((suggestion, index) => ({
      suggestion,
      index,
      rank: matchRank(suggestion.label, needle),
    }))
    .filter((entry) => entry.rank !== MATCH_NONE)
    .sort(compareEntries)
    .slice(0, MAX_SUGGESTIONS)
    .map((entry) => entry.suggestion)
}

/** Sorts the matched entries: rank, then kind, then alphabetically. */
function compareEntries(
  left: { suggestion: Suggestion; index: number; rank: number },
  right: { suggestion: Suggestion; index: number; rank: number },
): number {
  if (left.rank !== right.rank) return left.rank - right.rank

  const kind = KIND_ORDER[left.suggestion.kind] - KIND_ORDER[right.suggestion.kind]
  if (kind !== 0) return kind

  const label = compareLabels(left.suggestion.label, right.suggestion.label)
  if (label !== 0) return label

  return left.index - right.index
}

/** Compares two labels without depending on the locale. */
function compareLabels(left: string, right: string): number {
  if (left < right) return -1
  if (left > right) return 1
  return 0
}

/**
 * Builds the suggestion list for a caret position.
 *
 * The list depends on what the user typed:
 *
 * * right after `{{` — the variables, the nodes and the functions,
 * * a variable name — the same list, filtered,
 * * a node name — the nodes whose name matches,
 * * behind a dot — the fields of the item or of the node.
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
    return fieldSuggestions(position, sources) ?? []
  }

  // The user is typing a field of a path (`$json.add`) — the fields of
  // the path are offered, not the root list.
  if (position.partialSegment !== null) {
    const fields = fieldSuggestions(position, sources)
    if (fields) return fields
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
 * The field suggestions for a position inside a path.
 *
 * `$json.` offers the fields of the current item, `$json.address.` the
 * fields of `address`, `$node["Set"].` the fields the node produced.
 * A path that does not resolve offers nothing — the editor never
 * guesses a field that is not in the payload.
 *
 * Returns `null` when the position is not a field position at all.
 */
function fieldSuggestions(
  position: ExpressionPosition,
  sources: SuggestionSources,
): Suggestion[] | null {
  const root = position.root
  const segments = position.segments

  if (root === '$json' || root === 'json') {
    const tree = sources.itemTree
    if (!tree) {
      // Without a tree only the top level fields are known.
      if (segments.length > 0) return []
      return itemFieldSuggestions(
        sources.itemFields,
        '$json',
        position.partialSegment,
      )
    }

    const resolved = resolveFieldTree(tree, segments)
    if (!resolved) return []

    return itemFieldSuggestions(
      resolved.fields,
      ownerPath('$json', segments),
      position.partialSegment,
    )
  }

  if ((root === '$node' || root === '$') && position.nodeName) {
    const owner = `$node["${position.nodeName}"]`
    const tree = nodeFieldTree(position.nodeName, sources)

    if (!tree) {
      const fields = nodeFieldNames(position.nodeName, sources)
      const known = segments.every((segment) => segment === 'json' || segment === 'item')
      if (fields.length > 0 && known) {
        return itemFieldSuggestions(
          fields,
          ownerPath(owner, segments),
          position.partialSegment,
        )
      }
      return []
    }

    const resolved = resolveFieldTree(tree, payloadSegments(segments))
    if (!resolved) return []

    return itemFieldSuggestions(
      resolved.fields,
      ownerPath(owner, segments),
      position.partialSegment,
    )
  }

  return null
}

/**
 * Walks the field tree along a path.
 *
 * Returns `null` when a segment of the path does not exist.
 */
function resolveFieldTree(tree: FieldTree, segments: string[]): FieldTree | null {
  let current = tree

  for (const segment of segments) {
    if (!Object.prototype.hasOwnProperty.call(current.children, segment)) {
      return null
    }
    current = current.children[segment]
  }

  return current
}

/**
 * The segments below the payload of a node.
 *
 * `json` and `item` address the payload itself, so they are dropped.
 */
function payloadSegments(segments: string[]): string[] {
  const result = [...segments]
  while (result.length > 0 && (result[0] === 'json' || result[0] === 'item')) {
    result.shift()
  }
  return result
}

/** The path a field belongs to — shown as the detail of the entry. */
function ownerPath(owner: string, segments: string[]): string {
  return segments.length === 0 ? owner : `${owner}.${segments.join('.')}`
}

/** The field tree of a node. */
function nodeFieldTree(
  nodeName: string,
  sources: SuggestionSources,
): FieldTree | undefined {
  const fromContext = sources.nodeTrees?.[nodeName]
  if (fromContext) {
    return fromContext
  }

  const node = sources.nodes.find((candidate) => candidate.name === nodeName)
  return node?.fieldTree
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

/**
 * The suggestion entries of a set of field names.
 *
 * A field name is typed literally, so the list is filtered by an exact
 * or a prefix match — never by a fuzzy one.
 */
function itemFieldSuggestions(
  fields: string[],
  owner: string,
  partial: string | null,
): Suggestion[] {
  const names = partial === null ? fields : filterFieldNames(fields, partial)

  return names.slice(0, MAX_SUGGESTIONS).map((field) => ({
    kind: 'field' as const,
    label: field,
    detail: `field of ${owner}`,
    insertText: field,
  }))
}

/** Filters field names by an exact or a prefix match. */
function filterFieldNames(fields: string[], needle: string): string[] {
  if (needle === '') return fields

  const exact = fields.filter((field) => field === needle)
  const prefixed = fields.filter(
    (field) => field !== needle && field.toLowerCase().startsWith(needle.toLowerCase()),
  )

  return [...exact, ...prefixed]
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