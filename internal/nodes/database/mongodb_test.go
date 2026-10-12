package database

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
)

// TestMongoDBNode_Description — the new native-driver node
// exposes the same NodeDescription shape as before but with a
// `connectionString` parameter instead of `baseUrl` + `dataSource` +
// `apiKey`.
func TestMongoDBNode_Description(t *testing.T) {
	node := NewMongoDBNode()
	desc := node.Description()
	assert.Equal(t, "MongoDB", desc.Name)
	assert.Equal(t, "Database", desc.Category)
	// Connection String is the first required prop now.
	assert.NotEmpty(t, desc.Properties)
	assert.Equal(t, "connectionString", desc.Properties[0].Name)
	assert.True(t, desc.Properties[0].Required, "connectionString must be required")
}

// TestMongoDBNode_Validate — exercises the new parameter
// surface. Pure unit test, no driver round-trip.
func TestMongoDBNode_Validate(t *testing.T) {
	node := NewMongoDBNode()

	// nil params
	assert.Error(t, node.ValidateParameters(nil))

	// missing connectionString
	assert.Error(t, node.ValidateParameters(map[string]interface{}{
		"operation":  "find",
		"database":   "db",
		"collection": "col",
	}))

	// missing database
	assert.Error(t, node.ValidateParameters(map[string]interface{}{
		"connectionString": "mongodb://localhost:27017",
		"operation":        "find",
		"collection":       "col",
	}))

	// missing collection
	assert.Error(t, node.ValidateParameters(map[string]interface{}{
		"connectionString": "mongodb://localhost:27017",
		"operation":        "find",
		"database":         "db",
	}))

	// invalid operation
	assert.Error(t, node.ValidateParameters(map[string]interface{}{
		"connectionString": "mongodb://localhost:27017",
		"operation":        "dropDatabase",
		"database":         "db",
		"collection":       "col",
	}))

	// happy path (mongodb://)
	assert.NoError(t, node.ValidateParameters(map[string]interface{}{
		"connectionString": "mongodb://localhost:27017",
		"operation":        "find",
		"database":         "db",
		"collection":       "col",
	}))

	// happy path (mongodb+srv://)
	assert.NoError(t, node.ValidateParameters(map[string]interface{}{
		"connectionString": "mongodb+srv://user:pass@cluster0.x.mongodb.net/?appName=Cluster0",
		"operation":        "find",
		"database":         "db",
		"collection":       "col",
	}))
}

