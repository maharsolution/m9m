package gcp

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// gcsProperties describes the Google Cloud Storage node.
//
// gcp.CloudStorageNode reads `operation`, `bucket`, `object`,
// `source`, `destination`, `prefix`, `delimiter`, `contentType`,
// `storageClass`, `versioning`, `metadata`, `projectId`, `keyFile`,
// `keyFileContent`, `serviceAccountEmail`, `useADC` and `options`.
// The object-level fields are shown for the operations that touch an
// object.
func gcsProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Upload object", Value: "upload"},
		{Name: "Download object", Value: "download"},
		{Name: "Delete object", Value: "delete"},
		{Name: "List objects", Value: "list"},
		{Name: "Copy object", Value: "copy"},
		{Name: "Move object", Value: "move"},
		{Name: "Get object metadata", Value: "get_metadata"},
		{Name: "Set object metadata", Value: "set_metadata"},
		{Name: "Create bucket", Value: "create_bucket"},
		{Name: "Delete bucket", Value: "delete_bucket"},
		{Name: "List buckets", Value: "list_buckets"},
		{Name: "Get bucket", Value: "get_bucket"},
		{Name: "Set bucket lifecycle", Value: "set_bucket_lifecycle"},
		{Name: "Generate signed URL", Value: "generate_signed_url"},
	}
	objectOps := nodeprops.Show("operation", "upload", "download", "delete", "copy", "move", "get_metadata", "set_metadata")
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "upload",
			"Cloud Storage operation to run.",
			ops, true),
		nodeprops.Str("Bucket", "bucket", "",
			"Bucket the operation works on.",
			"my-bucket", true),
		nodeprops.Str("Object", "object", "",
			"Object name (path inside the bucket).",
			"folder/report.csv", false).
			WithDisplayOptions(objectOps),
		nodeprops.Str("Source", "source", "",
			"Expression or field holding the bytes to upload.",
			"={{ $json.data }}", false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Str("Destination", "destination", "",
			"Destination object path (copy / move operations).",
			"folder/copy.csv", false).
			WithDisplayOptions(nodeprops.Show("operation", "copy", "move")),
		nodeprops.Str("Prefix", "prefix", "",
			"Object prefix used to filter the listing.",
			"reports/", false).
			WithDisplayOptions(nodeprops.Show("operation", "list")),
		nodeprops.Str("Delimiter", "delimiter", "/",
			"Delimiter used to group the listing.",
			"/", false).
			WithDisplayOptions(nodeprops.Show("operation", "list")),
		nodeprops.Str("Content type", "contentType", "",
			"Content-Type of the stored object.",
			"text/csv", false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Opt("Storage class", "storageClass", "STANDARD",
			"Storage class of the stored object.",
			[]base.Option{
				{Name: "Standard", Value: "STANDARD"},
				{Name: "Nearline", Value: "NEARLINE"},
				{Name: "Coldline", Value: "COLDLINE"},
				{Name: "Archive", Value: "ARCHIVE"},
			}, false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Bool("Versioning", "versioning", false,
			"Enable versioning on the bucket.").
			WithDisplayOptions(nodeprops.Show("operation", "set_bucket_lifecycle")),
		nodeprops.JSON("Metadata", "metadata", "{}",
			"Object metadata (name → value).").
			WithDisplayOptions(nodeprops.Show("operation", "set_metadata")),
		nodeprops.Str("Project ID", "projectId", "",
			"Google Cloud project the bucket lives in.",
			"my-project", false),
		nodeprops.Str("Key file path", "keyFile", "",
			"Path of the service account JSON key file.",
			"/data/service-account.json", false),
		nodeprops.Text("Key file content", "keyFileContent", "",
			"Service account JSON key (pasted directly).",
			"", 8),
		nodeprops.Str("Service account email", "serviceAccountEmail", "",
			"Service account email used to sign requests.",
			"svc@my-project.iam.gserviceaccount.com", false),
		nodeprops.Bool("Use application default credentials", "useADC", false,
			"Authenticate with the host's application default credentials."),
		nodeprops.JSON("Advanced options", "options", "{}",
			"Extra operation options passed through to the Google Cloud SDK call."),
	}
	return append(props, nodeprops.Settings()...)
}
