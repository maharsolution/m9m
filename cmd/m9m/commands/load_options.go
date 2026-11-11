package commands

import (
	"context"
	"fmt"

	"github.com/neul-labs/m9m/internal/api"
	"github.com/neul-labs/m9m/internal/nodes/productivity"
)

// registerLoadOptionProviders wires the dynamic-option providers of the
// nodes whose parameter panel needs the user's own resources.
//
// n8n resolves these lists through `loadOptions`; m9m serves them from
// the endpoint the parameter panel already probes
// (`GET /api/v1/node-types/{name}/options`).
//
// The providers read the access token from the server-side credential
// store — the browser never receives a token.
func registerLoadOptionProviders() {
	api.RegisterOptionProvider(
		"n8n-nodes-base.googleSheets",
		"spreadsheetId",
		"loadSpreadsheets",
		googleSheetsSpreadsheetOptions,
	)
}

// googleSheetsSpreadsheetOptions lists the spreadsheets of the Google
// account behind the stored `googleSheetsOAuth2Api` credential.
func googleSheetsSpreadsheetOptions(ctx context.Context, req api.NodeTypeOptionRequest) ([]api.NodeTypeOption, error) {
	if req.Credentials == nil {
		return nil, fmt.Errorf("no credential store is available")
	}
	token, err := req.Credentials.AccessTokenForType(productivity.GoogleSheetsCredentialType)
	if err != nil {
		return nil, err
	}
	spreadsheets, err := productivity.ListSpreadsheets(ctx, token)
	if err != nil {
		return nil, err
	}
	options := make([]api.NodeTypeOption, 0, len(spreadsheets))
	for _, sheet := range spreadsheets {
		options = append(options, api.NodeTypeOption{
			Name:        sheet.Name,
			Value:       sheet.Value,
			Description: sheet.Value,
		})
	}
	return options, nil
}
