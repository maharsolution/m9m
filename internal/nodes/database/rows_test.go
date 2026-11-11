package database

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// A tiny in-package SQL driver.
//
// The real SQLite driver needs CGO, which is disabled in the CI image, so
// scanRows is exercised against a fake driver that serves a fixed result
// set. That keeps the test deterministic and runnable everywhere.
// ---------------------------------------------------------------------------

type fakeRowsDriver struct{}

type fakeRowsConn struct{}

type fakeRowsStmt struct {
	query string
}

type fakeRowsResult struct {
	columns []string
	values  [][]driver.Value
	pos     int
}

func (fakeRowsDriver) Open(string) (driver.Conn, error) { return fakeRowsConn{}, nil }

func (fakeRowsConn) Prepare(query string) (driver.Stmt, error) {
	return fakeRowsStmt{query: query}, nil
}
func (fakeRowsConn) Close() error              { return nil }
func (fakeRowsConn) Begin() (driver.Tx, error) { return nil, errors.New("not supported") }

func (s fakeRowsStmt) Close() error  { return nil }
func (s fakeRowsStmt) NumInput() int { return -1 }

func (s fakeRowsStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("not supported")
}

func (s fakeRowsStmt) Query([]driver.Value) (driver.Rows, error) {
	if strings.Contains(strings.ToLower(s.query), "empty") {
		return &fakeRowsResult{columns: []string{"id", "amount", "note"}}, nil
	}
	return &fakeRowsResult{
		columns: []string{"id", "amount", "note"},
		values: [][]driver.Value{
			{"a", int64(10), "first"},
			{"b", int64(20), nil},
		},
	}, nil
}

func (r *fakeRowsResult) Columns() []string { return r.columns }
func (r *fakeRowsResult) Close() error      { return nil }

func (r *fakeRowsResult) Next(dest []driver.Value) error {
	if r.pos >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.pos])
	r.pos++
	return nil
}

func init() {
	sql.Register("m9m-fake-rows", fakeRowsDriver{})
}

// TestScanRows_OneItemPerRow pins the n8n item shape for the SQL
// nodes' `executeQuery` operation: one item per row, each item's JSON
// holding that row's columns.
//
// m9m used to collapse the whole result set into a single item shaped
// `{"rows": [...]}`, which is not what n8n produces and broke parity
// for every workflow that ends in a database read.
func TestScanRows_OneItemPerRow(t *testing.T) {
	db, err := sql.Open("m9m-fake-rows", "")
	require.NoError(t, err)
	defer db.Close()

	rows, err := db.Query(`SELECT id, amount, note FROM t`)
	require.NoError(t, err)
	defer rows.Close()

	items, err := scanRows(rows)
	require.NoError(t, err)

	require.Len(t, items, 2, "one item per row")
	assert.Equal(t, "a", items[0].JSON["id"])
	assert.Equal(t, int64(10), items[0].JSON["amount"])
	assert.Equal(t, "first", items[0].JSON["note"])
	assert.NotContains(t, items[0].JSON, "rows",
		"the rows wrapper must not appear in the item")
	assert.Equal(t, "b", items[1].JSON["id"])
	assert.Nil(t, items[1].JSON["note"], "NULL columns must stay nil")
}

// TestScanRows_EmptyResult makes sure an empty result set yields zero
// items instead of one empty item.
func TestScanRows_EmptyResult(t *testing.T) {
	db, err := sql.Open("m9m-fake-rows", "")
	require.NoError(t, err)
	defer db.Close()

	rows, err := db.Query(`SELECT id, amount, note FROM empty_table`)
	require.NoError(t, err)
	defer rows.Close()

	items, err := scanRows(rows)
	require.NoError(t, err)
	assert.Empty(t, items)
}
