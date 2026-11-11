package azure

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// blobStorageProperties describes the Azure Blob Storage node.
//
// azure.BlobStorageNode reads `operation`, `container`, `blob`,
// `source`, `destination`, `prefix`, `contentType`, `accessTier`,
// `metadata`, `accountName`, `accountKey`, `sasToken`,
// `connectionString`, `tenantId`, `clientId`, `clientSecret`,
// `useManagedIdentity` and `options`. The blob-level fields are
// shown for the operations that touch a blob.
func blobStorageProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Upload blob", Value: "upload"},
		{Name: "Download blob", Value: "download"},
		{Name: "Delete blob", Value: "delete"},
		{Name: "List blobs", Value: "list"},
		{Name: "Copy blob", Value: "copy"},
		{Name: "Get blob properties", Value: "get_properties"},
		{Name: "Set blob properties", Value: "set_properties"},
		{Name: "Get blob metadata", Value: "get_metadata"},
		{Name: "Set blob metadata", Value: "set_metadata"},
		{Name: "Create container", Value: "create_container"},
		{Name: "Delete container", Value: "delete_container"},
		{Name: "List containers", Value: "list_containers"},
		{Name: "Acquire lease", Value: "acquire_lease"},
		{Name: "Renew lease", Value: "renew_lease"},
		{Name: "Release lease", Value: "release_lease"},
		{Name: "Break lease", Value: "break_lease"},
	}
	blobOps := nodeprops.Show("operation", "upload", "download", "delete", "copy", "get_properties", "set_properties", "get_metadata", "set_metadata")
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "upload",
			"Blob operation to run.",
			ops, true),
		nodeprops.Str("Container", "container", "",
			"Blob container name.",
			"my-container", true),
		nodeprops.Str("Blob name", "blob", "",
			"Blob name (path inside the container).",
			"folder/report.csv", false).
			WithDisplayOptions(blobOps),
		nodeprops.Str("Source", "source", "",
			"Expression or field holding the bytes to upload.",
			"={{ $json.data }}", false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Str("Destination", "destination", "",
			"Destination blob path (copy operation).",
			"folder/copy.csv", false).
			WithDisplayOptions(nodeprops.Show("operation", "copy")),
		nodeprops.Str("Prefix", "prefix", "",
			"Blob prefix used to filter the listing.",
			"reports/", false).
			WithDisplayOptions(nodeprops.Show("operation", "list")),
		nodeprops.Str("Content type", "contentType", "",
			"Content-Type of the stored blob.",
			"text/csv", false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Opt("Access tier", "accessTier", "Hot",
			"Access tier of the blob.",
			[]base.Option{
				{Name: "Hot", Value: "Hot"},
				{Name: "Cool", Value: "Cool"},
				{Name: "Cold", Value: "Cold"},
				{Name: "Archive", Value: "Archive"},
			}, false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.JSON("Metadata", "metadata", "{}",
			"Blob metadata (name → value).").
			WithDisplayOptions(nodeprops.Show("operation", "set_metadata")),
		nodeprops.Str("Account name", "accountName", "",
			"Storage account name (overrides the credential).",
			"mystorageaccount", false),
		nodeprops.Secret("Account key", "accountKey", "",
			"Storage account access key.",
			"", false),
		nodeprops.Secret("SAS token", "sasToken", "",
			"Shared access signature token.",
			"", false),
		nodeprops.Secret("Connection string", "connectionString", "",
			"Full storage connection string.",
			"", false),
		nodeprops.Str("Tenant ID", "tenantId", "",
			"Microsoft Entra tenant ID (service principal auth).",
			"00000000-0000-0000-0000-000000000000", false),
		nodeprops.Str("Client ID", "clientId", "",
			"Microsoft Entra client ID (service principal auth).",
			"00000000-0000-0000-0000-000000000000", false),
		nodeprops.Secret("Client secret", "clientSecret", "",
			"Microsoft Entra client secret.",
			"", false),
		nodeprops.Bool("Use managed identity", "useManagedIdentity", false,
			"Authenticate with the host's managed identity instead of a key."),
		nodeprops.JSON("Advanced options", "options", "{}",
			"Extra operation options passed through to the Azure SDK call."),
	}
	return append(props, nodeprops.Settings()...)
}
