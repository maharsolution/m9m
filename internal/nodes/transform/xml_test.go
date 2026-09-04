package transform

import (
	"strings"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newXmlInput(payload string) []model.DataItem {
	return []model.DataItem{
		{JSON: map[string]interface{}{"data": payload}},
	}
}

// TestXmlNode_XMLToJSON_Simple pins the contract for the simple
// "XML to JSON" case: each child element becomes a key in the resulting
// JSON object, and attributes are surfaced under the configured prefix
// (default "@"). This is the canonical shape m9m's webhook_xml test
// workflow expects from the server.
func TestXmlNode_XMLToJSON_Simple(t *testing.T) {
	node := NewXmlNode()
	in := newXmlInput(`<buku><judul>Belajar</judul><penulis>Anon</penulis></buku>`)

	out, err := node.Execute(in, map[string]interface{}{"mode": xmlModeXMLToJSON})
	require.NoError(t, err)
	require.Len(t, out, 1)

	root, ok := out[0].JSON["buku"].(map[string]interface{})
	require.True(t, ok, "expected root element to be a map, got %T", out[0].JSON["buku"])
	assert.Equal(t, "Belajar", root["judul"])
	assert.Equal(t, "Anon", root["penulis"])
}

// TestXmlNode_XMLToJSON_Attributes mirrors the spreadsheet's webhook_xml
// case: <harga mataUang="IDR">120000</harga> must surface mataUang as
// an attribute-prefixed key and 120000 as character data, because the
// element has mixed content.
func TestXmlNode_XMLToJSON_Attributes(t *testing.T) {
	node := NewXmlNode()
	in := newXmlInput(
		`<?xml version="1.0" encoding="UTF-8"?>` +
			`<buku id="001">` +
			`<judul>Belajar Pemrograman Web</judul>` +
			`<penulis>John Doe</penulis>` +
			`<tahun>2026</tahun>` +
			`<harga mataUang="IDR">120000</harga>` +
			`</buku>`,
	)

	out, err := node.Execute(in, map[string]interface{}{"mode": xmlModeXMLToJSON})
	require.NoError(t, err)

	root, ok := out[0].JSON["buku"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "001", root["@id"])
	assert.Equal(t, "Belajar Pemrograman Web", root["judul"])
	assert.Equal(t, "John Doe", root["penulis"])
	assert.Equal(t, "2026", root["tahun"])

	harga, ok := root["harga"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "IDR", harga["@mataUang"])
	assert.Equal(t, "120000", harga["_text"])
}

// TestXmlNode_XMLToJSON_Malformed covers the failure path: bad XML
// must surface as a node-level error rather than a panic, so the
// engine can wrap it into a webhook execution failure.
func TestXmlNode_XMLToJSON_Malformed(t *testing.T) {
	node := NewXmlNode()
	in := newXmlInput(`<not closed`)

	_, err := node.Execute(in, map[string]interface{}{"mode": xmlModeXMLToJSON})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "XML")
}

// TestXmlNode_JSONToXML_RoundTrip exercises the inverse mode. It takes
// the JSON shape produced by the n8n "XML to JSON" node (attributes as
// "@"-prefixed keys, character data under "_text") and confirms the
// round-trip emits well-formed XML that contains the original values.
//
// The wire-shape comparison against n8n's response lives in
// scripts/test-parity-sheet.py — this unit test just pins the Go
// implementation.
func TestXmlNode_JSONToXML_RoundTrip(t *testing.T) {
	node := NewXmlNode()
	in := []model.DataItem{
		{JSON: map[string]interface{}{
			"buku": map[string]interface{}{
				"judul":   "Belajar Pemrograman Web",
				"penulis": "John Doe",
				"tahun":   "2026",
				"harga": map[string]interface{}{
					"@mataUang": "IDR",
					"_text":     "120000",
				},
			},
		}},
	}

	out, err := node.Execute(in, map[string]interface{}{"mode": xmlModeJSONToXML})
	require.NoError(t, err)
	require.Len(t, out, 1)
	xmlStr, ok := out[0].JSON["data"].(string)
	require.True(t, ok, "expected string in data key, got %T", out[0].JSON["data"])

	// Sanity check: the output must be a valid XML declaration followed
	// by a <root> envelope that contains a <buku> sub-element with our
	// values somewhere inside.
	assert.True(t, strings.HasPrefix(xmlStr, "<?xml"), "expected XML declaration, got %q", xmlStr)
	assert.Contains(t, xmlStr, "Belajar Pemrograman Web")
	assert.Contains(t, xmlStr, "John Doe")
	assert.Contains(t, xmlStr, "2026")
	assert.Contains(t, xmlStr, "IDR")
	assert.Contains(t, xmlStr, "120000")
}

// TestXmlNode_ValidateParameters confirms that invalid modes are rejected
// at validation time (which the engine calls before Execute) and that
// the default-mode contract holds: n8n exports that omit `mode` entirely
// must validate cleanly.
func TestXmlNode_ValidateParameters(t *testing.T) {
	node := NewXmlNode()

	t.Run("missing mode is OK", func(t *testing.T) {
		require.NoError(t, node.ValidateParameters(map[string]interface{}{}))
	})

	t.Run("explicit xmlToJson is OK", func(t *testing.T) {
		require.NoError(t, node.ValidateParameters(map[string]interface{}{"mode": xmlModeXMLToJSON}))
	})

	t.Run("explicit jsonToXml is OK", func(t *testing.T) {
		require.NoError(t, node.ValidateParameters(map[string]interface{}{"mode": xmlModeJSONToXML}))
	})

	t.Run("invalid mode is rejected", func(t *testing.T) {
		err := node.ValidateParameters(map[string]interface{}{"mode": "garbage"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported XML mode")
	})

	t.Run("nil params is OK", func(t *testing.T) {
		require.NoError(t, node.ValidateParameters(nil))
	})
}

// TestXmlNode_Description pins the metadata so the registered name in
// the engine shows up as "XML" / "Data Transformation" — matches the
// n8n UI label that the operator expects to see in logs.
func TestXmlNode_Description(t *testing.T) {
	node := NewXmlNode()
	d := node.Description()
	assert.Equal(t, "XML", d.Name)
	assert.Equal(t, "Data Transformation", d.Category)
}
