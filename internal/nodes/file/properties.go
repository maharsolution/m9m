package file

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the file nodes.

func readBinaryProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringProp("File Path", "filePath", "", "Path of the file to read.", "/data/input.bin", true),
		base.StringOpt("File Format", "fileFormat", "autodetect", "How to interpret the file.", []base.Option{
			{Name: "Autodetect", Value: "autodetect"},
			{Name: "Text", Value: "text"},
			{Name: "Base64", Value: "base64"},
		}, true),
		base.StringProp("Property Name", "propertyName", "data", "Property to attach the binary data to on the output item.", "data", false),
	}
	return append(props, base.CommonSettings()...)
}

func writeBinaryProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringProp("File Name", "fileName", "", "Output file name (or path).", "output.bin", true),
		base.StringProp("Property Name", "propertyName", "data", "Source property on the input item (binary).", "data", true),
		base.StringOpt("File Format", "fileFormat", "autodetect", "How to encode the file.", []base.Option{
			{Name: "Autodetect", Value: "autodetect"},
			{Name: "Text", Value: "text"},
			{Name: "Base64", Value: "base64"},
		}, true),
	}
	return append(props, base.CommonSettings()...)
}
