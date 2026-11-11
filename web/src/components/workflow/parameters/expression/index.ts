/**
 * Public API of the browser-side expression engine.
 *
 * ```ts
 * import { evaluateExpression, suggest } from './expression'
 *
 * const result = evaluateExpression('{{ $json.user.id }}', { json: { user: { id: 7 } } })
 * // => { ok: true, value: 7 }
 *
 * const suggestions = suggest('$js')
 * // => [{ kind: 'variable', label: '$json', … }]
 * ```
 *
 * The engine is a hand-written parser plus interpreter — it never
 * evaluates arbitrary JavaScript in the page.
 */

export {
  parseExpression,
  parseValue,
  isExpressionEnvelope,
  wrapExpression,
  unwrapExpression,
  spanOf,
} from './parser'

export {
  ExpressionEvaluationError,
  ExpressionSyntaxError,
  type BinaryOperator,
  type CallNode,
  type ConditionalNode,
  type ExpressionChunk,
  type ExpressionError,
  type ExpressionNode,
  type IdentifierNode,
  type LiteralNode,
  type MemberNode,
  type SourceSpan,
  type TextChunk,
  type UnaryNode,
  type ValueChunk,
  type BinaryNode,
} from './types'

export {
  BUILTIN_FUNCTIONS,
  FUNCTIONS,
  getBuiltinFunction,
  isEmptyValue,
  isTruthy,
  toIntegerValue,
  toNumberValue,
  toStringValue,
  type BuiltinFunction,
  type FunctionCategory,
  type FunctionDefinition,
} from './functions'

export {
  createPreviewContext,
  evaluateExpression,
  evaluateValue,
  formatPreview,
  type EvaluationResult,
  type ExpressionContext,
} from './evaluator'

export {
  FUNCTION_CATALOGUE,
  VARIABLES,
  applySuggestion,
  nodeSuggestions,
  suggestionPrefix,
  suggest,
  type AvailableNode,
  type Suggestion,
  type SuggestionKind,
  type VariableDefinition,
} from './variables'