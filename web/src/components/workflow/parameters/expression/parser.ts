/**
 * Tokeniser and parser for the m9m/n8n expression subset.
 *
 * The grammar is a strict subset of JavaScript — enough for the
 * expressions n8n workflows actually use, small enough to parse
 * without a JavaScript engine:
 *
 * ```text
 * expression  := conditional
 * conditional := logicalOr ( '?' conditional ':' conditional )?
 * logicalOr   := logicalAnd ( '||' logicalAnd )*
 * logicalAnd  := equality ( '&&' equality )*
 * equality    := relational ( ('==' | '!=' | '===' | '!==') relational )*
 * relational  := additive ( ('<' | '<=' | '>' | '>=') additive )*
 * additive    := multiplicative ( ('+' | '-') multiplicative )*
 * multiplicative := unary ( ('*' | '/' | '%') unary )*
 * unary       := ('!' | '-') unary | postfix
 * postfix     := primary ( '.' ident | '[' expression ']' | '?.' ident
 *                          | '?.[' expression ']' | '(' args ')' )*
 * primary     := number | string | 'true' | 'false' | 'null' | 'undefined'
 *              | identifier | '(' expression ')'
 * ```
 *
 * Everything else (arrow functions, template literals, assignment,
 * `new`, `typeof`, …) is rejected with an `Unexpected token …` error.
 * The parser never executes anything — it only builds an AST.
 */

import {
  ExpressionSyntaxError,
  type BinaryOperator,
  type ExpressionChunk,
  type ExpressionNode,
  type SourceSpan,
  type TextChunk,
  type ValueChunk,
} from './types'

type TokenType = 'number' | 'string' | 'identifier' | 'punct' | 'eof'

interface Token {
  type: TokenType
  value: string
  start: number
  end: number
}

/** Multi-character punctuators, longest first so `===` beats `==`. */
const PUNCTUATORS = [
  '===',
  '!==',
  '==',
  '!=',
  '<=',
  '>=',
  '&&',
  '||',
  '?.',
  '+',
  '-',
  '*',
  '/',
  '%',
  '<',
  '>',
  '!',
  '?',
  ':',
  '(',
  ')',
  '[',
  ']',
  '.',
  ',',
]

/** Binary operator precedence — higher binds tighter. */
const PRECEDENCE: Record<string, number> = {
  '||': 1,
  '&&': 2,
  '==': 3,
  '!=': 3,
  '===': 3,
  '!==': 3,
  '<': 4,
  '<=': 4,
  '>': 4,
  '>=': 4,
  '+': 5,
  '-': 5,
  '*': 6,
  '/': 6,
  '%': 6,
}

const KEYWORD_LITERALS: Record<string, string | number | boolean | null | undefined> = {
  true: true,
  false: false,
  null: null,
  undefined: undefined,
}

function isDigit(char: string | undefined): boolean {
  return char !== undefined && char >= '0' && char <= '9'
}

function isIdentStart(char: string | undefined): boolean {
  if (char === undefined) return false
  return (
    (char >= 'a' && char <= 'z') ||
    (char >= 'A' && char <= 'Z') ||
    char === '_' ||
    char === '$'
  )
}

function isIdentPart(char: string | undefined): boolean {
  return isIdentStart(char) || isDigit(char)
}

function describeToken(token: Token): string {
  if (token.type === 'eof') return 'end of expression'
  return `'${token.value}'`
}

function unescapeString(raw: string): string {
  let out = ''
  for (let i = 0; i < raw.length; i += 1) {
    const char = raw[i]
    if (char !== '\\') {
      out += char
      continue
    }
    const next = raw[i + 1]
    i += 1
    switch (next) {
      case 'n':
        out += '\n'
        break
      case 't':
        out += '\t'
        break
      case 'r':
        out += '\r'
        break
      case 'b':
        out += '\b'
        break
      case 'f':
        out += '\f'
        break
      case 'v':
        out += '\v'
        break
      case '0':
        out += '\0'
        break
      case 'u': {
        const hex = raw.slice(i + 1, i + 5)
        if (/^[0-9a-fA-F]{4}$/.test(hex)) {
          out += String.fromCharCode(parseInt(hex, 16))
          i += 4
        } else {
          out += 'u'
        }
        break
      }
      case undefined:
        out += '\\'
        break
      default:
        out += next
    }
  }
  return out
}

