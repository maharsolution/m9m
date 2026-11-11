package productivity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListSpreadsheets_ReturnsOneOptionPerSpreadsheet asserts that the
// Drive answer is mapped to the `{name, value}` option shape the
// parameter panel expects.
func TestListSpreadsheets_ReturnsOneOptionPerSpreadsheet(t *testing.T) {
	var gotAuth string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"files":[` +
			`{"id":"id-1","name":"Budget"},` +
			`{"id":"id-2","name":""},` +
			`{"id":"","name":"ignored"}]}`))
	}))
	defer server.Close()

	original := googleDriveFilesURL
	googleDriveFilesURL = server.URL + "/drive/v3/files"
	defer func() { googleDriveFilesURL = original }()

	options, err := ListSpreadsheets(context.Background(), "token-123")
	require.NoError(t, err)
	require.Len(t, options, 2, "files without an id are skipped")

	assert.Equal(t, "Budget", options[0].Name)
	assert.Equal(t, "id-1", options[0].Value)
	assert.Equal(t, "id-2", options[1].Name, "a file without a title falls back to its id")
	assert.Equal(t, "id-2", options[1].Value)

	assert.Equal(t, "Bearer token-123", gotAuth)
	assert.Contains(t, gotQuery, "mimeType")
	assert.Contains(t, gotQuery, "trashed")
}

// TestListSpreadsheets_RequiresToken asserts that an empty token is
// rejected before any request leaves the process.
func TestListSpreadsheets_RequiresToken(t *testing.T) {
	_, err := ListSpreadsheets(context.Background(), "   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

// TestListSpreadsheets_ReportsUpstreamError asserts that a Drive error
// is surfaced instead of being swallowed into an empty list.
func TestListSpreadsheets_ReportsUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient scope"}}`))
	}))
	defer server.Close()

	original := googleDriveFilesURL
	googleDriveFilesURL = server.URL + "/drive/v3/files"
	defer func() { googleDriveFilesURL = original }()

	_, err := ListSpreadsheets(context.Background(), "token-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

// TestSpreadsheetID_AcceptsLocatorAndPlainString asserts that both the
// resource-locator envelope and the legacy plain string resolve to the
// spreadsheet id.
func TestSpreadsheetID_AcceptsLocatorAndPlainString(t *testing.T) {
	assert.Equal(t, "abc", SpreadsheetID("abc"))
	assert.Equal(t, "abc", SpreadsheetID(map[string]interface{}{
		"__rl":  true,
		"value": "abc",
		"mode":  "list",
	}))
	assert.Equal(t, "abc", SpreadsheetID(map[string]interface{}{
		"spreadsheetId": "abc",
	}))
	assert.Equal(t, "", SpreadsheetID(nil))
	assert.Equal(t, "", SpreadsheetID(42))
}
