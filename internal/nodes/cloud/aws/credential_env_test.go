package aws

import "testing"

func TestCredentialRegionPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{"default", map[string]interface{}{}, "us-east-1"},
		{"envelope", map[string]interface{}{"aws_region": "eu-west-1"}, "eu-west-1"},
		{"empty inline", map[string]interface{}{"region": "", "aws_region": "eu-west-1"}, "eu-west-1"},
		{"inline", map[string]interface{}{"region": "ap-south-1", "aws_region": "eu-west-1"}, "ap-south-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s3, err := (&S3OperationsNode{}).parseS3Config(tc.params)
			if err != nil {
				t.Fatal(err)
			}
			lambda, err := (&LambdaOperationsNode{}).parseLambdaConfig(tc.params)
			if err != nil {
				t.Fatal(err)
			}
			if s3.Region != tc.want || lambda.Region != tc.want {
				t.Fatalf("regions S3=%q Lambda=%q want %q", s3.Region, lambda.Region, tc.want)
			}
		})
	}
}
