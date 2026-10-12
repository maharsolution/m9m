package productivity

import "github.com/neul-labs/m9m/internal/nodes/base"

// googleSheetsProperties returns the Google Sheets node's parameter
// descriptors. BUG-UI-2026-09-09-001 — populates Properties so the
// Settings tab renders the structured form for a node that was
// previously only accessible via raw parameters JSON.
func googleSheetsProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Read Rows", Value: "read"},
		{Name: "Write Rows", Value: "write"},
		{Name: "Append Rows", Value: "append"},
		{Name: "Update Row", Value: "update"},
		{Name: "Clear", Value: "clear"},
		{Name: "Create Sheet", Value: "createSheet"},
		{Name: "Delete Sheet", Value: "deleteSheet"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Operation", "operation", "read", "What to do with the sheet.", ops, true),
		base.StringProp("Spreadsheet ID", "spreadsheetId", "", "Google Sheets spreadsheet ID (the long id in the URL).", "", true),
		base.StringProp("Sheet Name", "sheetName", "", "Sheet (tab) name within the spreadsheet.", "Sheet1", true),
		base.StringProp("Range", "range", "", "A1-notation range (e.g. A1:D10). Empty = all data.", "A1:Z1000", false),
		base.JsonProp("Data", "data", "[]", "Row data (array of arrays or array of objects)."),
		base.StringProp("Value Render", "valueRenderOption", "UNFORMATTED_VALUE", "How cell values render (FORMATTED_VALUE / UNFORMATTED_VALUE / FORMULA).", "UNFORMATTED_VALUE", false),
		base.StringProp("Insert Mode", "insertDataOption", "INSERT_ROWS", "OVERWRITE or INSERT_ROWS for write/append.", "INSERT_ROWS", false),
		base.BoolProp("Header Row", "headerRow", true, "Treat the first row as a header when writing."),
	}
	return append(props, base.CommonSettings()...)
}
