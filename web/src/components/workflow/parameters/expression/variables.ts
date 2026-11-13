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
 *
 * The autocomplete itself lives in `./suggestions`.
 */

import { FUNCTIONS } from './functions'
import {
  applySuggestionAt,
  detectExpressionPosition,
  suggestForPosition,
  type ExpressionPosition,
  type Suggestion,
  type SuggestionKind,
} from './suggestions'

export type { Suggestion, SuggestionKind }

/** A node the editor knows about. */
export interface AvailableNode {
  name: string
  type: string
  typeVersion?: number
  /** Field names the node produced — used by `$node["Name"].json.<field>`. */
  fields?: string[]
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

/** The functions m9m registers, re-exported for the editor. */
export const FUNCTION_CATALOGUE = FUNCTIONS

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
  const position: ExpressionPosition = {
    inExpression: true,
    prefix,
    replaceLength: prefix.length,
    root: null,
    nodeName: null,
    segments: [],
    nodeNamePrefix: null,
    afterDot: false,
  }

  return suggestForPosition(position, {
    variables: VARIABLES,
    nodes: availableNodes,
    functions: FUNCTIONS,
    itemFields: [],
    nodeFields: {},
  })
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
  const position = detectExpressionPosition(text, caret, { insideExpression: true })

  if (position.nodeNamePrefix !== null) {
    return text.slice(Math.max(0, caret - position.replaceLength), caret)
  }

  return position.prefix
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
  const position = detectExpressionPosition(text, caret, { insideExpression: true })
  return applySuggestionAt(text, caret, position, insertText)
}