/** Split the source into tokens. */
function tokenize(source: string): Token[] {
  const tokens: Token[] = []
  let index = 0

  while (index < source.length) {
    const char = source[index]

    if (char === ' ' || char === '\t' || char === '\n' || char === '\r') {
      index += 1
      continue
    }

    // Number literal.
    if (isDigit(char) || (char === '.' && isDigit(source[index + 1]))) {
      const start = index
      while (isDigit(source[index])) index += 1
      if (source[index] === '.' && isDigit(source[index + 1])) {
        index += 1
        while (isDigit(source[index])) index += 1
      } else if (source[index] === '.' && !isIdentStart(source[index + 1])) {
        // Trailing dot of `1.` — consume it so `1.toFixed` still parses.
        index += 1
      }
      if (source[index] === 'e' || source[index] === 'E') {
        const exponentStart = index
        index += 1
        if (source[index] === '+' || source[index] === '-') index += 1
        if (isDigit(source[index])) {
          while (isDigit(source[index])) index += 1
        } else {
          index = exponentStart
        }
      }
      tokens.push({ type: 'number', value: source.slice(start, index), start, end: index })
      continue
    }

    // String literal.
    if (char === '"' || char === "'") {
      const start = index
      const quote = char
      index += 1
      let terminated = false
      while (index < source.length) {
        const current = source[index]
        if (current === '\\') {
          index += 2
          continue
        }
        if (current === quote) {
          index += 1
          terminated = true
          break
        }
        index += 1
      }
      if (!terminated) {
        throw new ExpressionSyntaxError('Unterminated string literal', start, source.length)
      }
      tokens.push({
        type: 'string',
        value: unescapeString(source.slice(start + 1, index - 1)),
        start,
        end: index,
      })
      continue
    }

    // Identifier / keyword.
    if (isIdentStart(char)) {
      const start = index
      while (isIdentPart(source[index])) index += 1
      tokens.push({ type: 'identifier', value: source.slice(start, index), start, end: index })
      continue
    }

    // Punctuator.
    let matched = ''
    for (const punctuator of PUNCTUATORS) {
      if (source.startsWith(punctuator, index)) {
        matched = punctuator
        break
      }
    }
    if (!matched) {
      throw new ExpressionSyntaxError(
        `Unexpected character '${char}'`,
        index,
        index + 1,
      )
    }
    // `a?.5:b` is a conditional with a fractional literal, not optional
    // chaining — the JavaScript spec makes `?.` a punctuator only when the
    // next character is not a digit.
    if (matched === '?.' && isDigit(source[index + 2])) {
      tokens.push({ type: 'punct', value: '?', start: index, end: index + 1 })
      index += 1
      continue
    }
    tokens.push({ type: 'punct', value: matched, start: index, end: index + matched.length })
    index += matched.length
  }

  tokens.push({ type: 'eof', value: '', start: source.length, end: source.length })
  return tokens
}

class Parser {
  private readonly tokens: Token[]
  private position = 0

  constructor(source: string) {
    this.tokens = tokenize(source)
  }

  private peek(offset = 0): Token {
    const index = Math.min(this.position + offset, this.tokens.length - 1)
    return this.tokens[index]
  }

  private advance(): Token {
    const token = this.peek()
    if (token.type !== 'eof') this.position += 1
    return token
  }

  private match(value: string): boolean {
    const token = this.peek()
    if (token.type === 'punct' && token.value === value) {
      this.position += 1
      return true
    }
    return false
  }

  private expect(value: string): Token {
    const token = this.peek()
    if (token.type === 'punct' && token.value === value) {
      this.position += 1
      return token
    }
    throw new ExpressionSyntaxError(
      `Expected '${value}' but found ${describeToken(token)}`,
      token.start,
      token.end,
    )
  }

  private expectIdentifier(): Token {
    const token = this.advance()
    if (token.type !== 'identifier') {
      throw new ExpressionSyntaxError(
        `Expected a name but found ${describeToken(token)}`,
        token.start,
        token.end,
      )
    }
    return token
  }

  /** Parse the whole token stream into an AST. */
  parse(): ExpressionNode {
    if (this.peek().type === 'eof') {
      throw new ExpressionSyntaxError('Empty expression', 0, 0)
    }
    const node = this.parseConditional()
    const token = this.peek()
    if (token.type !== 'eof') {
      throw new ExpressionSyntaxError(
        `Unexpected token ${describeToken(token)}`,
        token.start,
        token.end,
      )
    }
    return node
  }

  private parseConditional(): ExpressionNode {
    const test = this.parseBinary(0)
    if (!this.match('?')) return test
    const consequent = this.parseConditional()
    this.expect(':')
    const alternate = this.parseConditional()
    return {
      type: 'conditional',
      test,
      consequent,
      alternate,
      start: test.start,
      end: alternate.end,
    }
  }

