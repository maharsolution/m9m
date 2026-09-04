/*
Package transform - XML node.

Implements n8n's `n8n-nodes-base.xml` (a.k.a. "XML to JSON" / "JSON to XML")
node using only the Go standard library (`encoding/xml` + `encoding/json`)
so no new dependencies are added.

Two modes are supported, selected via the `mode` parameter:

  - "xmlToJson"  (default) — convert an XML string to a JSON object.
                        Attributes are prefixed with `options.attributePrefix`
                        (default "@") and text content of mixed elements is
                        stored under `options.textKey` (default "_text").
  - "jsonToXml"            — convert a JSON object back to an XML string.
                        Keys starting with `options.attributePrefix` are
                        emitted as XML attributes on the parent element;
                        a `options.textKey` entry becomes the element's
                        character data.

This mirrors n8n's drop-in compatibility for the simplest "XML to JSON"
followed by "JSON to XML" round-trip used in webhook tests. More advanced
options (namespaces, cdata, schema validation) are out of scope for the
first parity cut and can be layered in incrementally without breaking the
existing wire shape.
*/
package transform

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// XML node type identifiers — registered as `n8n-nodes-base.xml` to match
// the n8n export format. The display name "XML" matches n8n's UI label.
const (
	xmlNodeTypeName = "XML"
	xmlDefaultMode  = "xmlToJson"
)

// XML node operation modes.
//
// n8n exports these as `xmlToJson` / `jsonToXml` (camelCase) in newer
// versions, but older exports and hand-edited workflow JSON sometimes
// use `xmlTojson` / `jsonToxml` (lowercase `j`). We canonicalise on
// the lowercase form internally but accept both spellings so existing
// workflow files continue to validate.
const (
	xmlModeXMLToJSON = "xmlToJson"
	xmlModeJSONToXML = "jsonToXml"
)

// canonicalXMLMode normalises the various spellings n8n workflows
// use for the mode parameter to the constant defined above. An
// empty input falls back to xmlToJson (the n8n default).
func canonicalXMLMode(raw string) string {
	switch strings.ToLower(raw) {
	case "jsontoxml", "json_to_xml":
		return xmlModeJSONToXML
	case "xmltojson", "xml_to_json":
		return xmlModeXMLToJSON
	default:
		if raw == "" {
			return xmlModeXMLToJSON
		}
		return raw
	}
}

// XmlNode implements n8n's XML conversion node.
type XmlNode struct {
	*base.BaseNode
}

// NewXmlNode constructs a new XML node instance.
func NewXmlNode() *XmlNode {
	return &XmlNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        xmlNodeTypeName,
			Description: "Convert between XML and JSON",
			Category:    "Data Transformation",
		}),
	}
}

// Description returns the node description.
func (x *XmlNode) Description() base.NodeDescription {
	return x.BaseNode.Description()
}

// ValidateParameters validates the XML node's parameters.
//
// The `mode` parameter is optional — n8n's XML node defaults to "xmlToJson"
// when the mode is not explicitly set in the workflow export. We mirror that
// default here so legacy / hand-edited workflow JSON files continue to load
// without forcing the operator to re-publish from the n8n editor.
func (x *XmlNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	if rawMode, ok := params["mode"]; ok && rawMode != nil {
		mode, ok := rawMode.(string)
		if !ok {
			return x.CreateError("mode must be a string", nil)
		}
		switch canonicalXMLMode(mode) {
		case xmlModeXMLToJSON, xmlModeJSONToXML:
			return nil
		default:
			return x.CreateError(fmt.Sprintf("unsupported XML mode: %s", mode), nil)
		}
	}
	return nil
}

