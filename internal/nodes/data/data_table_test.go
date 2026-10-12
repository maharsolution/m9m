package data

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memStore is a thread-safe in-memory DataTableStore for tests.
// Production uses *storage.MySQLStorage (or any other
// storage.WorkflowStorage) directly.
type memStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newMemStore() *memStore { return &memStore{m: map[string][]byte{}} }

func (s *memStore) SaveRaw(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	s.m[key] = cp
	return nil
}
func (s *memStore) GetRaw(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}
func (s *memStore) ListKeys(prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0)
	for k := range s.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}
func (s *memStore) DeleteRaw(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// seedRow is a tiny helper for tests.
func seedRow(t *testing.T, s *memStore, tableID, rowID string, data map[string]interface{}) {
	t.Helper()
	buf, err := json.Marshal(row{ID: rowID, Data: data})
	require.NoError(t, err)
	require.NoError(t, s.SaveRaw(dataTableKey(tableID, rowID), buf))
}

func TestDataTableNode_Description(t *testing.T) {
	n := NewDataTableNode(newMemStore())
	d := n.Description()
	assert.Equal(t, "Data Table", d.Name)
	assert.Equal(t, "Data", d.Category)
	require.NotEmpty(t, d.Properties)
	// dataTableId is the first required prop.
	assert.Equal(t, "dataTableId", d.Properties[0].Name)
	assert.True(t, d.Properties[0].Required)
}

func TestDataTableNode_Validate(t *testing.T) {
	n := NewDataTableNode(newMemStore())

	assert.Error(t, n.ValidateParameters(nil))
	assert.Error(t, n.ValidateParameters(map[string]interface{}{
		"operation": "get",
	}))
	assert.Error(t, n.ValidateParameters(map[string]interface{}{
		"dataTableId": "t1",
		"operation":   "drop",
	}))
	assert.NoError(t, n.ValidateParameters(map[string]interface{}{
		"dataTableId": "t1",
		"operation":   "get",
	}))
}

func TestDataTableNode_MissingStore(t *testing.T) {
	n := NewDataTableNode(nil)
	_, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t1",
		"operation":   "get",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage backend is not wired")
}

func TestDataTableNode_MissingDataTableId(t *testing.T) {
	n := NewDataTableNode(newMemStore())
	_, err := n.Execute(nil, map[string]interface{}{
		"operation": "get",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dataTableId is required")
}

func TestDataTableNode_InsertAndGet(t *testing.T) {
	store := newMemStore()
	n := NewDataTableNode(store)

	// Insert two rows.
	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "customers",
		"operation":   "insert",
		"dataToSend":  `{"customer_id": "C001", "isCSFP": true}`,
	})
	require.NoError(t, err)
	require.Len(t, out, 1)
	id1 := out[0].JSON["id"].(string)
	assert.NotEmpty(t, id1)

	_, err = n.Execute(nil, map[string]interface{}{
		"dataTableId": "customers",
		"operation":   "insert",
		"dataToSend":  `{"customer_id": "C002", "isCSFP": false}`,
	})
	require.NoError(t, err)

	// Get all.
	out, err = n.Execute(nil, map[string]interface{}{
		"dataTableId": "customers",
		"operation":   "get",
	})
	require.NoError(t, err)
	assert.Len(t, out, 2)
}

func TestDataTableNode_Get_EqualsFilter(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"customer_id": "C001", "isCSFP": true})
	seedRow(t, store, "t", "r2", map[string]interface{}{"customer_id": "C002", "isCSFP": true})
	seedRow(t, store, "t", "r3", map[string]interface{}{"customer_id": "C003", "isCSFP": false})
	n := NewDataTableNode(store)

	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "get",
		"filters":     `{"conditions": [{"keyName": "customer_id", "keyValue": "C002", "condition": "equals"}]}`,
	})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "C002", out[0].JSON["row"].(map[string]interface{})["customer_id"])
}

func TestDataTableNode_Get_IsTrue(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"isCSFP": true})
	seedRow(t, store, "t", "r2", map[string]interface{}{"isCSFP": true})
	seedRow(t, store, "t", "r3", map[string]interface{}{"isCSFP": false})
	n := NewDataTableNode(store)

	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "get",
		"filters":     `{"conditions": [{"keyName": "isCSFP", "condition": "isTrue"}]}`,
	})
	require.NoError(t, err)
	assert.Len(t, out, 2)
}

