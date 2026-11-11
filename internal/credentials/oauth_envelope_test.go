package credentials

import (
	"encoding/json"
	"testing"

	"github.com/neul-labs/m9m/internal/storage"
)

func TestNodeCredentialsStructuredOAuthToken(t *testing.T) {
	setupManagerTestEnv(t)
	manager, err := NewCredentialManager()
	if err != nil {
		t.Fatal(err)
	}
	err = manager.StoreCredentialFromWorkflow(map[string]interface{}{"id": "oauth", "name": "OAuth", "type": "googleSheetsOAuth2Api", "data": map[string]interface{}{"oauthTokenData": map[string]interface{}{"access_token": "secret", "expires_in": 3600}}})
	if err != nil {
		t.Fatal(err)
	}
	manager.RegisterNodeCredentials("node", "googleSheetsOAuth2Api", "oauth")
	values, err := manager.GetNodeCredentials("node")
	if err != nil {
		t.Fatal(err)
	}
	var token map[string]interface{}
	if err := json.Unmarshal([]byte(values["googleSheetsOAuth2Api_oauthTokenData"]), &token); err != nil {
		t.Fatal(err)
	}
	if token["access_token"] != "secret" {
		t.Fatal("token lost during injection")
	}
	manager.UpsertCredential(&storage.Credential{ID: "oauth", Type: "googleSheetsOAuth2Api", Data: map[string]interface{}{"oauthTokenData": map[string]interface{}{"access_token": "updated"}}})
	values, err = manager.GetNodeCredentials("node")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(values["googleSheetsOAuth2Api_oauthTokenData"]), &token); err != nil {
		t.Fatal(err)
	}
	if token["access_token"] != "updated" {
		t.Fatal("structured storage token lost during injection")
	}
}
