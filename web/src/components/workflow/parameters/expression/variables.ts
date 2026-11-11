/**
 * Variable and function catalogue used by the expression editor's
 * autocomplete.
 *
 * The variable list mirrors the n8n context variables m9m creates in
 * `internal/expressions/data_proxy.go`:
 *
 * | Variable      | Meaning |
 * | ------------- | ------- |
 * | `$json`       | JSON payload of the current item |
 * | `$input`      | incoming items (`all()`, `first()`, `last()`, `item`) |
 * | `$node`       | last output of another node (`$node["Name"].json`) |
 * | `$workflow`   | workflow metadata (id, name, active, …) |
 * | `$execution`  | execution metadata (id, mode, startedAt, …) |
 * | `$now`        | current timestamp, Luxon style (`$now.format('yyyy-MM-dd')`) |
 * | `$runIndex`   | index of the current loop iteration |
 *
 * The function list comes from the registry the Go engine installs in
 * `internal/expressions/functions.go` — see {@link FUNCTIONS} in
 * `./functions` for the full catalogue with signatures and examples.
 */

import { FUNCTIONS, type FunctionCategory } from './functions'

/** A node the editor knows about. */
export interface AvailableNode {
  name: string
  type: string
  typeVersion?: number
}

/** Metadata for one context variable. */
export interface VariableDefinition {
  /** Variable name including the `$` prefix. */
  name: string
  /** Short human readable summary. */
  description: string
  /** Example usage. */
  example: string
}

/** The context variables m9m supports. */
export const VARIABLES: VariableDefinition[] = [
  {
    name: '$json',
    description: 'JSON payload of the current input item',
    example: '$json.user.id',
  },
  {
    name: '$input',
    description: 'Items arriving from the previous node (all(), first(), last(), item)',
    example: '$input.first().json.body',
  },
  {
    name: '$node',
    description: 'Last output of another node in the workflow',
    example: '$node["Set"].json',
  },
  {
    name: '$workflow',
    description: 'Metadata of the current workflow (id, name, active, settings)',
    example: '$workflow.name',
  },
  {
    name: '$execution',
    description: 'Metadata of the current execution (id, mode, startedAt)',
    example: '$execution.id',
  },
  {
    name: '$now',
    description: 'Current timestamp with Luxon-style helpers (toISOString, format)',
    example: "$now.format('yyyy-MM-dd')",
  },
  {
    name: '$runIndex',
    description: 'Index of the current loop iteration',
    example: '$runIndex',
  },
]

/** Kind of an autocomplete suggestion. */
export type SuggestionKind = 'variable' | 'node' | 'function'

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

/** The functions m9m registers, re-exported for the editor. */
export const FUNCTION_CATALOGUE = FUNCTIONS

/** Maximum number of suggestions returned by {@link suggest}. */
const MAX_SUGGESTIONS = 25

/** Normalise a prefix for case-insensitive comparison. */
function normalise(value: string): string {
  return value.toLowerCase()
}

/** Build the `$node["Name"]` entries for the given nodes. */
export function nodeSuggestions(nodes: AvailableNode[] = []): Suggestion[] {
  return nodes.map((node) => ({
    kind: 'node' as const,
    label: `$node["${node.name}"]`,
    detail: node.type,
    insertText: `$node["${node.name}"].json`,
  }))
}

/**
 * Suggest variables, nodes and functions for a prefix.
 *
 * The prefix is the token the user has typed so far, e.g. `$js`,
 * `$node["Se` or `spl`. An empty prefix returns the variables only.
 *
 * @param prefix - the token typed at the caret.
 * @param availableNodes - nodes that can be referenced through `$node`.
 * @returns matching suggestions, most relevant first.
 */
export function suggest(prefix: string, availableNodes: AvailableNode[] = []): Suggestion[] {
  const needle = normalise(prefix)

  if (needle === '') {
    return [
      ...VARIABLES.map((variable) => variableSuggestion(variable)),
      ...nodeSuggestions(availableNodes),
    ].slice(0, MAX_SUGGESTIONS)
  }

  const suggestions: Suggestion[] = []

  for (const variable of VARIABLES) {
    if (matches(variable.name, needle)) {
      suggestions.push(variableSuggestion(variable))
    }
  }

  for (const node of nodeSuggestions(availableNodes)) {
    if (matches(node.label, needle)) {
      suggestions.push(node)
    }
  }

  // A prefix that does not start with `$` is most likely a function name.
  if (!needle.startsWith('$')) {
    for (const fn of FUNCTIONS) {
      if (matches(fn.name, needle)) {
        suggestions.push(functionSuggestion(fn))
      }
    }
  }

  return suggestions.slice(0, MAX_SUGGESTIONS)
}

/** Build the suggestion entry for a variable. */
function variableSuggestion(variable: VariableDefinition): Suggestion {
  return {
    kind: 'variable',
    label: variable.name,
    detail: variable.description,
    insertText: variable.name,
  }
}

/** Build the suggestion entry for a function. */
function functionSuggestion(fn: {
  name: string
  description: string
  signature: string
  category: FunctionCategory
}): Suggestion {
  return {
    kind: 'function',
    label: fn.name,
    detail: `${fn.signature} — ${fn.description}`,
    insertText: `${fn.name}(`,
  }
}

/** Whether a label matches the needle (prefix match first, then contains). */
function matches(label: string, needle: string): boolean {
  const haystack = normalise(label)
  if (haystack.startsWith(needle)) return true
  if (needle.startsWith('$')) {
    // `$node["Set"]` should still match a `$nod` prefix.
    return haystack.replace(/[^a-z]/g, '').startsWith(needle.replace(/[^a-z]/g, ''))
  }
  return haystack.includes(needle)
}

/**
 * Extract the autocomplete prefix at a caret position.
 *
 * The prefix is the run of identifier characters directly before the
 * caret. A quoted node name (`$node["Se`) is returned as a whole so
 * the node suggestions can be filtered by it.
 *
 * @param text - the full editor content.
 * @param caret - caret offset inside `text`.
 * @returns the prefix, or an empty string when there is none.
 */
export function suggestionPrefix(text: string, caret: number): string {
  const before = text.slice(0, Math.max(0, caret))
  const quoted = before.match(/\$node\["[^"]*$/i)
  if (quoted) return quoted[0]

  const match = before.match(/[$A-Za-z_][$A-Za-z0-9_]*$/)
  return match ? match[0] : ''
}

/**
 * Replace the prefix at the caret with a suggestion.
 *
 * @param text - the full editor content.
 * @param caret - caret offset inside `text`.
 * @param insertText - text to insert.
 * @returns the new content and the caret position after the insertion.
 */
export function applySuggestion(
  text: string,
  caret: number,
  insertText: string,
): { text: string; caret: number } {
  const prefix = suggestionPrefix(text, caret)
  const start = Math.max(0, caret - prefix.length)
  const next = `${text.slice(0, start)}${insertText}${text.slice(caret)}`
  return { text: next, caret: start + insertText.length }
}