func TestDataTableNode_Get_AllConditions_AND(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"customer_id": "C001", "isCSFP": true})
	seedRow(t, store, "t", "r2", map[string]interface{}{"customer_id": "C002", "isCSFP": false})
	n := NewDataTableNode(store)

	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "get",
		"matchType":   "allConditions",
		"filters":     `{"conditions": [{"keyName": "customer_id", "keyValue": "C001", "condition": "equals"}, {"keyName": "isCSFP", "condition": "isTrue"}]}`,
	})
	require.NoError(t, err)
	assert.Len(t, out, 1, "AND of equals+isTrue should match exactly one row")
}

func TestDataTableNode_Get_AnyCondition_OR(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"customer_id": "C001", "isCSFP": false})
	seedRow(t, store, "t", "r2", map[string]interface{}{"customer_id": "C002", "isCSFP": true})
	seedRow(t, store, "t", "r3", map[string]interface{}{"customer_id": "C003", "isCSFP": false})
	n := NewDataTableNode(store)

	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "get",
		"matchType":   "anyCondition",
		"filters":     `{"conditions": [{"keyName": "customer_id", "keyValue": "C001", "condition": "equals"}, {"keyName": "isCSFP", "condition": "isTrue"}]}`,
	})
	require.NoError(t, err)
	assert.Len(t, out, 2, "OR of equals(C001) or isTrue should match 2 rows")
}

func TestDataTableNode_Get_ExpressionInFilter(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"customer_id": "C001"})
	seedRow(t, store, "t", "r2", map[string]interface{}{"customer_id": "C002"})
	n := NewDataTableNode(store)

	// Use an n8n expression to pull the customer_id from the
	// input item. This is the canonical pattern from the
	// Finnet workflow.
	out, err := n.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{"id": "C002"}}},
		map[string]interface{}{
			"dataTableId": "t",
			"operation":   "get",
			"filters":     `{"conditions": [{"keyName": "customer_id", "keyValue": "={{ $json.id }}", "condition": "equals"}]}`,
		},
	)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "C002", out[0].JSON["row"].(map[string]interface{})["customer_id"])
}

func TestDataTableNode_Update(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"customer_id": "C001", "status": "active"})
	seedRow(t, store, "t", "r2", map[string]interface{}{"customer_id": "C002", "status": "active"})
	n := NewDataTableNode(store)

	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "update",
		"filters":     `{"conditions": [{"keyName": "customer_id", "keyValue": "C001", "condition": "equals"}]}`,
		"dataToSend":  `{"status": "inactive"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, out[0].JSON["updatedCount"])

	// Re-read r1 — should now be inactive.
	raw, err := store.GetRaw(dataTableKey("t", "r1"))
	require.NoError(t, err)
	var r row
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "inactive", r.Data["status"])

	// r2 should be untouched.
	raw, err = store.GetRaw(dataTableKey("t", "r2"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "active", r.Data["status"])
}

func TestDataTableNode_Update_EmptyDataToSend(t *testing.T) {
	n := NewDataTableNode(newMemStore())
	_, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "update",
		"filters":     `{"conditions": [{"keyName": "x", "keyValue": "y", "condition": "equals"}]}`,
		"dataToSend":  `{}`,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestDataTableNode_Upsert_UpdatesExisting(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"customer_id": "C001", "status": "active"})
	n := NewDataTableNode(store)

	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId":      "t",
		"operation":        "upsert",
		"columnToMatchOn":  "customer_id",
		"dataToSend":       `{"customer_id": "C001", "status": "inactive"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", out[0].JSON["action"])
	assert.Equal(t, "r1", out[0].JSON["id"])

	raw, err := store.GetRaw(dataTableKey("t", "r1"))
	require.NoError(t, err)
	var r row
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "inactive", r.Data["status"])
}

