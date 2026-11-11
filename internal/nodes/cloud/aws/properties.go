package aws

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// lambdaProperties describes the AWS Lambda node.
//
// aws.LambdaOperationsNode reads `operation`, `functionName`,
// `region`, `accessKeyId`, `secretAccessKey`, `sessionToken`,
// `payload`, `invocationType`, `logType`, `qualifier`, `runtime`,
// `role`, `handler`, `description`, `timeout`, `memorySize`,
// `environment`, `tags` and `options`. The invocation-only and the
// deployment-only fields are shown for the operations that use them.
func lambdaProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Invoke function", Value: "invoke"},
		{Name: "Create function", Value: "create"},
		{Name: "Update function", Value: "update"},
		{Name: "Delete function", Value: "delete"},
		{Name: "Get function", Value: "get"},
		{Name: "List functions", Value: "list"},
		{Name: "Update function code", Value: "update_code"},
		{Name: "Update function configuration", Value: "update_configuration"},
		{Name: "Create alias", Value: "create_alias"},
		{Name: "Update alias", Value: "update_alias"},
		{Name: "Delete alias", Value: "delete_alias"},
		{Name: "List aliases", Value: "list_aliases"},
		{Name: "Publish version", Value: "publish_version"},
		{Name: "List versions", Value: "list_versions"},
	}
	invokeOnly := nodeprops.Show("operation", "invoke")
	deployOnly := nodeprops.Show("operation", "create", "update")
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "invoke",
			"Lambda operation to run.",
			ops, true),
		nodeprops.Str("Function name", "functionName", "",
			"Lambda function name (or ARN).",
			"my-function", true),
		nodeprops.Str("Region", "region", "",
			"AWS region (defaults to the credential's region).",
			"us-east-1", false),
		nodeprops.Str("Access key ID", "accessKeyId", "",
			"AWS access key ID (defaults to the credential).",
			"AKIA…", false),
		nodeprops.Secret("Secret access key", "secretAccessKey", "",
			"AWS secret access key (defaults to the credential).",
			"", false),
		nodeprops.Secret("Session token", "sessionToken", "",
			"Temporary AWS session token.",
			"", false),
		nodeprops.JSON("Payload", "payload", "{}",
			"Event payload sent to the function.").
			WithDisplayOptions(invokeOnly),
		nodeprops.Opt("Invocation type", "invocationType", "RequestResponse",
			"How the function is invoked.",
			[]base.Option{
				{Name: "Request response (synchronous)", Value: "RequestResponse"},
				{Name: "Event (asynchronous)", Value: "Event"},
				{Name: "Dry run", Value: "DryRun"},
			}, false).
			WithDisplayOptions(invokeOnly),
		nodeprops.Opt("Log type", "logType", "None",
			"Whether the invocation returns the last 4 KB of the execution log.",
			[]base.Option{
				{Name: "None", Value: "None"},
				{Name: "Tail", Value: "Tail"},
			}, false).
			WithDisplayOptions(invokeOnly),
		nodeprops.Str("Qualifier", "qualifier", "",
			"Version or alias to invoke (e.g. $LATEST, prod).",
			"$LATEST", false).
			WithDisplayOptions(invokeOnly),
		nodeprops.Str("Runtime", "runtime", "",
			"Runtime of the function (e.g. python3.11).",
			"python3.11", false).
			WithDisplayOptions(deployOnly),
		nodeprops.Str("Role", "role", "",
			"Execution role ARN of the function.",
			"arn:aws:iam::123456789012:role/lambda-role", false).
			WithDisplayOptions(deployOnly),
		nodeprops.Str("Handler", "handler", "",
			"Entry point of the function (e.g. index.handler).",
			"index.handler", false).
			WithDisplayOptions(deployOnly),
		nodeprops.Str("Description", "description", "",
			"Description of the function.",
			"", false).
			WithDisplayOptions(deployOnly),
		nodeprops.Num("Timeout (s)", "timeout", 3,
			"Function timeout in seconds.", false).
			WithDisplayOptions(deployOnly),
		nodeprops.Num("Memory size (MB)", "memorySize", 128,
			"Memory allocated to the function in megabytes.", false).
			WithDisplayOptions(deployOnly),
		nodeprops.JSON("Environment variables", "environment", "{}",
			"Environment variables of the function (name → value).").
			WithDisplayOptions(deployOnly),
		nodeprops.JSON("Tags", "tags", "{}",
			"Tags applied to the function (name → value).").
			WithDisplayOptions(nodeprops.Show("operation", "create")),
		nodeprops.JSON("Advanced options", "options", "{}",
			"Extra operation options passed through to the AWS SDK call."),
	}
	return append(props, nodeprops.Settings()...)
}

