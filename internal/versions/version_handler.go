package versions

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// AuthorResolver extracts the acting user ID from a request.
//
// The resolver is injected by the package that mounts these routes
// (internal/api wires its own auth context). Keeping it as a hook
// means this package never imports internal/api — which would be an
// import cycle, because internal/api registers the version routes.
type AuthorResolver func(r *http.Request) string

// VersionHandler handles version HTTP requests
type VersionHandler struct {
	versionManager *VersionManager
	authorResolver AuthorResolver
}

// NewVersionHandler creates a new version handler
func NewVersionHandler(versionManager *VersionManager) *VersionHandler {
	return &VersionHandler{
		versionManager: versionManager,
	}
}

// SetAuthorResolver wires the resolver used to attribute a version to
// the user that created or restored it. Without one the handler falls
// back to "system".
func (h *VersionHandler) SetAuthorResolver(resolver AuthorResolver) {
	h.authorResolver = resolver
}

// resolveAuthor returns the acting user for the request.
func (h *VersionHandler) resolveAuthor(r *http.Request) string {
	if h.authorResolver != nil {
		if id := h.authorResolver(r); id != "" {
			return id
		}
	}
	return "system"
}

// RegisterRoutes registers the version routes under /api/v1.
func (h *VersionHandler) RegisterRoutes(router *mux.Router) {
	h.RegisterRoutesOn(router.PathPrefix("/api/v1").Subrouter())
}

// RegisterRoutesOn registers the version routes on an existing
// /api/v1 subrouter.
//
// internal/api/routes.go uses this so the version routes live in the
// same subrouter — and therefore the same ordered route table — as
// the rest of the API. gorilla/mux resolves routes in registration
// order, so keeping one table is what guarantees that
// "/workflows/{id}/versions" is never shadowed by "/workflows/{id}".
func (h *VersionHandler) RegisterRoutesOn(api *mux.Router) {
	// Static segments must be registered BEFORE the "{versionId}"
	// wildcard: gorilla/mux matches in registration order, so
	// "/workflows/{id}/versions/compare" would otherwise be read as
	// a version whose id is "compare".
	api.HandleFunc("/workflows/{id}/versions/compare", h.CompareVersions).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/{id}/versions", h.ListVersions).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/{id}/versions", h.CreateVersion).Methods("POST", "OPTIONS")
	api.HandleFunc("/workflows/{id}/versions/{versionId}", h.GetVersion).Methods("GET", "OPTIONS")
	api.HandleFunc("/workflows/{id}/versions/{versionId}", h.DeleteVersion).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/workflows/{id}/versions/{versionId}/restore", h.RestoreVersion).Methods("POST", "OPTIONS")
}

// ListVersions lists all versions for a workflow
func (h *VersionHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workflowID := vars["id"]

	// Parse query parameters
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit == 0 {
		limit = 50 // Default limit
	}
	if limit < 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// An unknown workflow is a 404, not an empty list — the UI has to
	// be able to tell "no snapshots yet" from "no such workflow".
	if !h.versionManager.WorkflowExists(workflowID) {
		writeAPIError(w, http.StatusNotFound, "Workflow not found")
		return
	}

	filters := VersionListFilters{
		WorkflowID: workflowID,
		Limit:      limit,
		Offset:     offset,
	}

	items, total, err := h.versionManager.ListVersions(filters)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*WorkflowVersion{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data":   items,
		"total":  total,
		"count":  len(items),
		"limit":  limit,
		"offset": offset,
	})
}

// CreateVersion creates a new version for a workflow
func (h *VersionHandler) CreateVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workflowID := vars["id"]

	var request VersionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !h.versionManager.WorkflowExists(workflowID) {
		writeAPIError(w, http.StatusNotFound, "Workflow not found")
		return
	}

	version, err := h.versionManager.CreateVersion(workflowID, h.resolveAuthor(r), &request)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(version)
}

// GetVersion retrieves a specific version
func (h *VersionHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workflowID := vars["id"]
	versionID := vars["versionId"]

	version, err := h.versionManager.GetVersion(versionID)
	if err != nil || version.WorkflowID != workflowID {
		writeAPIError(w, http.StatusNotFound, "Version not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(version)
}

// DeleteVersion deletes a version
func (h *VersionHandler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workflowID := vars["id"]
	versionID := vars["versionId"]

	version, err := h.versionManager.GetVersion(versionID)
	if err != nil || version.WorkflowID != workflowID {
		writeAPIError(w, http.StatusNotFound, "Version not found")
		return
	}

	if err := h.versionManager.DeleteVersion(versionID); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RestoreVersion restores a workflow to a specific version
func (h *VersionHandler) RestoreVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workflowID := vars["id"]
	versionID := vars["versionId"]

	version, err := h.versionManager.GetVersion(versionID)
	if err != nil || version.WorkflowID != workflowID {
		writeAPIError(w, http.StatusNotFound, "Version not found")
		return
	}

	var request VersionRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		// Use default values if no body provided
		request.CreateBackup = true
	}

	restored, err := h.versionManager.RestoreVersion(versionID, h.resolveAuthor(r), &request)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message":       "Workflow restored successfully",
		"restoredFrom":  restored,
		"backupCreated": request.CreateBackup,
	})
}

// CompareVersions compares two versions
func (h *VersionHandler) CompareVersions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workflowID := vars["id"]

	fromVersionID := r.URL.Query().Get("from")
	toVersionID := r.URL.Query().Get("to")

	if fromVersionID == "" || toVersionID == "" {
		writeAPIError(w, http.StatusBadRequest, "Both 'from' and 'to' query parameters are required")
		return
	}

	fromVersion, err := h.versionManager.GetVersion(fromVersionID)
	if err != nil || fromVersion.WorkflowID != workflowID {
		writeAPIError(w, http.StatusNotFound, "Version not found")
		return
	}
	toVersion, err := h.versionManager.GetVersion(toVersionID)
	if err != nil || toVersion.WorkflowID != workflowID {
		writeAPIError(w, http.StatusNotFound, "Version not found")
		return
	}

	comparison, err := h.versionManager.CompareVersions(fromVersionID, toVersionID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(comparison)
}

// writeAPIError writes the same JSON error envelope the rest of the
// m9m API uses: {"error": true, "message": "...", "code": 404}.
func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   true,
		"message": message,
		"code":    status,
	})
}