func TestDataTableNode_Upsert_InsertsWhenMissing(t *testing.T) {
	store := newMemStore()
	n := NewDataTableNode(store)

	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId":     "t",
		"operation":       "upsert",
		"columnToMatchOn": "customer_id",
		"dataToSend":      `{"customer_id": "C001", "status": "active"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "inserted", out[0].JSON["action"])
	assert.NotEmpty(t, out[0].JSON["insertedId"])

	// And the row is now findable.
	keys, err := store.ListKeys(tablePrefix("t"))
	require.NoError(t, err)
	assert.Len(t, keys, 1)
}

func TestDataTableNode_Upsert_DefaultsColumnFromFirstKey(t *testing.T) {
	store := newMemStore()
	n := NewDataTableNode(store)

	_, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "upsert",
		"dataToSend":  `{"customer_id": "C001", "status": "active"}`,
	})
	require.NoError(t, err)
}

func TestDataTableNode_Delete(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"isCSFP": true})
	seedRow(t, store, "t", "r2", map[string]interface{}{"isCSFP": false})
	seedRow(t, store, "t", "r3", map[string]interface{}{"isCSFP": true})
	n := NewDataTableNode(store)

	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "delete",
		"filters":     `{"conditions": [{"keyName": "isCSFP", "condition": "isTrue"}]}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, out[0].JSON["deletedCount"])

	keys, err := store.ListKeys(tablePrefix("t"))
	require.NoError(t, err)
	assert.Len(t, keys, 1, "only r2 should remain")
}

func TestDataTableNode_Delete_RefusesEmptyFilter(t *testing.T) {
	store := newMemStore()
	seedRow(t, store, "t", "r1", map[string]interface{}{"x": 1})
	n := NewDataTableNode(store)

	_, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "delete",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to wipe")
}

func TestDataTableNode_Get_ReturnAll(t *testing.T) {
	store := newMemStore()
	for i := 0; i < 5; i++ {
		seedRow(t, store, "t", string(rune('a'+i)), map[string]interface{}{"k": i})
	}
	n := NewDataTableNode(store)

	// Default limit is 50, so all 5 should come back.
	out, err := n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "get",
		"limit":       2,
	})
	require.NoError(t, err)
	assert.Len(t, out, 2)

	// returnAll=1 → no limit.
	out, err = n.Execute(nil, map[string]interface{}{
		"dataTableId": "t",
		"operation":   "get",
		"returnAll":   1,
		"limit":       2,
	})
	require.NoError(t, err)
	assert.Len(t, out, 5)
}

func TestDataTableNode_KeyFormat(t *testing.T) {
	assert.Equal(t, "dataTable:tbl:rowid", dataTableKey("tbl", "rowid"))
	assert.Equal(t, "dataTable:tbl:", tablePrefix("tbl"))
	assert.Equal(t, "rowid", rowIDFromKey("dataTable:tbl:rowid"))
	assert.Equal(t, "", rowIDFromKey("not-a-dataTable-key"))
}

// TestConditions — unit tests for the condition evaluator.
func TestConditions(t *testing.T) {
	r := &row{Data: map[string]interface{}{"name": "alice", "age": 30, "active": true, "score": 95.5}}
	cases := []struct {
		name string
		c    condition
		want bool
	}{
		{"equals-true", condition{"name", "alice", "equals"}, true},
		{"equals-false", condition{"name", "bob", "equals"}, false},
		{"notEquals-true", condition{"name", "bob", "notEquals"}, true},
		{"notEquals-false", condition{"name", "alice", "notEquals"}, false},
		{"contains-string", condition{"name", "ali", "contains"}, true},
		{"contains-miss", condition{"name", "xyz", "contains"}, false},
		{"greaterThan-true", condition{"age", 18, "greaterThan"}, true},
		{"greaterThan-false", condition{"age", 50, "greaterThan"}, false},
		{"lessThan-true", condition{"age", 50, "lessThan"}, true},
		{"lessThan-false", condition{"age", 18, "lessThan"}, false},
		{"isTrue-true", condition{"active", nil, "isTrue"}, true},
		{"isFalse-true", condition{"active", nil, "isFalse"}, false},
		{"exists-true", condition{"name", nil, "exists"}, true},
		{"exists-false", condition{"missing", nil, "exists"}, false},
		{"notExists-true", condition{"missing", nil, "notExists"}, true},
		{"unknown-op", condition{"name", "x", "wat"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evalCondition(tc.c, r)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReflectEqual_NumericCoercion(t *testing.T) {
	// 30 (int) and 30.0 (float) should be equal.
	assert.True(t, reflectEqual(30, 30.0))
	// 30 (int) and "30" (string) should be equal.
	assert.True(t, reflectEqual(30, "30"))
	// true (bool) and "true" (string) should be equal.
	assert.True(t, reflectEqual(true, "true"))
	// nil and "" should NOT be equal.
	assert.False(t, reflectEqual(nil, ""))
}
