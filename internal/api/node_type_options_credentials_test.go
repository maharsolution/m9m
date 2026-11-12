package api

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/storage"
)

// TestGetNodeTypeOptions_ProviderReceivesCredentialResolver asserts that
// a `loadOptions` provider can resolve a token from the server-side
// credential store — the browser never receives the secret itself.
func TestGetNodeTypeOptions_ProviderReceivesCredentialResolver(t *testing.T) {
	t.Setenv("N8N_ENCRYPTION_SALT", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("N8N_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("M9M_DATA_DIR", t.TempDir())

	credMgr, err := credentials.NewCredentialManager()
	require.NoError(t, err)

	eng := engine.NewWorkflowEngine()
	eng.RegisterNodeExecutor(optionsTestNodeType, &optionsTestNode{
		description: base.NodeDescription{
			Name:     "Options Test",
			Category: "utility",
			Properties: []base.NodeProperty{
				{
					Name:        "resource",
					DisplayName: "Resource",
					Type:        "resourceLocator",
					Options: []base.Option{
						{Name: "List", Value: "list", Action: optionsTestAction},
					},
				},
			},
		},
	})

	server := NewAPIServer(eng, nil, storage.NewMemoryStorage())
	server.SetCredentialManager(credMgr)

	router := mux.NewRouter()
	server.RegisterRoutes(router)

	var seen CredentialResolver
	RegisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction,
		func(ctx context.Context, req NodeTypeOptionRequest) ([]NodeTypeOption, error) {
			seen = req.Credentials
			return []NodeTypeOption{{Name: "One", Value: "one"}}, nil
		})
	defer UnregisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/node-types/"+optionsTestNodeType+"/options?property=resource", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, seen, "the provider must receive the credential resolver")
	assert.Same(t, credMgr, seen)
}
