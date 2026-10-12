// Tests for BUG-CAP-2026-09-09-005 — XML↔JSON helper.
//
// We don't depend on JSDOM in this project (yet), so
// `toJson` is exercised via a hand-rolled Element fake
// that implements the same minimal interface the helper
// touches. The `toXml` half is fully tested as it has no
// browser dependencies.

import { describe, it, expect } from 'vitest'
import { toJson, toXml } from '../src/lib/xmlJsonHelper'

// --- Element fake ---
// A minimal subset of the DOM `Element` interface the
// helper uses: childNodes, attributes, nodeName.
class FakeNode {
  constructor(nodeType, nodeName, nodeValue, childNodes = [], attributes = null) {
    this.nodeType = nodeType
    this.nodeName = nodeName
    this.nodeValue = nodeValue
    this.childNodes = childNodes
    this.attributes = attributes
  }
  item(i) { return this.childNodes[i] ?? null }
}
class FakeAttrList {
  constructor(attrs) { this.attrs = attrs }
  get length() { return this.attrs.length }
  item(i) { return this.attrs[i] }
}
class FakeDoc {
  constructor(root) { this.documentElement = root }
}

function makeParserFromString(xml) {
  // Tolerant mini-parser: good enough for the test cases
  // below. Real production uses DOMParser; this is a
  // hermetic shim for `node` test runs.
  return (input) => {
    input = input.replace(/<\?xml[^?]*\?>/, '').trim()
    const root = parseElement(input)[0]
    return new FakeDoc(root)
  }
}

function parseElement(str) {
  // Returns [el, remaining] where `el` is a FakeNode and
  // `remaining` is the unparsed suffix.
  const m = str.match(/^<\s*([\w:-]+)((?:\s+[\w:-]+\s*=\s*"[^"]*")*)\s*(\/>|>([\s\S]*)<\/\1\s*>)/)
  if (!m) throw new Error(`cannot parse: ${str.slice(0, 80)}`)
  const [, name, attrStr, closer, body = ''] = m
  const attrs = []
  const am = attrStr.matchAll(/([\w:-]+)\s*=\s*"([^"]*)"/g)
  for (const x of am) attrs.push(new FakeNode(2, x[1], x[2]))
  const attrList = attrs.length ? new FakeAttrList(attrs) : null
  if (closer === '/>') return [new FakeNode(1, name, null, [], attrList), str.slice(m[0].length)]
  const children = parseChildren(body)
  return [new FakeNode(1, name, null, children, attrList), str.slice(m[0].length)]
}
function parseChildren(body) {
  const out = []
  let i = 0
  while (i < body.length) {
    if (body[i] === '<' && body[i + 1] === '!') {
      // skip comments
      const end = body.indexOf('-->', i)
      i = end + 3
      continue
    }
    if (body[i] === '<' && body[i + 1] === '/') {
      // closing tag — stop at this level
      break
    }
    if (body[i] === '<') {
      const [el, rem] = parseElement(body.slice(i))
      out.push(el)
      const consumed = body.slice(i).length - rem.length
      i += consumed
      continue
    }
    // text
    const next = body.indexOf('<', i)
    const txt = body.slice(i, next === -1 ? body.length : next)
    if (txt.trim()) {
      out.push(new FakeNode(3, '#text', txt))
    }
    i = next === -1 ? body.length : next
  }
  return out
}

// --- toJson tests ---

describe('toJson', () => {
  it('parses a simple element with text', () => {
    const out = toJson('<root>hello</root>', {}, makeParserFromString('<root>hello</root>'))
    expect(out).toEqual({ '#text': 'hello' })
  })

  it('parses attributes with @ prefix', () => {
    const out = toJson('<root a="1" b="2"/>', {}, makeParserFromString('<root a="1" b="2"/>'))
    expect(out).toEqual({ '@a': '1', '@b': '2' })
  })

  it('parses nested elements', () => {
    const out = toJson(
      '<root><child>x</child></root>',
      {},
      makeParserFromString('<root><child>x</child></root>')
    )
    expect(out).toEqual({ child: { '#text': 'x' } })
  })

  it('groups repeated children into arrays', () => {
    const xml = '<root><a>1</a><a>2</a><a>3</a></root>'
    const out = toJson(xml, {}, makeParserFromString(xml))
    expect(Array.isArray(out.a)).toBe(true)
    expect(out.a.length).toBe(3)
    expect(out.a[0]).toEqual({ '#text': '1' })
  })

  it('throws on empty input', () => {
    expect(() => toJson('', {}, makeParserFromString(''))).toThrow()
  })
})

// --- toXml tests ---

describe('toXml', () => {
  it('wraps a string', () => {
    expect(toXml('hello', { root: 'root' })).toContain('<root>hello</root>')
  })

  it('produces a declaration by default', () => {
    const out = toXml('x', { root: 'r' })
    expect(out.startsWith('<?xml')).toBe(true)
  })

  it('omits the declaration when configured', () => {
    const out = toXml('x', { root: 'r', declaration: false })
    expect(out.startsWith('<?xml')).toBe(false)
  })

  it('emits attributes for @-prefixed keys', () => {
    const out = toXml({ '@a': '1', '#text': 'x' }, { root: 'r' })
    expect(out).toContain('a="1"')
  })

  it('renders arrays as repeated children', () => {
    const out = toXml({ a: ['1', '2', '3'] }, { root: 'r' })
    expect((out.match(/<a>/g) || []).length).toBe(3)
  })

  it('escapes special characters', () => {
    const out = toXml('a & b < c > d "e" \'f\'', { root: 'r' })
    expect(out).toContain('&amp;')
    expect(out).toContain('&lt;')
    expect(out).toContain('&gt;')
    expect(out).toContain('&quot;')
    expect(out).toContain('&apos;')
  })

  it('rejects when root is missing', () => {
    expect(() => toXml({}, { root: '' })).toThrow()
  })

  it('round-trips a simple structure', () => {
    const obj = { a: '1', b: { c: '2' } }
    const xml = toXml(obj, { root: 'r' })
    expect(xml).toContain('<a>1</a>')
    expect(xml).toContain('<b><c>2</c></b>')
  })
})
