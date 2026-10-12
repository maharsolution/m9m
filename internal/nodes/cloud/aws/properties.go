package aws

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the AWS nodes.

// lambdaProperties returns the AWS Lambda node's parameter
// descriptors.
func lambdaProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringOpt("Invocation Type", "invocationType", "RequestResponse", "How Lambda is invoked.", []base.Option{
			{Name: "RequestResponse (sync)", Value: "RequestResponse"},
			{Name: "Event (async)", Value: "Event"},
			{Name: "DryRun", Value: "DryRun"},
		}, true),
		base.StringProp("Function Name", "functionName", "", "Lambda function name or ARN.", "my-function", true),
		base.StringProp("Qualifier", "qualifier", "", "Version or alias to invoke (e.g. $LATEST, prod).", "", false),
		base.StringOpt("Payload Format", "payloadFormat", "json", "How the payload is encoded.", []base.Option{
			{Name: "JSON", Value: "json"},
			{Name: "String", Value: "string"},
			{Name: "Binary (base64)", Value: "base64"},
		}, true),
		base.JsonProp("Payload", "payload", "{}", "Payload to send to the Lambda."),
		base.StringProp("Region", "region", "", "AWS region override (defaults to the credential's region).", "", false),
	}
	return append(props, base.CommonSettings()...)
}

// s3Properties returns the AWS S3 node's parameter descriptors.
func s3Properties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Get", Value: "get"},
		{Name: "Put", Value: "put"},
		{Name: "Delete", Value: "delete"},
		{Name: "List", Value: "list"},
		{Name: "Copy", Value: "copy"},
		{Name: "Upload (multipart)", Value: "multipart"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Operation", "operation", "get", "S3 operation to perform.", ops, true),
		base.StringProp("Bucket", "bucket", "", "S3 bucket name.", "", true),
		base.StringProp("Key", "key", "", "Object key (path within the bucket).", "", false),
		base.StringProp("Destination Bucket", "destinationBucket", "", "Destination bucket (copy only).", "", false),
		base.StringProp("Destination Key", "destinationKey", "", "Destination key (copy only).", "", false),
		base.StringProp("Region", "region", "", "AWS region (defaults to credential region).", "", false),
		base.StringProp("Content Type", "contentType", "", "Override the Content-Type (put / multipart).", "", false),
		base.BoolProp("ACL Public Read", "aclPublicRead", false, "Set the object's ACL to public-read."),
		base.JsonProp("Metadata", "metadata", "{}", "Object metadata map (put / multipart)."),
		base.StringProp("Prefix", "prefix", "", "Key prefix filter (list only).", "", false),
		base.NumberProp("Max Keys", "maxKeys", 1000, "Max keys to return (list only).", false),
	}
	return append(props, base.CommonSettings()...)
}