  private parseBinary(minPrecedence: number): ExpressionNode {
    let left = this.parseUnary()
    for (;;) {
      const token = this.peek()
      if (token.type !== 'punct') break
      const precedence = PRECEDENCE[token.value]
      if (precedence === undefined || precedence < minPrecedence) break
      this.position += 1
      const right = this.parseBinary(precedence + 1)
      left = {
        type: 'binary',
        operator: token.value as BinaryOperator,
        left,
        right,
        start: left.start,
        end: right.end,
      }
    }
    return left
  }

  private parseUnary(): ExpressionNode {
    const token = this.peek()
    if (token.type === 'punct' && (token.value === '!' || token.value === '-')) {
      this.position += 1
      const argument = this.parseUnary()
      return {
        type: 'unary',
        operator: token.value as '!' | '-',
        argument,
        start: token.start,
        end: argument.end,
      }
    }
    return this.parsePostfix()
  }

  private parsePostfix(): ExpressionNode {
    let node = this.parsePrimary()
    for (;;) {
      const token = this.peek()
      if (token.type !== 'punct') break

      if (token.value === '.') {
        this.position += 1
        const name = this.expectIdentifier()
        node = {
          type: 'member',
          object: node,
          property: name.value,
          computed: false,
          optional: false,
          start: node.start,
          end: name.end,
        }
        continue
      }

      if (token.value === '?.') {
        this.position += 1
        const next = this.peek()
        if (next.type === 'punct' && next.value === '(') {
          node = this.parseCall(node, true)
          continue
        }
        if (next.type === 'punct' && next.value === '[') {
          this.position += 1
          const key = this.parseConditional()
          const close = this.expect(']')
          node = {
            type: 'member',
            object: node,
            property: key,
            computed: true,
            optional: true,
            start: node.start,
            end: close.end,
          }
          continue
        }
        const name = this.expectIdentifier()
        node = {
          type: 'member',
          object: node,
          property: name.value,
          computed: false,
          optional: true,
          start: node.start,
          end: name.end,
        }
        continue
      }

      if (token.value === '[') {
        this.position += 1
        const key = this.parseConditional()
        const close = this.expect(']')
        node = {
          type: 'member',
          object: node,
          property: key,
          computed: true,
          optional: false,
          start: node.start,
          end: close.end,
        }
        continue
      }

      if (token.value === '(') {
        node = this.parseCall(node, false)
        continue
      }

      break
    }
    return node
  }

  private parseCall(callee: ExpressionNode, optional: boolean): ExpressionNode {
    this.expect('(')
    const args: ExpressionNode[] = []
    if (!this.match(')')) {
      do {
        args.push(this.parseConditional())
      } while (this.match(','))
      this.expect(')')
    }
    const last = this.tokens[Math.max(this.position - 1, 0)]
    return {
      type: 'call',
      callee,
      args,
      optional,
      start: callee.start,
      end: last.end,
    }
  }

  private parsePrimary(): ExpressionNode {
    const token = this.advance()

    if (token.type === 'number') {
      return { type: 'literal', value: Number(token.value), start: token.start, end: token.end }
    }

    if (token.type === 'string') {
      return { type: 'literal', value: token.value, start: token.start, end: token.end }
    }

    if (token.type === 'identifier') {
      if (token.value in KEYWORD_LITERALS) {
        return {
          type: 'literal',
          value: KEYWORD_LITERALS[token.value],
          start: token.start,
          end: token.end,
        }
      }
      return { type: 'identifier', name: token.value, start: token.start, end: token.end }
    }

    if (token.type === 'punct' && token.value === '(') {
      const node = this.parseConditional()
      this.expect(')')
      return node
    }

    throw new ExpressionSyntaxError(
      `Unexpected token ${describeToken(token)}`,
      token.start,
      token.end,
    )
  }
}

/**
 * Parse a bare expression body (the text between `{{` and `}}`).
 *
 * @throws {ExpressionSyntaxError} when the body is not valid syntax.
 */
export function parseExpression(source: string): ExpressionNode {
  return new Parser(source).parse()
}

/**
 * Shift every span of an AST by `offset` characters.
 *
 * The parser works on the body of a `{{ … }}` chunk, so the spans it
 * produces are relative to that body. The evaluator needs offsets into
 * the *original* value to highlight the offending expression, therefore
 * the spans are shifted by the offset of the chunk.
 *
 * @param node - the parsed AST.
 * @param offset - number of characters the AST has to be moved by.
 * @returns the same AST with absolute spans.
 */
