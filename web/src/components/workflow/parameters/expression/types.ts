/**
 * Shared types for the browser-side expression engine.
 *
 * m9m evaluates node parameters that start with `=` on the server
 * (`internal/expressions`, goja based). The editor needs the same
 * *semantics* in the browser so the user can preview a value and see
 * errors while typing.
 *
 * The engine in this folder is a hand-written parser + interpreter for
 * the small JavaScript subset n8n expressions use. It never evaluates
 * arbitrary code: no `eval`, no `new Function`, no `with`. Every AST
 * node is walked explicitly by {@link ../evaluator}.
 */

/** A half-open source range `[start, end)` inside the expression source. */
export interface SourceSpan {
  /** Offset of the first character of the node. */
  start: number
  /** Offset just past the last character of the node. */
  end: number
}

/** A literal value: number, string, boolean, `null` or `undefined`. */
export interface LiteralNode extends SourceSpan {
  type: 'literal'
  value: string | number | boolean | null | undefined
}

/** A bare identifier such as `$json`, `uppercase` or `true`. */
export interface IdentifierNode extends SourceSpan {
  type: 'identifier'
  name: string
}

/** Property access: `a.b`, `a["b"]`, `a[0]`, `a?.b`. */
export interface MemberNode extends SourceSpan {
  type: 'member'
  object: ExpressionNode
  /** Static property name (`.b`) or the computed key expression (`["b"]`). */
  property: string | ExpressionNode
  /** True when the key is written in bracket notation. */
  computed: boolean
  /** True for optional chaining (`?.`). */
  optional: boolean
}

/** A call expression: `$input.all()`, `$("Node").first()`. */
export interface CallNode extends SourceSpan {
  type: 'call'
  callee: ExpressionNode
  args: ExpressionNode[]
  /** True for an optional call (`?.(`). */
  optional: boolean
}

/** Prefix operator: `!value` or `-value`. */
export interface UnaryNode extends SourceSpan {
  type: 'unary'
  operator: '!' | '-'
  argument: ExpressionNode
}

/** Binary operator: arithmetic, comparison or logical. */
export interface BinaryNode extends SourceSpan {
  type: 'binary'
  operator: BinaryOperator
  left: ExpressionNode
  right: ExpressionNode
}

/** Ternary: `test ? consequent : alternate`. */
export interface ConditionalNode extends SourceSpan {
  type: 'conditional'
  test: ExpressionNode
  consequent: ExpressionNode
  alternate: ExpressionNode
}

/** Binary operators supported by the engine. */
export type BinaryOperator =
  | '+'
  | '-'
  | '*'
  | '/'
  | '%'
  | '=='
  | '!='
  | '==='
  | '!=='
  | '<'
  | '<='
  | '>'
  | '>='
  | '&&'
  | '||'

/** Every node the parser can produce. */
export type ExpressionNode =
  | LiteralNode
  | IdentifierNode
  | MemberNode
  | CallNode
  | UnaryNode
  | BinaryNode
  | ConditionalNode

/** A literal text run inside a parameter value (outside `{{ … }}`). */
export interface TextChunk extends SourceSpan {
  type: 'text'
  value: string
}

/** An embedded expression inside a parameter value (`{{ … }}`). */
export interface ExpressionChunk extends SourceSpan {
  type: 'expression'
  node: ExpressionNode
}

/** A parameter value split into text and expression chunks. */
export type ValueChunk = TextChunk | ExpressionChunk

/**
 * The kind of an expression error.
 *
 * `syntax` means the expression cannot be parsed at all, `runtime`
 * means it is valid but could not be evaluated (missing data, a wrong
 * argument count, …).
 */
export type ExpressionErrorKind = 'syntax' | 'runtime'

/**
 * An error produced by the engine.
 *
 * `start`/`end` are offsets into the source that was parsed, so the
 * editor can highlight the offending span.
 */
export interface ExpressionError {
  message: string
  start: number
  end: number
  /** Whether the expression is invalid or merely could not run. */
  kind: ExpressionErrorKind
}

/** Thrown by the parser when the source is not valid m9m expression syntax. */
export class ExpressionSyntaxError extends Error {
  readonly start: number
  readonly end: number

  constructor(message: string, start: number, end: number) {
    super(message)
    this.name = 'ExpressionSyntaxError'
    this.start = start
    this.end = end
  }
}

/** Thrown by the evaluator when a syntactically valid expression cannot run. */
export class ExpressionEvaluationError extends Error {
  readonly start: number
  readonly end: number

  constructor(message: string, start: number, end: number) {
    super(message)
    this.name = 'ExpressionEvaluationError'
    this.start = start
    this.end = end
  }
}
