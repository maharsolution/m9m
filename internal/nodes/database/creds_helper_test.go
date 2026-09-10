package database

import (
	"testing"

	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/stretchr/testify/assert"
)

// baseNodeForTest returns a fresh BaseNode the helper can use to call
// GetStringParameter/GetIntParameter. All it needs is a non-nil
// receiver — these methods don't touch any node state.
func baseNodeForTest() *base.BaseNode {
	return base.NewBaseNode(base.NodeDescription{Name: "test"})
}

func TestResolveConnectionParams_ExplicitValues(t *testing.T) {
	b := baseNodeForTest()
	params := map[string]interface{}{
		"host":     "db.example.com",
		"port":     3307,
		"database": "appdb",
		"user":     "app",
		"password": "secret",
		"tls":      "true",
	}
	host, port, db, user, pass, tls := resolveConnectionParams(b, params, "mySql", 3306)
	assert.Equal(t, "db.example.com", host)
	assert.Equal(t, 3307, port)
	assert.Equal(t, "appdb", db)
	assert.Equal(t, "app", user)
	assert.Equal(t, "secret", pass)
	assert.Equal(t, "true", tls)
}

func TestResolveConnectionParams_CredentialFallback(t *testing.T) {
	// No explicit keys — everything comes from credential-injected
	// `mySql_<key>` params. This is the realistic shape for a
	// synced workflow whose credentials live in the m9m store.
	b := baseNodeForTest()
	params := map[string]interface{}{
		"mySql_host":     "187.77.113.218",
		"mySql_port":     "3306",
		"mySql_database": "m9m_prod",
		"mySql_user":     "m9m_admin",
		"mySql_password": "Secure_Db_Pass_2026",
	}
	host, port, db, user, pass, _ := resolveConnectionParams(b, params, "mySql", 3306)
	assert.Equal(t, "187.77.113.218", host)
	assert.Equal(t, 3306, port)
	assert.Equal(t, "m9m_prod", db)
	assert.Equal(t, "m9m_admin", user)
	assert.Equal(t, "Secure_Db_Pass_2026", pass)
}

func TestResolveConnectionParams_ExplicitBeatsCredential(t *testing.T) {
	b := baseNodeForTest()
	params := map[string]interface{}{
		"host":           "override.example.com",
		"mySql_host":     "from-credential.example.com",
		"mySql_port":     3306,
		"mySql_database": "cred_db",
	}
	host, _, db, _, _, _ := resolveConnectionParams(b, params, "mySql", 3306)
	assert.Equal(t, "override.example.com", host, "explicit host must win over credential-injected")
	assert.Equal(t, "cred_db", db, "credential-injected database is used when explicit is empty")
}

func TestResolveConnectionParams_DefaultPort(t *testing.T) {
	b := baseNodeForTest()
	params := map[string]interface{}{
		"mySql_host":     "h",
		"mySql_database": "d",
		"mySql_user":     "u",
		"mySql_password": "p",
	}
	_, port, _, _, _, _ := resolveConnectionParams(b, params, "mySql", 3306)
	assert.Equal(t, 3306, port)

	// Postgres default port (5432) round-trips correctly
	_, pgPort, _, _, _, _ := resolveConnectionParams(b, params, "postgres", 5432)
	assert.Equal(t, 5432, pgPort)
}

func TestResolveConnectionParams_NoPrefix(t *testing.T) {
	// Empty credPrefix → only explicit keys are honoured. Used by
	// legacy CLI workflows that don't have a synced credential.
	b := baseNodeForTest()
	params := map[string]interface{}{
		"host":     "explicit",
		"database": "d",
	}
	host, _, db, _, _, _ := resolveConnectionParams(b, params, "", 3306)
	assert.Equal(t, "explicit", host)
	assert.Equal(t, "d", db)

	empty := map[string]interface{}{}
	host2, _, _, _, _, _ := resolveConnectionParams(b, empty, "", 3306)
	assert.Equal(t, "", host2)
}