// TestToBSON — exercises the JSON ↔ bson.M conversion. The
// `filter` parameter is a string in the workflow JSON; the node
// must accept it, parse it, and feed the result to the driver.
func TestToBSON(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want map[string]interface{}
	}{
		{"nil", nil, map[string]interface{}{}},
		{"empty string", "", map[string]interface{}{}},
		{"empty brace", "{}", map[string]interface{}{}},
		{"empty brace with spaces", "  {}  ", map[string]interface{}{}},
		{"null", "null", map[string]interface{}{}},
		{"plain object", `{"a": 1, "b": "x"}`, map[string]interface{}{"a": float64(1), "b": "x"}},
		{"nested object", `{"a": {"b": [1, 2]}}`, map[string]interface{}{"a": map[string]interface{}{"b": []interface{}{float64(1), float64(2)}}}},
		{"map passthrough", map[string]interface{}{"a": "b"}, map[string]interface{}{"a": "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := toBSON(tt.in)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestToBSON_InvalidJSON(t *testing.T) {
	_, err := toBSON("{not-json")
	assert.Error(t, err)
}

func TestToBSON_UnsupportedType(t *testing.T) {
	_, err := toBSON(42)
	assert.Error(t, err)
}

// TestConvertMapToBSON — sanity check on the simple
// map-to-bson.M copy. We only need the keys; values pass
// through.
func TestConvertMapToBSON(t *testing.T) {
	in := map[string]interface{}{"a": "x", "b": 2, "c": true}
	out := convertMapToBSON(in)
	assert.Equal(t, "x", out["a"])
	assert.Equal(t, 2, out["b"])
	assert.Equal(t, true, out["c"])
	assert.Len(t, out, 3)
}

// TestParsePipeline — the aggregation pipeline is a JSON array.
// Same JSON-or-slice-or-nil semantics as documents.
func TestParsePipeline(t *testing.T) {
	out, err := parsePipeline(`[{"$match": {"active": true}}, {"$count": "n"}]`)
	assert.NoError(t, err)
	assert.Len(t, out, 2)

	out, err = parsePipeline([]interface{}{
		map[string]interface{}{"$match": map[string]interface{}{"active": true}},
	})
	assert.NoError(t, err)
	assert.Len(t, out, 1)

	out, err = parsePipeline("")
	assert.NoError(t, err)
	assert.Nil(t, out)
}

// TestObjectIDToString — primitive.ObjectID → hex (covered
// indirectly via the objectIDToString helper); the string and
// int paths are the ones the unit suite can hit without
// importing primitive.
func TestObjectIDToString(t *testing.T) {
	assert.Equal(t, "abc", objectIDToString("abc"))
	assert.Equal(t, "42", objectIDToString(42))
}

// TestMongoDBNode_MissingConnectionString — Execute must fail
// cleanly with a clear error when the user forgot to paste a
// connectionString.
func TestMongoDBNode_MissingConnectionString(t *testing.T) {
	node := NewMongoDBNode()
	_, err := node.Execute(nil, map[string]interface{}{
		"operation":  "find",
		"database":   "db",
		"collection": "col",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connectionString is required")
}

// TestMongoDBNode_MissingDatabase — same for database.
func TestMongoDBNode_MissingDatabase(t *testing.T) {
	node := NewMongoDBNode()
	_, err := node.Execute(nil, map[string]interface{}{
		"connectionString": "mongodb://localhost:27017",
		"operation":        "find",
		"collection":       "col",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database is required")
}

// TestMongoDBNode_MissingCollection — same for collection.
func TestMongoDBNode_MissingCollection(t *testing.T) {
	node := NewMongoDBNode()
	_, err := node.Execute(nil, map[string]interface{}{
		"connectionString": "mongodb://localhost:27017",
		"operation":        "find",
		"database":         "db",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "collection is required")
}

// TestMongoDBNode_BadConnectionString — connect/ping should
// fail on an unreachable URI. We use a privileged port nobody
// listens on and a 200ms server-selection timeout so the test
// runs in <1s without needing a live MongoDB.
func TestMongoDBNode_BadConnectionString(t *testing.T) {
	node := NewMongoDBNode()
	_, err := node.Execute(nil, map[string]interface{}{
		"connectionString": "mongodb://localhost:1/?serverSelectionTimeoutMS=200&connectTimeoutMS=200",
		"operation":        "find",
		"database":         "db",
		"collection":       "col",
	})
	assert.Error(t, err, "expected an error for an unreachable mongo")
}

// TestMongoDBNode_FirstDocument — the document parameter accepts
// a JSON string or a map. Empty falls back to input data.
func TestMongoDBNode_FirstDocument(t *testing.T) {
	node := NewMongoDBNode()

	d, err := node.firstDocument(`{"name": "x"}`, nil)
	assert.NoError(t, err)
	assert.Equal(t, "x", d["name"])

	_, err = node.firstDocument("", nil)
	assert.Error(t, err)

	// fallback to first input item
	d, err = node.firstDocument(nil, []model.DataItem{
		{JSON: map[string]interface{}{"name": "y"}},
	})
	assert.NoError(t, err)
	assert.Equal(t, "y", d["name"])
}

// TestMongoDBNode_ManyDocuments — the documents parameter
// accepts a JSON array, a []interface{}, or falls back to
// input items.
func TestMongoDBNode_ManyDocuments(t *testing.T) {
	node := NewMongoDBNode()

	docs, err := node.manyDocuments(`[{"a": 1}, {"a": 2}]`, nil)
	assert.NoError(t, err)
	assert.Len(t, docs, 2)

	docs, err = node.manyDocuments([]interface{}{
		map[string]interface{}{"a": 1},
		map[string]interface{}{"a": 2},
	}, nil)
	assert.NoError(t, err)
	assert.Len(t, docs, 2)

	// fallback to input items
	docs, err = node.manyDocuments(nil, []model.DataItem{
		{JSON: map[string]interface{}{"a": 1}},
		{JSON: map[string]interface{}{"a": 2}},
	})
	assert.NoError(t, err)
	assert.Len(t, docs, 2)

	// empty everything → error
	_, err = node.manyDocuments("", nil)
	assert.Error(t, err)
}

// TestNormalizeBSONValue — covers the BSON-to-JSON shape
// conversion. The `ObjectID` / `DateTime` cases are covered in
// integration tests; here we exercise the plain-map and
// slice-of-maps paths that don't need a driver.
func TestNormalizeBSONValue(t *testing.T) {
	in := map[string]interface{}{
		"a": "x",
		"b": []interface{}{1.0, 2.0, "s"},
		"c": map[string]interface{}{"nested": true},
	}
	out := normalizeBSONValue(in)
	m, ok := out.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "x", m["a"])
	assert.Equal(t, true, m["c"].(map[string]interface{})["nested"])
}
