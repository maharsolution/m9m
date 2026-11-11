package database

import (
	"database/sql"

	"github.com/neul-labs/m9m/internal/model"
)

// scanRows reads a SQL result set into one item per row, the way n8n's
// MySQL / Postgres / SQLite nodes do it.
//
// n8n returns ONE item per row for the `executeQuery` operation: each
// item's JSON holds that row's columns. m9m used to return a single
// item shaped `{"rows": [...]}` for the whole result set, which broke
// parity for every workflow that ends in a database read — the
// webhook response then carried one object with a `rows` array
// instead of the row objects themselves.
//
// The column values are passed through untouched: the driver decides
// whether a column arrives as a string, a number, a bool, a []byte or
// nil, and n8n's drivers make the same choice for the same column.
func scanRows(rows *sql.Rows) ([]model.DataItem, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	items := make([]model.DataItem, 0, 8)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			if i < len(values) {
				row[col] = values[i]
			} else {
				row[col] = nil
			}
		}

		items = append(items, model.DataItem{JSON: row})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}
