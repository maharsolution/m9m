package productivity

import "testing"

func TestGoogleSheetsOAuthTokenEnvelope(t *testing.T) {
	for _, value := range []interface{}{map[string]interface{}{"access_token": "secret"}, `{"access_token":"secret"}`} {
		if got := NewGoogleSheetsNode().getAccessToken(map[string]interface{}{"googleSheetsOAuth2Api_oauthTokenData": value}); got != "secret" {
			t.Fatalf("token resolution failed for %T", value)
		}
	}
	for _, value := range []interface{}{`map[access_token:secret]`, `{broken`, `{"access_token":null}`} {
		if got := NewGoogleSheetsNode().getAccessToken(map[string]interface{}{"googleSheetsOAuth2Api_oauthTokenData": value}); got != "" {
			t.Fatal("invalid token data accepted")
		}
	}
}
