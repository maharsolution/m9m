package vcs

import (
	"github.com/neul-labs/m9m/internal/model"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubCredentialServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/user" {
			t.Errorf("path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "token secret" {
			t.Error("missing credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"example"}`))
	}))
	defer server.Close()
	node := NewGitHubNode()
	result, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, map[string]interface{}{"resource": "user", "operation": "getAuthenticated", "githubApi_accessToken": "secret", "githubApi_server": server.URL + "/api/v3/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].JSON["login"] != "example" {
		t.Fatalf("unexpected result %v", result)
	}
	for _, server := range []string{"file:///tmp/api", "https://user:pass@example.com"} {
		_, err := node.makeAPIRequest("GET", "/user", "secret", nil, map[string]interface{}{"githubApi_server": server})
		if err == nil {
			t.Fatal("invalid server accepted")
		}
	}
}
