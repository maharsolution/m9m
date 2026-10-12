package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"
)

// DataTableRow is the on-the-wire shape of a single dataTable
// row. Mirrors the shape the `internal/nodes/data` executor
// stores (id + a free-form JSON object), so what you write is
// what you read.
type DataTableRow struct {
	ID  string                 `json:"id"`
	Row map[string]interface{} `json:"row"`
}

// DataTableBulkInsertRequest is the body shape for
// `POST /api/v1/data-tables/{id}/rows` (used by sync-service
// for the n8n → m9m one-shot import).
type DataTableBulkInsertRequest struct {
	Rows []DataTableRow `json:"rows"`
}

// DataTableBulkInsertResponse is the success envelope.
type DataTableBulkInsertResponse struct {
	Inserted int      `json:"inserted"`
	IDs      []string `json:"ids"`
}

// ListDataTables returns the row counts for every data table
// currently stored. We don't store a separate "table
// metadata" row in m9m (n8n does, in `data_table`); instead
// we discover tables by listing all `dataTable:` keys and
// grouping by table id.
func (s *APIServer) ListDataTables(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}
	keys, err := s.storage.ListKeys("dataTable:")
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list data table keys", err)
		return
	}
	counts := map[string]int{}
	for _, k := range keys {
		// keys look like `dataTable:<tableId>:<rowId>`.
		rest := strings.TrimPrefix(k, "dataTable:")
		idx := strings.IndexByte(rest, ':')
		if idx < 0 {
			continue
		}
		counts[rest[:idx]]++
	}
	// Stable order.
	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]interface{}{
			"id":    id,
			"count": counts[id],
		})
	}
	s.sendJSON(w, http.StatusOK, map[string]interface{}{"data": out})
}

// ListDataTableRows returns every row of a single table. The
// rows are sorted by id for deterministic output (the same
// pattern the executor uses internally).
func (s *APIServer) ListDataTableRows(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}
	vars := mux.Vars(r)
	tableID := strings.TrimSpace(vars["id"])
	if tableID == "" {
		s.sendError(w, http.StatusBadRequest, "data table id is required", nil)
		return
	}
	keys, err := s.storage.ListKeys("dataTable:" + tableID + ":")
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list data table keys", err)
		return
	}
	sort.Strings(keys)
	out := make([]DataTableRow, 0, len(keys))
	for _, k := range keys {
		raw, err := s.storage.GetRaw(k)
		if err != nil {
			s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to read %s", k), err)
			return
		}
		var r DataTableRow
		if err := json.Unmarshal(raw, &r); err != nil {
			s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Corrupt row at %s", k), err)
			return
		}
		out = append(out, r)
	}
	s.sendJSON(w, http.StatusOK, map[string]interface{}{"data": out})
}

// BulkInsertDataTableRows is the one-shot import endpoint used
// by sync-service. Existing rows with the same `id` are
// overwritten (idempotent on re-runs).
func (s *APIServer) BulkInsertDataTableRows(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}
	vars := mux.Vars(r)
	tableID := strings.TrimSpace(vars["id"])
	if tableID == "" {
		s.sendError(w, http.StatusBadRequest, "data table id is required", nil)
		return
	}
	var req DataTableBulkInsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON body", err)
		return
	}
	if len(req.Rows) == 0 {
		s.sendError(w, http.StatusBadRequest, "rows array is required (use an empty array to insert nothing)", nil)
		return
	}
	ids := make([]string, 0, len(req.Rows))
	for i, r := range req.Rows {
		if r.ID == "" {
			s.sendError(w, http.StatusBadRequest, fmt.Sprintf("rows[%d].id is required", i), nil)
			return
		}
		if r.Row == nil {
			s.sendError(w, http.StatusBadRequest, fmt.Sprintf("rows[%d].row is required", i), nil)
			return
		}
		buf, err := json.Marshal(r)
		if err != nil {
			s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("rows[%d]: encode failed", i), err)
			return
		}
		if err := s.storage.SaveRaw("dataTable:"+tableID+":"+r.ID, buf); err != nil {
			s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("rows[%d]: save failed", i), err)
			return
		}
		ids = append(ids, r.ID)
	}
	s.sendJSON(w, http.StatusOK, DataTableBulkInsertResponse{
		Inserted: len(ids),
		IDs:      ids,
	})
}

// DeleteDataTableRow removes a single row. Idempotent — 404
// on missing row is treated as success.
func (s *APIServer) DeleteDataTableRow(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}
	vars := mux.Vars(r)
	tableID := strings.TrimSpace(vars["id"])
	rowID := strings.TrimSpace(vars["rowId"])
	if tableID == "" || rowID == "" {
		s.sendError(w, http.StatusBadRequest, "data table id and row id are required", nil)
		return
	}
	key := "dataTable:" + tableID + ":" + rowID
	if err := s.storage.DeleteRaw(key); err != nil {
		// We treat a missing-row delete as success (idempotent).
		// storage backends that return a typed NotFound would
		// need a separate check; the in-memory / MySQL / Postgres
		// / Badger backends all currently return a generic
		// "not found" string for ListKeys misses, but DeleteRaw
		// is a no-op for missing keys, so this branch is rare.
		s.sendError(w, http.StatusInternalServerError, "Failed to delete row", err)
		return
	}
	s.sendJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}