// s3Properties describes the AWS S3 node.
//
// aws.S3OperationsNode reads `operation`, `bucket`, `key`, `source`,
// `destination`, `prefix`, `contentType`, `acl`, `storageClass`,
// `versioning`, `metadata`, `copySource`, `region`, `accessKeyId`,
// `secretAccessKey`, `sessionToken`, `endpoint`,
// `s3ForcePathStyle`, `disableSSL` and `options`. The object-level
// fields only apply to the operations that touch objects.
func s3Properties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Upload object", Value: "upload"},
		{Name: "Download object", Value: "download"},
		{Name: "Delete object", Value: "delete"},
		{Name: "List objects", Value: "list"},
		{Name: "Copy object", Value: "copy"},
		{Name: "Head object", Value: "head"},
		{Name: "Create bucket", Value: "create_bucket"},
		{Name: "Delete bucket", Value: "delete_bucket"},
		{Name: "Set bucket policy", Value: "set_bucket_policy"},
		{Name: "Get bucket policy", Value: "get_bucket_policy"},
		{Name: "Set bucket lifecycle", Value: "set_bucket_lifecycle"},
		{Name: "Set bucket versioning", Value: "set_bucket_versioning"},
		{Name: "Generate presigned URL", Value: "generate_presigned_url"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "upload",
			"S3 operation to run.",
			ops, true),
		nodeprops.Str("Bucket", "bucket", "",
			"Bucket the operation works on.",
			"my-bucket", true),
		nodeprops.Str("Key", "key", "",
			"Object key (path inside the bucket).",
			"folder/report.csv", false).
			WithDisplayOptions(nodeprops.Show("operation", "upload", "download", "delete", "head", "copy")),
		nodeprops.Str("Source", "source", "",
			"Expression or field holding the bytes to upload.",
			"={{ $json.data }}", false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Str("Destination", "destination", "",
			"Destination key (copy operation).",
			"folder/copy.csv", false).
			WithDisplayOptions(nodeprops.Show("operation", "copy")),
		nodeprops.JSON("Copy source", "copySource", "{}",
			"Source object of a server-side copy (bucket + key).").
			WithDisplayOptions(nodeprops.Show("operation", "copy")),
		nodeprops.Str("Prefix", "prefix", "",
			"Key prefix used to filter the listing.",
			"reports/", false).
			WithDisplayOptions(nodeprops.Show("operation", "list")),
		nodeprops.Str("Content type", "contentType", "",
			"Content-Type of the stored object.",
			"text/csv", false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Opt("ACL", "acl", "private",
			"Canned ACL applied to the stored object.",
			[]base.Option{
				{Name: "Private", Value: "private"},
				{Name: "Public read", Value: "public-read"},
				{Name: "Public read/write", Value: "public-read-write"},
				{Name: "Authenticated read", Value: "authenticated-read"},
			}, false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Opt("Storage class", "storageClass", "STANDARD",
			"Storage class of the stored object.",
			[]base.Option{
				{Name: "Standard", Value: "STANDARD"},
				{Name: "Reduced redundancy", Value: "REDUCED_REDUNDANCY"},
				{Name: "Standard IA", Value: "STANDARD_IA"},
				{Name: "Glacier", Value: "GLACIER"},
			}, false).
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Bool("Versioning", "versioning", false,
			"Enable versioning on the bucket.").
			WithDisplayOptions(nodeprops.Show("operation", "set_bucket_versioning")),
		nodeprops.JSON("Metadata", "metadata", "{}",
			"Object metadata (name → value).").
			WithDisplayOptions(nodeprops.Show("operation", "upload")),
		nodeprops.Str("Region", "region", "",
			"AWS region (defaults to the credential's region).",
			"us-east-1", false),
		nodeprops.Str("Access key ID", "accessKeyId", "",
			"AWS access key ID (defaults to the credential).",
			"AKIA…", false),
		nodeprops.Secret("Secret access key", "secretAccessKey", "",
			"AWS secret access key (defaults to the credential).",
			"", false),
		nodeprops.Secret("Session token", "sessionToken", "",
			"Temporary AWS session token.",
			"", false),
		nodeprops.Str("Endpoint", "endpoint", "",
			"Custom S3 endpoint (MinIO, R2, …).",
			"https://s3.example.com", false),
		nodeprops.Bool("Force path style", "s3ForcePathStyle", false,
			"Use path-style addressing (required by most S3-compatible stores)."),
		nodeprops.Bool("Disable SSL", "disableSSL", false,
			"Talk to the endpoint over plain HTTP."),
		nodeprops.JSON("Advanced options", "options", "{}",
			"Extra operation options passed through to the AWS SDK call."),
	}
	return append(props, nodeprops.Settings()...)
}
