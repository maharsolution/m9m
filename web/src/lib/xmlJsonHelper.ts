// XML ↔ JSON helper for BUG-CAP-2026-09-09-005.
//
// m9m ships an XML node (`n8n-nodes-base.xml`) for
// in-workflow XML handling, but the UI never exposed a
// "Convert XML ↔ JSON" affordance. This module is the
// pure-function helper the helper-panel uses; it can
// be invoked from a node form, a free-form playground
// (Settings → Tools), or a code cell.
//
// The format conventions match the XML node:
//   - `toJson` produces a flat-ish map: each element
//     becomes a key, with attributes under `@<name>` and
//     children under `#text` / an array. This matches
//     n8n's `xmlToJson` behaviour for simple cases.
//   - `toXml` produces a single-rooted XML string. If
//     the input has no root, an error is returned.

export interface XmlToJsonOptions {
  // Include XML attributes as `@<name>` keys. Default true.
  includeAttributes?: boolean
  // Text content of an element-with-children goes under
  // `#text`. Default true.
  includeText?: boolean
  // Trim whitespace from text content. Default true.
  trimText?: boolean
}

/**
 * Read the i-th entry of a DOM `NodeList`/`NamedNodeMap`.
 *
 * The browser exposes `item(i)`, but a plain array (as used by the
 * hermetic test doubles) only supports index access — both shapes are
 * accepted so the helper works with either.
 */
function childAt(nodes: NodeListOf<ChildNode> | ChildNode[], index: number): ChildNode | null {
  const list = nodes as NodeListOf<ChildNode> & { item?: (i: number) => ChildNode | null }
  if (typeof list.item === 'function') return list.item(index)
  return (nodes as ChildNode[])[index] ?? null
}

/** Same as {@link childAt} but for attribute maps. */
function attrAt(
  attributes: NamedNodeMap | Attr[],
  index: number,
): Attr | null {
  const list = attributes as NamedNodeMap & { item?: (i: number) => Attr | null }
  if (typeof list.item === 'function') return list.item(index)
  return (attributes as Attr[])[index] ?? null
}

// toJson is a browser-friendly XML→JSON converter.
// Uses `DOMParser` in the browser; tests inject a
// parser via the `parse` parameter. The parser MUST
// return a `Document` (or any object with a
// `documentElement: Element | null` field).
export function toJson(
  xml: string,
  options: XmlToJsonOptions = {},
  parse: (xml: string) => { documentElement: Element | null } = defaultParse
): Record<string, unknown> {
  const includeAttributes = options.includeAttributes !== false
  const includeText = options.includeText !== false
  const trimText = options.trimText !== false
  const doc = parse(xml)
  const root = doc.documentElement
  if (!root) {
    throw new Error('xmlJsonHelper: no root element')
  }
  return elementToObject(root, includeAttributes, includeText, trimText)
}

function elementToObject(
  el: Element,
  includeAttributes: boolean,
  includeText: boolean,
  trimText: boolean
): Record<string, unknown> {
  const out: Record<string, unknown> = {}

  // Attributes.
  if (includeAttributes && el.attributes) {
    for (let i = 0; i < el.attributes.length; i++) {
      const a = attrAt(el.attributes, i)
      if (!a) continue
      // A DOM `Attr` exposes `name`/`value`; a DOM-less parser may only
      // provide the node name/value pair.
      const name = a.name ?? (a as unknown as { nodeName?: string }).nodeName
      if (!name) continue
      const value = a.value ?? (a as unknown as { nodeValue?: string | null }).nodeValue ?? ''
      out[`@${name}`] = value
    }
  }

  // Children.
  const childElements: Element[] = []
  let textContent = ''
  const childNodes = el.childNodes
  for (let i = 0; i < childNodes.length; i++) {
    const n = childAt(childNodes, i)
    if (!n) continue
    if (n.nodeType === 3 /* TEXT */) {
      textContent += n.nodeValue ?? ''
    } else if (n.nodeType === 1 /* ELEMENT */) {
      childElements.push(n as Element)
    }
  }

  for (const child of childElements) {
    const childObj = elementToObject(child, includeAttributes, includeText, trimText)
    const existing = out[child.nodeName]
    if (existing === undefined) {
      out[child.nodeName] = childObj
    } else if (Array.isArray(existing)) {
      existing.push(childObj)
    } else {
      out[child.nodeName] = [existing, childObj]
    }
  }

  if (includeText && textContent.trim() !== '') {
    const text = trimText ? textContent.trim() : textContent
    if (Object.keys(out).length === 0) {
      // Element had only text — store as the element's value.
      return { '#text': text }
    }
    out['#text'] = text
  }

  return out
}

