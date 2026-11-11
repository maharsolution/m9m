package file

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// readBinaryProperties describes the Read Binary File node.
//
// file.ReadBinaryFileNode.Execute reads `filePath`, `encoding` and
// `includeHashes` — the old `fileFormat` / `propertyName` fields were
// never read (the binary payload is always attached under the "data"
// binary key).
func readBinaryProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("File path", "filePath", "",
			"Path of the file to read. Must sit inside one of the engine's allowed directories.",
			"/data/input.bin", true),
		nodeprops.Opt("Encoding", "encoding", "binary",
			"How the file content is encoded in the output item.",
			[]base.Option{
				{Name: "Binary", Value: "binary"},
				{Name: "Base64", Value: "base64"},
				{Name: "Hex", Value: "hex"},
				{Name: "UTF-8", Value: "utf8"},
				{Name: "Text", Value: "text"},
			}, false),
		nodeprops.Bool("Include hashes", "includeHashes", false,
			"Add MD5/SHA1/SHA256 hashes of the file to the output item."),
	}
	return append(props, nodeprops.Settings()...)
}

// writeBinaryProperties describes the Write Binary File node.
//
// file.WriteBinaryFileNode.Execute reads `filePath`, `dataSource`
// (binary | json | expression), `binaryPropertyName`, `dataField`,
// `dataExpression` and `encoding`. The three source-specific fields
// are mutually exclusive, so each one is only shown for its own
// dataSource value.
func writeBinaryProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("File path", "filePath", "",
			"Path the data is written to. Must sit inside one of the engine's allowed directories.",
			"/data/output.bin", true),
		nodeprops.Opt("Data source", "dataSource", "binary",
			"Where the bytes to write come from.",
			[]base.Option{
				{Name: "Binary data", Value: "binary"},
				{Name: "JSON field", Value: "json"},
				{Name: "Expression", Value: "expression"},
			}, true),
		nodeprops.Str("Binary property name", "binaryPropertyName", "data",
			"Key of the binary property on the input item that holds the file content.",
			"data", false).
			WithDisplayOptions(nodeprops.Show("dataSource", "binary")),
		nodeprops.Str("JSON field", "dataField", "data",
			"Field of the input item that holds the content to write.",
			"data", false).
			WithDisplayOptions(nodeprops.Show("dataSource", "json")),
		nodeprops.Str("Expression", "dataExpression", "",
			"Expression evaluated against the input item; its result is written to the file.",
			"={{ $json.data }}", false).
			WithDisplayOptions(nodeprops.Show("dataSource", "expression")),
		nodeprops.Opt("Encoding", "encoding", "auto",
			"How a string payload is decoded before it is written.",
			[]base.Option{
				{Name: "Auto", Value: "auto"},
				{Name: "UTF-8", Value: "utf8"},
				{Name: "Text", Value: "text"},
				{Name: "Base64", Value: "base64"},
				{Name: "Hex", Value: "hex"},
			}, false),
	}
	return append(props, nodeprops.Settings()...)
}
