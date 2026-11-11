package aws

import "github.com/neul-labs/m9m/internal/nodes/base"

// applyAWSCredentialEnvelope fills empty fields from the flattened
// `aws_*` credential envelope injected by the credential manager.
// Fields already set (from inline workflow params) are kept — the
// envelope is only a fallback.
func applyAWSCredentialEnvelope(params map[string]interface{}, region, accessKeyID, secretAccessKey, sessionToken *string) {
	if *region == "" {
		if v := base.CredentialString(params, "aws_region"); v != "" {
			*region = v
		}
	}
	if *region == "" {
		*region = "us-east-1"
	}
	if *accessKeyID == "" {
		if v := base.CredentialString(params, "aws_accessKeyId"); v != "" {
			*accessKeyID = v
		}
	}
	if *secretAccessKey == "" {
		if v := base.CredentialString(params, "aws_secretAccessKey"); v != "" {
			*secretAccessKey = v
		}
	}
	if *sessionToken == "" {
		if v := base.CredentialString(params, "aws_sessionToken"); v != "" {
			*sessionToken = v
		}
	}
}
