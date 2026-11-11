package productivity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SpreadsheetOption is one spreadsheet entry returned by the
// `loadOptions` provider of the Google Sheets node.
//
// It mirrors the wire shape of `api.NodeTypeOption` (`{name, value}`)
// without importing the API layer: the node package stays free of HTTP
// server dependencies and the adapter in `cmd/m9m/commands/serve.go`
// converts one into the other.
type SpreadsheetOption struct {
	// Name is the human-readable spreadsheet title.
	Name string `json:"name"`
	// Value is the spreadsheet id (the long id in the sheet URL).
	Value string `json:"value"`
}

// googleSheetsMimeType is the Drive mime type of a Google spreadsheet.
const googleSheetsMimeType = "application/vnd.google-apps.spreadsheet"

// googleDriveFilesURL is the Drive endpoint used to list the
// spreadsheets of the authenticated account. It is a variable so the
// tests can point it at a local server.
var googleDriveFilesURL = "https://www.googleapis.com/drive/v3/files"

// googleDriveHTTPClient performs the Drive calls. Its timeout is far
// below the API layer's provider timeout so a slow Google API cannot
// hold the parameter panel hostage.
var googleDriveHTTPClient = &http.Client{Timeout: 15 * time.Second}

// ListSpreadsheets returns the spreadsheets visible to the Google
// account behind the OAuth token.
//
// The list comes from the Drive API because the Sheets API has no
// "list spreadsheets" endpoint — a spreadsheet is a Drive file of the
// spreadsheet mime type. n8n does the same for its `loadSpreadsheets`
// action.
func ListSpreadsheets(ctx context.Context, token string) ([]SpreadsheetOption, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("google access token is required")
	}

	query := url.Values{}
	query.Set("q", fmt.Sprintf("mimeType = '%s' and trashed = false", googleSheetsMimeType))
	query.Set("fields", "files(id,name)")
	query.Set("pageSize", "100")
	query.Set("orderBy", "name")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		googleDriveFilesURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build drive request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := googleDriveHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list spreadsheets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("drive api error (status %d): %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode drive response: %w", err)
	}

	options := make([]SpreadsheetOption, 0, len(payload.Files))
	for _, file := range payload.Files {
		if file.ID == "" {
			continue
		}
		name := file.Name
		if name == "" {
			name = file.ID
		}
		options = append(options, SpreadsheetOption{Name: name, Value: file.ID})
	}
	return options, nil
}

// GoogleSheetsCredentialType is the credential type the Google Sheets
// node uses. The `loadOptions` provider resolves its access token from
// a credential of this type.
const GoogleSheetsCredentialType = "googleSheetsOAuth2Api"

// SpreadsheetID extracts the spreadsheet id from a parameter value.
//
// The parameter is a resource locator: the parameter panel stores it as
// `{__rl: true, value, mode}`, but older workflows and n8n's own
// `spreadsheetId` field carry the plain id string. Both are accepted.
func SpreadsheetID(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]interface{}:
		if id, ok := value["value"].(string); ok {
			return strings.TrimSpace(id)
		}
		if id, ok := value["spreadsheetId"].(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}
