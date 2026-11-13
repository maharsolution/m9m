/**
 * The data the expression editor completes with.
 *
 * The node details view knows the payload the current item carries and
 * the output every other node produced (after the workflow ran). It
 * hands that data to the parameter fields through provide / inject, so
 * the expression editor can offer the same completions n8n offers.
 */

import type { ComputedRef, InjectionKey } from 'vue'
import type { ExpressionFieldSources } from './expressionFields'

/** The completion data of the open node. */
export type ExpressionContextValue = ExpressionFieldSources

/**
 * The injection key of the expression context.
 *
 * The value is a computed ref, so the consumers react when a new
 * execution arrives.
 */
export const EXPRESSION_CONTEXT_KEY: InjectionKey<
  ComputedRef<ExpressionContextValue>
> = Symbol('m9m:expression-context')
