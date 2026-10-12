package gcp

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the GCS node.

func gcsProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Get", Value: "get"},
		{Name: "Put", Value: "put"},
		{Name: "Delete", Value: "delete"},
		{Name: "List", Value: "list"},
		{Name: "Signed URL", Value: "signedUrl"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Operation", "operation", "get", "GCS operation to perform.", ops, true),
		base.StringProp("Bucket", "bucket", "", "GCS bucket name.", "", true),
		base.StringProp("Object Key", "objectKey", "", "Object key (path within the bucket).", "", false),
		base.StringProp("Destination Bucket", "destinationBucket", "", "Destination bucket (copy only).", "", false),
		base.StringProp("Destination Key", "destinationKey", "", "Destination key (copy only).", "", false),
		base.StringProp("Content Type", "contentType", "", "Override the Content-Type (put).", "", false),
		base.StringProp("Prefix", "prefix", "", "Object prefix filter (list only).", "", false),
		base.StringProp("Signed URL Expiry (s)", "signedUrlExpiry", "3600", "Validity in seconds for signed URL.", "3600", false),
	}
	return append(props, base.CommonSettings()...)
}
