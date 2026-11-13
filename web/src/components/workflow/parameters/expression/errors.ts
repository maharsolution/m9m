/**
 * The error banner rules of the expression editor.
 *
 * The editor shows a red banner under the field when the expression
 * cannot be parsed or cannot run. Two rules keep the banner away
 * while the user is still typing:
 *
 * 1. While the suggestion dropdown is open the banner stays away —
 *    the list is the better feedback.
 * 2. While the caret sits inside an expression that is not closed yet
 *    (a `{{` without a matching `}}`), the *unterminated* error is
 *    suppressed. A half-written expression is not broken, it is
 *    unfinished — so `{{$json.bdy` must not report a missing `}}`.
 *    Every other syntax error still shows: `{{$json..}}` reports
 *    `Expected a name but found '.'`.
 *
 * The rules live here — and not in the component — so they can be
 * unit tested without a DOM.
 */

import type { ExpressionError } from './types'

/**
 * Whether the caret sits inside an expression that is still open.
 *
 * An expression is open when a `{{` stands in front of the caret and
 * no `}}` closes it before the caret.
 *
 * @param text - the full content of the editor.
 * @param caret - the caret offset inside `text`.
 */
export function isExpressionOpenAtCaret(text: string, caret: number): boolean {
  const before = text.slice(0, Math.max(0, caret))
  const open = before.lastIndexOf('{{')
  if (open === -1) return false

  return before.indexOf('}}', open + 2) === -1
}

/** What the editor knows about the current state of the field. */
export interface DisplayErrorOptions {
  /** The full content of the editor. */
  text: string
  /** The caret offset inside `text`. */
  caret: number
  /** True while the suggestion dropdown is open. */
  suggestionsOpen: boolean
  /** True when the editor has a sample payload to evaluate against. */
  hasSampleInput: boolean
}

/**
 * The error the editor shows under the field.
 *
 * @param error - the error of the current evaluation, if any.
 * @param options - the state of the editor.
 * @returns the error to show, or `null` when the banner stays quiet.
 */
export function selectDisplayError(
  error: ExpressionError | null,
  options: DisplayErrorOptions,
): ExpressionError | null {
  if (!error) return null

  // The dropdown is the better feedback while it is open.
  if (options.suggestionsOpen) return null

  // The expression under the caret is still open — the user is typing,
  // the expression is not broken. Only the unterminated error is
  // suppressed; every other syntax error still shows.
  //
  // The error has to belong to the open expression: its `{{` stands in
  // front of the caret. An unterminated expression *behind* the caret
  // is a real error of the value and stays visible.
  if (
    error.kind === 'unterminated' &&
    error.start <= options.caret &&
    isExpressionOpenAtCaret(options.text, options.caret)
  ) {
    return null
  }

  // A runtime error is only shown when the editor has data to evaluate
  // the expression against. Without a preview there is nothing to
  // resolve `$json` with, and a red banner would only report that the
  // editor knows nothing — not that the expression is wrong.
  if (
    error.kind === 'runtime' &&
    !options.hasSampleInput &&
    error.message.startsWith('Cannot read property')
  ) {
    return null
  }

  return error
}