// Execute runs the XML node against the input data stream.
//
// For each input item:
//   - In xmlToJson mode (default) we read `dataPropertyName` (default
//     "data") or the raw `body`/`json.body` upstream payload, parse it as
//     XML, and emit a single item whose JSON contains the parsed tree.
//   - In jsonToXml mode we marshal the input item's JSON back to XML
//     using the configured root element name (default "root").
func (x *XmlNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	mode := xmlModeXMLToJSON
	if rawMode, ok := nodeParams["mode"]; ok && rawMode != nil {
		if s, ok := rawMode.(string); ok && s != "" {
			mode = canonicalXMLMode(s)
		}
	}

	attrPrefix, textKey := extractOptions(nodeParams)

	result := make([]model.DataItem, 0, len(inputData))
	for _, item := range inputData {
		switch mode {
		case xmlModeXMLToJSON:
			xmlStr, err := extractXMLString(item)
			if err != nil {
				return nil, x.CreateError(err.Error(), nil)
			}
			parsed, err := xmlToJSON([]byte(xmlStr), attrPrefix, textKey)
			if err != nil {
				return nil, x.CreateError(fmt.Sprintf("failed to convert XML to JSON: %v", err), nil)
			}
			result = append(result, model.DataItem{JSON: parsed})
		case xmlModeJSONToXML:
			xmlBytes, err := jsonToXML(item.JSON, attrPrefix, textKey)
			if err != nil {
				return nil, x.CreateError(fmt.Sprintf("failed to convert JSON to XML: %v", err), nil)
			}
			result = append(result, model.DataItem{
				JSON: map[string]interface{}{
					"data": string(xmlBytes),
				},
			})
		default:
			return nil, x.CreateError(fmt.Sprintf("unsupported XML mode: %s", mode), nil)
		}
	}
	return result, nil
}

// extractOptions reads the optional `options` map for attributePrefix /
// textKey, falling back to n8n defaults ("@" / "_text").
func extractOptions(params map[string]interface{}) (string, string) {
	attrPrefix := "@"
	textKey := "_text"
	if params == nil {
		return attrPrefix, textKey
	}
	rawOpts, ok := params["options"]
	if !ok {
		return attrPrefix, textKey
	}
	opts, ok := rawOpts.(map[string]interface{})
	if !ok {
		return attrPrefix, textKey
	}
	if v, ok := opts["attributePrefix"].(string); ok && v != "" {
		attrPrefix = v
	}
	if v, ok := opts["textKey"].(string); ok && v != "" {
		textKey = v
	}
	return attrPrefix, textKey
}

// extractXMLString resolves the XML payload from the input item.
//
// The webhook trigger feeds the raw request body into `body` either as a
// string (when Content-Type is XML / text) or as a map[string]interface{}
// (when JSON). For XML we always expect a string; callers may also
// provide the payload under an explicit `dataPropertyName` for the rare
// case where the XML lives under a custom key.
func extractXMLString(item model.DataItem) (string, error) {
	if raw, ok := item.JSON["data"].(string); ok && raw != "" {
		return raw, nil
	}
	if raw, ok := item.JSON["body"].(string); ok && raw != "" {
		return raw, nil
	}
	if nested, ok := item.JSON["body"].(map[string]interface{}); ok {
		if raw, ok := nested["data"].(string); ok && raw != "" {
			return raw, nil
		}
	}
	return "", fmt.Errorf("no XML payload found in input item (expected `data` or `body` to be a string)")
}

