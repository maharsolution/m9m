package transform

import (
	"strings"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestXmlNode_WebhookRoundTrip mirrors the live `/webhook/webhook_xml`
// payload from the spreadsheet, runs the same flow (XML→JSON then
// JSON→XML) the deployed workflow uses, and confirms the round-trip
// contains the original values in a recognisable XML structure.
//
// The full wire-shape comparison against the n8n baseline lives in
// scripts/test-parity-sheet.py; this test just pins the local
// implementation's behaviour so future refactors don't silently
// regress the webhook test.
func TestXmlNode_WebhookRoundTrip(t *testing.T) {
	node := NewXmlNode()
	in := []model.DataItem{
		{JSON: map[string]interface{}{"data": strings.Join([]string{
			`<?xml version="1.0" encoding="UTF-8"?>`,
			`<buku id="001">`,
			`<judul>Belajar Pemrograman Web</judul>`,
			`<penulis>John Doe</penulis>`,
			`<tahun>2026</tahun>`,
			`<harga mataUang="IDR">120000</harga>`,
			`</buku>`,
		}, "")}},
	}

	// XML → JSON
	parsed, err := node.Execute(in, map[string]interface{}{"mode": xmlModeXMLToJSON})
	require.NoError(t, err)
	require.Len(t, parsed, 1)

	root, ok := parsed[0].JSON["buku"].(map[string]interface{})
	require.True(t, ok, "expected root key 'buku' to be a map")
	assert.Equal(t, "001", root["@id"])
	assert.Equal(t, "Belajar Pemrograman Web", root["judul"])

	harga, ok := root["harga"].(map[string]interface{})
	require.True(t, ok, "expected 'harga' to remain a map because of mixed content")
	assert.Equal(t, "IDR", harga["@mataUang"])
	assert.Equal(t, "120000", harga["_text"])

	// JSON → XML
	emitted, err := node.Execute(parsed, map[string]interface{}{"mode": xmlModeJSONToXML})
	require.NoError(t, err)
	require.Len(t, emitted, 1)

	out, ok := emitted[0].JSON["data"].(string)
	require.True(t, ok, "expected JSON→XML output to land under 'data' as a string")

	assert.True(t, strings.HasPrefix(out, "<?xml"))
	assert.Contains(t, out, "<buku")
	// Attribute order is sorted — id is the only attribute on <buku>.
	assert.Contains(t, out, `id="001"`)
	assert.Contains(t, out, "<judul>Belajar Pemrograman Web</judul>")
	assert.Contains(t, out, "<penulis>John Doe</penulis>")
	assert.Contains(t, out, "<tahun>2026</tahun>")
	assert.Contains(t, out, "<harga")
	assert.Contains(t, out, `mataUang="IDR"`)
	assert.Contains(t, out, "120000")
}
