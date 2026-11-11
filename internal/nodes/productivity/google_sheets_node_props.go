package productivity

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// googleSheetsProperties describes the Google Sheets node.
//
// productivity.GoogleSheetsNode reads `resource` (sheet |
// spreadsheet), `operation` (read / append / update / clear for
// sheets, create / get for spreadsheets), `spreadsheetId`, `range`,
// `title` and `values`. The fields that only apply to one resource —
// and, for `values`, to one operation — are shown conditionally.
func googleSheetsProperties() []base.NodeProperty {
	resources := []base.Option{
		{Name: "Sheet", Value: "sheet"},
		{Name: "Spreadsheet", Value: "spreadsheet"},
	}
	ops := []base.Option{
		{Name: "Read rows", Value: "read"},
		{Name: "Append rows", Value: "append"},
		{Name: "Update rows", Value: "update"},
		{Name: "Clear sheet", Value: "clear"},
		{Name: "Create spreadsheet", Value: "create"},
		{Name: "Get spreadsheet", Value: "get"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Resource", "resource", "sheet",
			"Whether the node works on a sheet inside a spreadsheet or on the spreadsheet itself.",
			resources, true),
		nodeprops.Opt("Operation", "operation", "read",
			"What to do with the selected resource.",
			ops, true),
		spreadsheetIDProperty().
			WithDisplayOptions(nodeprops.Show("resource", "sheet")),
		nodeprops.Str("Range", "range", "A:Z",
			"A1-notation range of the sheet (empty = all data).",
			"Sheet1!A1:Z1000", false).
			WithDisplayOptions(nodeprops.Show("resource", "sheet")),
		nodeprops.Str("Title", "title", "New Spreadsheet",
			"Title of the spreadsheet that is created.",
			"New Spreadsheet", false).
			WithDisplayOptions(nodeprops.Show("resource", "spreadsheet", "operation", "create")),
		nodeprops.JSON("Values", "values", "[]",
			"Rows written to the sheet (array of arrays or array of objects).").
			WithDisplayOptions(nodeprops.Show("resource", "sheet", "operation", "append", "update")),
	}
	return append(props, nodeprops.Settings()...)
}

// spreadsheetIDProperty describes the "which spreadsheet" parameter.
//
// It is a resource locator (n8n's `documentId`): the panel stores
// `{__rl: true, value, mode}` and the `loadSpreadsheets` action fills
// the picker with the spreadsheets of the connected Google account.
// The plain id stays valid, so workflows that were saved before the
// locator existed keep working.
func spreadsheetIDProperty() base.NodeProperty {
	return base.NodeProperty{
		DisplayName: "Spreadsheet ID",
		Name:        "spreadsheetId",
		Type:        "resourceLocator",
		Default:     "",
		Description: "ID of the spreadsheet (the long id in the sheet URL).",
		Required:    true,
		Placeholder: "1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms",
		Options: []base.Option{
			{
				Name:        "From list",
				Value:       "list",
				Action:      "loadSpreadsheets",
				Description: "Pick a spreadsheet from the connected Google account.",
			},
			{Name: "Enter ID", Value: "id"},
		},
		TypeOptions: map[string]interface{}{
			"modes": []map[string]interface{}{
				{"displayName": "From list", "name": "list", "value": "list"},
				{"displayName": "Enter ID", "name": "id", "value": "id"},
			},
		},
	}
}