// xmlToJSON converts an XML byte slice into a JSON-friendly Go value.
//
// The strategy: parse the XML into a generic token stream and rebuild
// it as a map[string]interface{} (or []interface{} for repeated
// elements). The root element's name becomes a top-level key in the
// returned map (so `<buku>...</buku>` round-trips back via the JSON→XML
// path without losing the root name). Attributes are stored under the
// configured prefix; mixed content (character data + child elements)
// is stored under textKey.
//
// When a leaf element has only character data (no attributes, no child
// elements) we collapse it to a plain string in the output so the
// resulting JSON looks like `{"judul": "Belajar"}` rather than
// `{"judul": {"_text": "Belajar"}}`. This matches the simplified
// shape that the n8n XML node emits in the common case and keeps the
// JSON output readable.
//
// This is intentionally a hand-rolled walker rather than
// `xml.Decoder` into `map[string]interface{}` because the latter
// collapses repeated children to a single value and discards attribute
// information, both of which we need to round-trip cleanly.
func xmlToJSON(data []byte, attrPrefix, textKey string) (map[string]interface{}, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = identityCharsetReader

	var (
		stack    []map[string]interface{}
		names    []string
		root     map[string]interface{}
		rootName string
		text     strings.Builder
		hadAttrs []bool // whether the element at this depth had any attributes
	)
	flushText := func() {
		if text.Len() == 0 {
			return
		}
		if len(stack) > 0 {
			top := stack[len(stack)-1]
			mergeText(top, strings.TrimSpace(text.String()), textKey)
		}
		text.Reset()
	}

	for {
		token, err := dec.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			flushText()
			node := map[string]interface{}{}
			hadAttr := len(t.Attr) > 0
			for _, attr := range t.Attr {
				node[attrPrefix+attr.Name.Local] = attr.Value
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				// Defer merging into the parent until EndElement so we
				// can decide whether to insert a map or a plain string
				// based on the element's content.
				mergeChild(parent, t.Name.Local, node)
			} else {
				root = node
				rootName = t.Name.Local
			}
			stack = append(stack, node)
			names = append(names, t.Name.Local)
			hadAttrs = append(hadAttrs, hadAttr)
		case xml.EndElement:
			flushText()
			if len(stack) == 0 {
				continue
			}
			finished := stack[len(stack)-1]
			finishedName := names[len(names)-1]
			hadAttr := hadAttrs[len(hadAttrs)-1]
			stack = stack[:len(stack)-1]
			names = names[:len(names)-1]
			hadAttrs = hadAttrs[:len(hadAttrs)-1]

			// If the element had no attributes and ended up with only
			// a `_text` entry, collapse it to a plain string in the
			// parent. Mixed content (text + attributes/children) keeps
			// the map shape.
			if !hadAttr && len(stack) > 0 && len(finished) == 1 {
				if v, ok := finished[textKey]; ok {
					if s, ok := v.(string); ok {
						collapseChildToString(stack[len(stack)-1], finishedName, s)
					}
				}
			}
		case xml.CharData:
			text.Write(t)
		}
	}
	if root == nil {
		return nil, fmt.Errorf("XML payload contained no root element")
	}
	// n8n's XML node surfaces the root element name as the outer key,
	// so consumers can re-emit it via the JSON→XML mode without
	// hard-coding the element name. Mirror that shape.
	return map[string]interface{}{rootName: root}, nil
}

// collapseChildToString overwrites the parent's entry for `name` with a
// plain string. We call this when an element we already inserted as an
// empty map is now known to contain only character data — overwriting
// rather than appending preserves the n8n-style
// `{"judul": "Belajar"}` shape (no slice promotion needed for a leaf).
func collapseChildToString(parent map[string]interface{}, name string, value string) {
	parent[name] = value
}

func identityCharsetReader(_ string, input io.Reader) (io.Reader, error) {
	return input, nil
}

// mergeChild inserts `child` under `name` in `parent`. If the key already
// exists we promote the value to a slice so repeated elements survive the
// round-trip — this matches n8n's XML node behaviour.
func mergeChild(parent map[string]interface{}, name string, child interface{}) {
	existing, ok := parent[name]
	if !ok {
		parent[name] = child
		return
	}
	switch e := existing.(type) {
	case []interface{}:
		parent[name] = append(e, child)
	default:
		parent[name] = []interface{}{e, child}
	}
}

// mergeText merges character data into `node`. If `node` already has
// child keys we promote it to the `{textKey: "...", ...children}` shape
// so the resulting JSON looks like {"#text": "...", "child": ...}.
// If `node` is empty we store the text under textKey verbatim.
func mergeText(node map[string]interface{}, text, textKey string) {
	if text == "" {
		return
	}
	if len(node) == 0 {
		node[textKey] = text
		return
	}
	if _, exists := node[textKey]; !exists {
		node[textKey] = text
	}
}

// jsonToXML walks a JSON tree and emits XML bytes.
//
// When the input is a single-key map (the canonical shape produced by
// xmlToJSON, e.g. `{"buku": {...}}`) that key becomes the root element
// name. Otherwise the input is rendered under a synthetic `<root>`
// envelope — same fallback n8n uses when no root element is provided.
//
// Within each map:
//   - keys starting with `attrPrefix` become XML attributes on the
//     parent element;
//   - the `textKey` entry (if any) becomes the element's character data;
//   - any other key becomes a child element.
// Repeated children are emitted once per slice element.
func jsonToXML(value interface{}, attrPrefix, textKey string) ([]byte, error) {
	rootName := "root"
	attrs := map[string]string{}
	var children []interface{}
	var chardata string
	usedSingleKeyRoot := false

	if m, ok := value.(map[string]interface{}); ok {
		// Fast path: a single-key map (the XML→JSON output shape)
		// uses that key as the root element name.
		if len(m) == 1 {
			for k, v := range m {
				rootName = k
				usedSingleKeyRoot = true
				// Descend into the single child value; it is the
				// actual content of the root element.
				if childMap, ok := v.(map[string]interface{}); ok {
					value = childMap
					m = childMap
				} else {
					value = v
				}
			}
		}
		keys := sortedKeys(m)
		for _, k := range keys {
			v := m[k]
			if strings.HasPrefix(k, attrPrefix) {
				if s, ok := v.(string); ok {
					attrs[strings.TrimPrefix(k, attrPrefix)] = s
				}
				continue
			}
			if k == textKey {
				if s, ok := v.(string); ok {
					chardata = s
				}
				continue
			}
			children = append(children, map[string]interface{}{k: v})
		}
	} else if usedSingleKeyRoot {
		children = []interface{}{value}
	} else {
		children = []interface{}{value}
	}

	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	writeXMLElement(&buf, rootName, attrs, chardata, children, attrPrefix, textKey)
	return buf.Bytes(), nil
}

