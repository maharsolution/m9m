package azure

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the Azure Blob Storage node.

func blobStorageProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Get", Value: "get"},
		{Name: "Put", Value: "put"},
		{Name: "Delete", Value: "delete"},
		{Name: "List", Value: "list"},
		{Name: "SAS URL", Value: "sasUrl"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Operation", "operation", "get", "Blob operation to perform.", ops, true),
		base.StringProp("Container", "container", "", "Blob container name.", "", true),
		base.StringProp("Blob Name", "blobName", "", "Blob name (path within the container).", "", false),
		base.StringProp("Account Name", "accountName", "", "Storage account name (overrides the credential).", "", false),
		base.StringProp("Content Type", "contentType", "", "Override the Content-Type (put).", "", false),
		base.StringProp("Prefix", "prefix", "", "Blob prefix filter (list only).", "", false),
		base.StringProp("SAS Expiry (h)", "sasExpiryHours", "1", "SAS URL validity in hours (1-8760).", "1", false),
	}
	return append(props, base.CommonSettings()...)
}