export interface JsonToXmlOptions {
  // Root element name. Required.
  root: string
  // Whether to emit XML declaration. Default true.
  declaration?: boolean
  // Indent string (e.g. "  "). Default "" (no indent).
  indent?: string
}

// toJson's inverse. Produces a single-rooted XML
// document. Whitespace / newlines are preserved per
// `indent`.
export function toXml(
  obj: Record<string, unknown>,
  options: JsonToXmlOptions
): string {
  if (!options.root) {
    throw new Error('xmlJsonHelper: root element name is required')
  }
  const body = objectToXml(obj, options.root, 0, options.indent ?? '')
  if (options.declaration === false) return body
  return '<?xml version="1.0" encoding="UTF-8"?>\n' + body
}

function objectToXml(value: unknown, tag: string, depth: number, indent: string): string {
  const pad = indent.repeat(depth)
  const closePad = indent.repeat(Math.max(0, depth - 1))
  // Without an indent string the output stays compact — the same shape
  // n8n's XML node produces (`<root><a>1</a></root>`). A non-empty
  // indent switches to the pretty, newline separated form.
  const pretty = indent.length > 0
  if (value === null || value === undefined) {
    return `${pad}<${tag}/>`
  }
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return `${pad}<${tag}>${escape(String(value))}</${tag}>`
  }
  if (Array.isArray(value)) {
    return value.map((v) => objectToXml(v, tag, depth + 1, indent)).join(pretty ? '\n' : '')
  }
  if (typeof value === 'object') {
    const parts: string[] = []
    const attrs: string[] = []
    let text = ''
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      if (k.startsWith('@')) {
        attrs.push(`${k.slice(1)}="${escape(String(v))}"`)
      } else if (k === '#text') {
        text = String(v)
      } else if (Array.isArray(v)) {
        for (const item of v) {
          parts.push(objectToXml(item, k, depth + 1, indent))
        }
      } else {
        parts.push(objectToXml(v, k, depth + 1, indent))
      }
    }
    const attrStr = attrs.length ? ' ' + attrs.join(' ') : ''
    if (parts.length === 0) {
      return text
        ? `${pad}<${tag}${attrStr}>${escape(text)}</${tag}>`
        : `${pad}<${tag}${attrStr}/>`
    }
    if (!pretty) {
      return `${pad}<${tag}${attrStr}>${parts.join('')}</${tag}>`
    }
    return `${pad}<${tag}${attrStr}>\n${parts.join('\n')}\n${closePad}</${tag}>`
  }
  return `${pad}<${tag}>${escape(String(value))}</${tag}>`
}

function escape(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

// defaultParse uses the browser's DOMParser. In Node
// tests (vitest) we inject a JSDOM-backed parser —
// for now the test passes a string-built fake.
function defaultParse(xml: string): { documentElement: Element | null } {
  if (typeof DOMParser === 'undefined') {
    throw new Error('xmlJsonHelper: no DOMParser; inject one via the parse parameter')
  }
  const doc: Document = new DOMParser().parseFromString(xml, 'text/xml')
  if (doc.getElementsByTagName('parsererror').length > 0) {
    throw new Error('xmlJsonHelper: malformed XML')
  }
  return doc
}