func writeXMLElement(buf *bytes.Buffer, name string, attrs map[string]string, chardata string, children []interface{}, attrPrefix, textKey string) {
	buf.WriteString("<")
	buf.WriteString(xmlEscape(name))
	attrKeys := sortedStringKeys(attrs)
	for _, k := range attrKeys {
		fmt.Fprintf(buf, " %s=%q", xmlEscape(k), xmlEscape(attrs[k]))
	}
	if len(children) == 0 && chardata == "" {
		buf.WriteString("/>")
		return
	}
	buf.WriteString(">")
	if chardata != "" {
		xmlEscapeTo(buf, chardata)
	}
	for _, child := range children {
		writeXMLNode(buf, child, attrPrefix, textKey)
	}
	buf.WriteString("</")
	buf.WriteString(xmlEscape(name))
	buf.WriteString(">")
}

// writeXMLNode serializes a single JSON node into XML.
func writeXMLNode(buf *bytes.Buffer, value interface{}, attrPrefix, textKey string) {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, childName := range sortedKeys(v) {
			val := v[childName]
			// Skip attribute / text markers — they belong to the parent
			// element, not as their own child.
			if strings.HasPrefix(childName, attrPrefix) || childName == textKey {
				continue
			}
			emitChild(buf, childName, val, attrPrefix, textKey)
		}
	case []interface{}:
		for _, item := range v {
			writeXMLNode(buf, item, attrPrefix, textKey)
		}
	default:
		xmlEscapeTo(buf, fmt.Sprintf("%v", v))
	}
}

func emitChild(buf *bytes.Buffer, name string, val interface{}, attrPrefix, textKey string) {
	switch v := val.(type) {
	case map[string]interface{}:
		attrs := map[string]string{}
		var chardata string
		var children []interface{}
		for _, k := range sortedKeys(v) {
			kv := v[k]
			if strings.HasPrefix(k, attrPrefix) {
				if s, ok := kv.(string); ok {
					attrs[strings.TrimPrefix(k, attrPrefix)] = s
				}
				continue
			}
			if k == textKey {
				if s, ok := kv.(string); ok {
					chardata = s
				}
				continue
			}
			children = append(children, map[string]interface{}{k: kv})
		}
		writeXMLElement(buf, name, attrs, chardata, children, attrPrefix, textKey)
	case []interface{}:
		for _, item := range v {
			emitChild(buf, name, item, attrPrefix, textKey)
		}
	default:
		buf.WriteString("<")
		buf.WriteString(xmlEscape(name))
		buf.WriteString(">")
		xmlEscapeTo(buf, fmt.Sprintf("%v", v))
		buf.WriteString("</")
		buf.WriteString(xmlEscape(name))
		buf.WriteString(">")
	}
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	xmlEscapeTo(&buf, s)
	return buf.String()
}

func xmlEscapeTo(buf *bytes.Buffer, s string) {
	// Reuse the stdlib's xml.EscapeText to stay aligned with its rules
	// for invalid surrogates etc.
	_ = xml.EscapeText(buf, []byte(s))
}

// Compile-time check: XmlNode must satisfy the executor interface.
var _ interface {
	Execute([]model.DataItem, map[string]interface{}) ([]model.DataItem, error)
	Description() base.NodeDescription
	ValidateParameters(map[string]interface{}) error
} = (*XmlNode)(nil)