export function shiftSpans(node: ExpressionNode, offset: number): ExpressionNode {
  if (offset === 0) return node
  const start = node.start + offset
  const end = node.end + offset

  switch (node.type) {
    case 'literal':
    case 'identifier':
      return { ...node, start, end }

    case 'member':
      return {
        type: 'member',
        object: shiftSpans(node.object, offset),
        property:
          typeof node.property === 'string' ? node.property : shiftSpans(node.property, offset),
        computed: node.computed,
        optional: node.optional,
        start,
        end,
      }

    case 'call':
      return {
        type: 'call',
        callee: shiftSpans(node.callee, offset),
        args: node.args.map((argument) => shiftSpans(argument, offset)),
        optional: node.optional,
        start,
        end,
      }

    case 'unary':
      return {
        type: 'unary',
        operator: node.operator,
        argument: shiftSpans(node.argument, offset),
        start,
        end,
      }

    case 'binary':
      return {
        type: 'binary',
        operator: node.operator,
        left: shiftSpans(node.left, offset),
        right: shiftSpans(node.right, offset),
        start,
        end,
      }

    case 'conditional':
      return {
        type: 'conditional',
        test: shiftSpans(node.test, offset),
        consequent: shiftSpans(node.consequent, offset),
        alternate: shiftSpans(node.alternate, offset),
        start,
        end,
      }

    default: {
      // Every node type is handled above, so `node` narrows to `never`
      // here — the cast keeps the span available.
      const unsupported = node as unknown as ExpressionNode
      return { ...unsupported, start, end }
    }
  }
}

/**
 * Split a parameter value into text and expression chunks.
 *
 * Mirrors the server-side chunking in `internal/expressions/parser.go`:
 * `{{ … }}` runs are expressions, everything else is literal text.
 * `\{` escapes a literal brace.
 *
 * @throws {ExpressionSyntaxError} when a `{{` has no matching `}}` or a
 * chunk does not parse.
 */
export function parseValue(source: string): ValueChunk[] {
  const chunks: ValueChunk[] = []
  let index = 0
  let textStart = 0

  while (index < source.length) {
    const open = source.indexOf('{{', index)
    if (open === -1) break

    // `\{` is an escaped brace — treat it as text.
    if (open > 0 && source[open - 1] === '\\') {
      index = open + 2
      continue
    }

    const close = findClosingBraces(source, open + 2)
    if (close === -1) {
      throw new ExpressionSyntaxError(
        "Unterminated expression: missing '}}'",
        open,
        source.length,
      )
    }

    if (open > textStart) {
      const text: TextChunk = {
        type: 'text',
        value: source.slice(textStart, open),
        start: textStart,
        end: open,
      }
      chunks.push(text)
    }

    const body = source.slice(open + 2, close)
    let node: ExpressionNode
    try {
      // The parser works on the chunk body, so its spans are shifted to
      // the offset of the chunk to stay absolute inside `source`.
      node = shiftSpans(parseExpression(body), open + 2)
    } catch (error) {
      if (error instanceof ExpressionSyntaxError) {
        throw new ExpressionSyntaxError(
          error.message,
          open + 2 + error.start,
          open + 2 + error.end,
        )
      }
      throw error
    }
    const expression: ExpressionChunk = {
      type: 'expression',
      node,
      start: open + 2,
      end: close,
    }
    chunks.push(expression)

    index = close + 2
    textStart = index
  }

  if (textStart < source.length) {
    const text: TextChunk = {
      type: 'text',
      value: source.slice(textStart),
      start: textStart,
      end: source.length,
    }
    chunks.push(text)
  }

  return chunks
}

/** Find the `}}` that closes the `{{` opened before `from`. */
function findClosingBraces(source: string, from: number): number {
  let depth = 1
  let index = from
  while (index < source.length) {
    const open = source.indexOf('{{', index)
    const close = source.indexOf('}}', index)
    if (close === -1) return -1
    if (open !== -1 && open < close) {
      depth += 1
      index = open + 2
      continue
    }
    depth -= 1
    if (depth === 0) return close
    index = close + 2
  }
  return -1
}

/**
 * True when the value is an m9m expression envelope.
 *
 * Matches `={{ … }}` (the canonical n8n form) and the bare `{{ … }}`
 * form the server also accepts.
 */
export function isExpressionEnvelope(value: unknown): boolean {
  if (typeof value !== 'string') return false
  return /^\s*=?\{\{[\s\S]*\}\}\s*$/.test(value)
}

/** Wrap an expression body into the canonical `={{ … }}` envelope. */
export function wrapExpression(body: string): string {
  return `={{ ${body} }}`
}

/**
 * Remove the expression envelope from a value.
 *
 * `={{ $json.id }}` becomes `$json.id`; `Hello {{ $json.id }}` keeps its
 * text (it is not a pure envelope).
 */
export function unwrapExpression(value: string): string {
  const match = value.match(/^\s*=?\{\{([\s\S]*)\}\}\s*$/)
  if (!match) return value.replace(/^=/, '')
  return match[1].trim()
}

/** Source span helper used by the editor to highlight a range. */
export function spanOf(node: SourceSpan): SourceSpan {
  return { start: node.start, end: node.end }
}
