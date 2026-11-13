package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/neul-labs/m9m/internal/compatibility"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// maxImportPayloadBytes caps the size of an uploaded/imported workflow
// document (32 MiB). It is a variable so the tests can lower the cap.
var maxImportPayloadBytes int64 = 32 << 20

// errImportPayloadTooLarge marks a payload that exceeds
// maxImportPayloadBytes. The endpoint answers 413 for it.
var errImportPayloadTooLarge = errors.New("the import payload is too large")

// maxBulkExportWorkflows caps how many workflows a bulk export returns
// when no explicit id list is given.
const maxBulkExportWorkflows = 1000

// WorkflowExportItem describes one workflow inside an export request.
type WorkflowExportRequest struct {
	IDs []string `json:"ids"`
}

// WorkflowImportItem reports the result of one imported workflow.
type WorkflowImportItem struct {
	Index     int      `json:"index"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	SourceID  string   `json:"sourceId,omitempty"`
	VersionID string   `json:"versionId,omitempty"`
	Active    bool     `json:"active"`
	Warnings  []string `json:"warnings,omitempty"`
}

// WorkflowImportError reports one rejected workflow of a batch. A bad
// entry never aborts the rest of the batch.
type WorkflowImportError struct {
	Index   int    `json:"index"`
	Name    string `json:"name,omitempty"`
	Message string `json:"message"`
}

// WorkflowImportSummary is the response body of an import request.
type WorkflowImportSummary struct {
	Imported    int                   `json:"imported"`
	Failed      int                   `json:"failed"`
	ActiveState string                `json:"activeState"`
	Workflows   []WorkflowImportItem  `json:"workflows"`
	Errors      []WorkflowImportError `json:"errors"`
	// Credentials reports the credential import of a bundle envelope.
	// It is nil for a plain workflow import.
	Credentials *credentialImportSummary `json:"credentials,omitempty"`
}

// ExportWorkflow writes one workflow in n8n format.
//
// GET /api/v1/workflows/{id}/export
//
// The body is a single n8n workflow object (n8n editor "Download"
// parity). The response carries `Content-Type: application/json` and
// `Content-Disposition: attachment; filename="<slug>.json"`.
func (s *APIServer) ExportWorkflow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	workflow, err := s.storage.GetWorkflow(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	importer := compatibility.NewN8nWorkflowImporter()
	data, err := importer.ExportWorkflowWithOptions(workflow, compatibility.ExportOptions{
		TagIDResolver: s.resolveTagID,
	})
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to export workflow", err)
		return
	}

	filename := workflowSlug(workflow.Name)
	if filename == "" {
		filename = workflowSlug(workflow.ID)
	}
	if filename == "" {
		// A name that carries no usable characters still has to end up
		// in a valid file name.
		filename = "workflow"
	}

	// Bundle mode: the export carries the credentials of the workflow
	// as well, encrypted with the instance key. The envelope is
	//
	//	{"workflows":[<workflow>], "credentials":[<credential>, ...]}
	//
	// which is the m9m equivalent of the n8n CLI export with
	// `--separate` (n8n writes the two documents into two files, m9m
	// keeps them together so one POST restores the workflow).
	if includeCredentialsRequested(r) {
		bundle, bundleErr := s.buildWorkflowCredentialBundle(workflow, data)
		if bundleErr != nil {
			s.sendError(w, http.StatusInternalServerError, "Failed to export the workflow bundle", bundleErr)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename+".json"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bundle)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename+".json"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// includeCredentialsRequested reports whether the caller asked for the
// bundle mode of a workflow export (`?includeCredentials=true`).
func includeCredentialsRequested(r *http.Request) bool {
	value := strings.TrimSpace(r.URL.Query().Get("includeCredentials"))
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// ExportWorkflows writes a list of workflows in n8n format.
//
// GET  /api/v1/workflows/export?ids=id1,id2
// POST /api/v1/workflows/export  {"ids": ["id1", "id2"]}
//
// The body is always an ARRAY of n8n workflow objects, also for a
// single id (n8n CLI `export:workflow` parity). Without an id list
// every workflow is exported.
func (s *APIServer) ExportWorkflows(w http.ResponseWriter, r *http.Request) {
	ids, err := s.parseExportIDs(r)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid export request", err)
		return
	}

	var workflows []*model.Workflow
	if len(ids) == 0 {
		listed, _, listErr := s.storage.ListWorkflows(storage.WorkflowFilters{Limit: maxBulkExportWorkflows})
		if listErr != nil {
			s.sendError(w, http.StatusInternalServerError, "Failed to list workflows", listErr)
			return
		}
		workflows = listed
	} else {
		var missing []string
		for _, id := range ids {
			workflow, getErr := s.storage.GetWorkflow(id)
			if getErr != nil {
				missing = append(missing, id)
				continue
			}
			workflows = append(workflows, workflow)
		}
		if len(missing) > 0 {
			s.sendError(w, http.StatusNotFound,
				fmt.Sprintf("Workflow(s) not found: %s", strings.Join(missing, ", ")), nil)
			return
		}
	}

	if len(workflows) == 0 {
		s.sendError(w, http.StatusNotFound, "No workflows found", nil)
		return
	}

	importer := compatibility.NewN8nWorkflowImporter()
	exported := make([]*model.Workflow, 0, len(workflows))
	for _, workflow := range workflows {
		exported = append(exported, workflow)
	}

	data, err := importer.ExportWorkflowsWithOptions(exported, compatibility.ExportOptions{
		TagIDResolver: s.resolveTagID,
	})
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to export workflows", err)
		return
	}

	filename := "workflows-export.json"

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ImportWorkflows imports n8n workflows into m9m.
//
// POST /api/v1/workflows/import
//
// The request body may be
//   - multipart/form-data with a .json file field ("file", or any
//     uploaded file),
//   - a JSON workflow object,
//   - a JSON array of workflow objects,
//   - a JSON envelope {"workflows": [...], "activeState": "fromJson"}.
//
// Every entry must carry BOTH a "nodes" and a "connections" property,
// otherwise the entry is rejected with a clear message. One bad entry
// never aborts the rest of the batch: the response reports the
// imported workflows and the per-item errors.
//
// Imported workflows are inactive by default. The JSON "active" flag
// is only honoured when the request explicitly asks for it
// (`?active=fromJson`, `?activeState=fromJson`, or the envelope's
// "activeState" field) - n8n's `activeState: fromJson` semantics.
func (s *APIServer) ImportWorkflows(w http.ResponseWriter, r *http.Request) {
	payload, err := s.readImportPayload(w, r)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errImportPayloadTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		s.sendError(w, status, "Failed to read the import payload", err)
		return
	}

	entries, credentialEntries, honourActive, plainCredentials, err := parseImportBundlePayload(payload)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid import payload", err)
		return
	}

	if honourActive || importHonoursActive(r) {
		honourActive = true
	}

	// A bundle envelope carries the credentials of the workflows. They
	// are imported FIRST so the node credential references can be
	// resolved against them - a credential that is imported afterwards
	// would leave the workflow with unresolved references (and the
	// usual "could not be resolved" warning).
	var importedCredentials *importedCredentialSet
	var credentialSummary *credentialImportSummary
	if len(credentialEntries) > 0 {
		credentialSummary, importedCredentials = s.importCredentialEntries(credentialEntries, plainCredentials)
	}

	importer := compatibility.NewN8nWorkflowImporter()
	importOptions := compatibility.ImportOptions{
		HonourActive: honourActive,
		// The resolver consults the credentials of the bundle first and
		// falls back to the local credential store.
		CredentialResolver: s.resolveCredentialRefWithImported(importedCredentials),
		IDExists: func(id string) bool {
			if id == "" {
				return false
			}
			_, getErr := s.storage.GetWorkflow(id)
			return getErr == nil
		},
		KnownNodeTypes: s.knownNodeTypes(),
		// n8n's editor names a node that arrives without a name
		// after the node type display name.
		NodeDisplayNames: s.nodeDisplayNames(),
	}

	summary := WorkflowImportSummary{
		ActiveState: activeStateValue(honourActive),
		Workflows:   []WorkflowImportItem{},
		Errors:      []WorkflowImportError{},
		Credentials: credentialSummary,
	}

	for index, entry := range entries {
		name := importEntryName(entry)

		if err := importer.ValidateN8nWorkflow(entry); err != nil {
			summary.Errors = append(summary.Errors, WorkflowImportError{
				Index:   index,
				Name:    name,
				Message: err.Error(),
			})
			continue
		}

		result, err := importer.ImportWorkflowWithOptions(entry, importOptions)
		if err != nil {
			summary.Errors = append(summary.Errors, WorkflowImportError{
				Index:   index,
				Name:    name,
				Message: err.Error(),
			})
			continue
		}

		workflow := result.Workflow
		// An imported workflow has no Debug flag of its own - n8n does
		// not know the concept. It always lands with Debug OFF.
		workflow.Debug = false
		if err := validateWorkflow(workflow); err != nil {
			summary.Errors = append(summary.Errors, WorkflowImportError{
				Index:   index,
				Name:    name,
				Message: err.Error(),
			})
			continue
		}

		if err := s.storage.SaveWorkflow(workflow); err != nil {
			summary.Errors = append(summary.Errors, WorkflowImportError{
				Index:   index,
				Name:    name,
				Message: fmt.Sprintf("failed to store the workflow: %v", err),
			})
			continue
		}

		// An imported workflow may already be active (only with
		// activeState=fromJson). Register its webhooks so trigger
		// nodes work right away, like CreateWorkflow does.
		if s.webhookManager != nil && workflow.Active {
			if regErr := s.webhookManager.RegisterWorkflowWebhooks(workflow, false); regErr != nil {
				fmt.Printf("[m9m] warning: failed to register webhooks for imported workflow %s: %v\n", workflow.ID, regErr)
			}
		}

		summary.Workflows = append(summary.Workflows, WorkflowImportItem{
			Index:     index,
			ID:        workflow.ID,
			Name:      workflow.Name,
			SourceID:  result.SourceID,
			VersionID: result.VersionID,
			Active:    workflow.Active,
			Warnings:  importWarnings(result),
		})
	}

	summary.Imported = len(summary.Workflows)
	summary.Failed = len(summary.Errors)

	if summary.Imported == 0 {
		message := "No workflow was imported"
		if len(summary.Errors) > 0 {
			message = fmt.Sprintf("%s: %s", message, summary.Errors[0].Message)
		}
		// The summary fields are repeated at the top level so a client
		// can read the per-item errors from a failed response without
		// digging into the nested "summary" object.
		s.sendJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":       true,
			"message":     message,
			"code":        http.StatusBadRequest,
			"imported":    summary.Imported,
			"failed":      summary.Failed,
			"activeState": summary.ActiveState,
			"workflows":   summary.Workflows,
			"errors":      summary.Errors,
			"summary":     summary,
		})
		return
	}

	s.sendJSON(w, http.StatusCreated, summary)
}

// readImportPayload reads the workflow document from a multipart upload
// or from the raw request body. The body is capped at
// maxImportPayloadBytes; an oversized payload is reported as
// errImportPayloadTooLarge instead of being silently truncated.
func (s *APIServer) readImportPayload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Body != nil {
		// MaxBytesReader aborts the read as soon as the cap is hit, so
		// an oversized upload never reaches the JSON decoder.
		r.Body = http.MaxBytesReader(w, r.Body, maxImportPayloadBytes)
	}

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxImportPayloadBytes); err != nil {
			if isPayloadTooLarge(err) {
				return nil, importPayloadTooLargeError()
			}
			return nil, fmt.Errorf("failed to parse the multipart form: %w", err)
		}
		if r.MultipartForm == nil {
			return nil, errors.New("the multipart form is empty")
		}

		// Prefer the "file" field, fall back to any uploaded file.
		candidates := r.MultipartForm.File["file"]
		if len(candidates) == 0 {
			for _, files := range r.MultipartForm.File {
				candidates = files
				break
			}
		}
		if len(candidates) == 0 {
			return nil, errors.New("the multipart request has no file field")
		}

		file, err := candidates[0].Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open the uploaded file: %w", err)
		}
		defer file.Close()

		// Read one byte past the cap so an oversized file is detected
		// instead of being truncated into invalid JSON.
		data, err := io.ReadAll(io.LimitReader(file, maxImportPayloadBytes+1))
		if err != nil {
			return nil, importPayloadReadError(err)
		}
		if int64(len(data)) > maxImportPayloadBytes {
			return nil, importPayloadTooLargeError()
		}
		return data, nil
	}

	if r.Body == nil {
		return nil, errors.New("the request has no body")
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, importPayloadReadError(err)
	}
	if int64(len(data)) > maxImportPayloadBytes {
		return nil, importPayloadTooLargeError()
	}
	return data, nil
}

// importPayloadTooLargeError builds the error for an oversized import
// payload.
func importPayloadTooLargeError() error {
	return fmt.Errorf("%w: the limit is %d MiB", errImportPayloadTooLarge, maxImportPayloadBytes>>20)
}

// importPayloadReadError maps a body read error onto the
// errImportPayloadTooLarge sentinel when the cap was hit.
func importPayloadReadError(err error) error {
	if isPayloadTooLarge(err) {
		return importPayloadTooLargeError()
	}
	return fmt.Errorf("failed to read the request body: %w", err)
}

// isPayloadTooLarge reports whether the error was caused by the
// payload cap.
func isPayloadTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

// parseImportPayload splits the payload into the workflow entries and
// reads the optional activeState of the envelope form.
func parseImportPayload(data []byte) ([]json.RawMessage, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, false, errors.New("the request body is empty")
	}

	switch trimmed[0] {
	case '[':
		var entries []json.RawMessage
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, false, fmt.Errorf("the request body is not a valid JSON array: %w", err)
		}
		return entries, false, nil
	case '{':
		var envelope struct {
			Workflows   []json.RawMessage `json:"workflows"`
			ActiveState string            `json:"activeState"`
			Active      string            `json:"active"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err == nil && envelope.Workflows != nil {
			return envelope.Workflows, isFromJson(envelope.ActiveState) || isFromJson(envelope.Active), nil
		}
		return []json.RawMessage{json.RawMessage(trimmed)}, false, nil
	default:
		return nil, false, errors.New("the request body must be a workflow object or an array of workflow objects")
	}
}

// importHonoursActive reports whether the request explicitly asks for
// the JSON "active" flag to be honoured.
func importHonoursActive(r *http.Request) bool {
	query := r.URL.Query()
	return isFromJson(query.Get("activeState")) || isFromJson(query.Get("active"))
}

func isFromJson(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "fromJson")
}

func activeStateValue(honourActive bool) string {
	if honourActive {
		return "fromJson"
	}
	return "false"
}

// importEntryName reads the workflow name of an entry for error
// messages. It never fails.
func importEntryName(entry json.RawMessage) string {
	var probe struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(entry, &probe); err != nil {
		return ""
	}
	return probe.Name
}

// importWarnings turns the conversion issues of an import into
// human-readable warnings.
func importWarnings(result *compatibility.ImportResult) []string {
	var warnings []string
	for _, issue := range result.ConversionIssues {
		if issue.Type != "warning" {
			continue
		}
		message := issue.Message
		if issue.NodeID != "" {
			message = fmt.Sprintf("node %s: %s", issue.NodeID, message)
		}
		warnings = append(warnings, message)
	}
	return warnings
}

// resolveCredentialRef maps an n8n credential reference onto a
// credential that exists in m9m.
//
// n8n resolves a credential reference in two steps
// (`workflow-helpers.ts`, `replaceInvalidCredentials`):
//   - a reference that carries an id is resolved by id AND type
//     (`findOneBy({ id, type })`),
//   - a reference without an id (the legacy string shape or an
//     explicit `null` id) is resolved by name AND type, and only a
//     unique match wins.
//
// The resolved name is the one m9m stores, so the exported document
// carries the same name as the credential it points at. Only the id
// and the name are read - the secret data of a credential is never
// touched.
func (s *APIServer) resolveCredentialRef(credType, name, id string) (string, string, bool) {
	credentials, err := s.storage.ListCredentials()
	if err != nil {
		return "", "", false
	}

	if id != "" {
		for _, credential := range credentials {
			// The type is part of the lookup: a credential id that
			// belongs to another credential type must not be reused.
			if credential.ID == id && credential.Type == credType {
				return credential.ID, credential.Name, true
			}
		}
	}

	if name == "" {
		return "", "", false
	}

	var match *storage.Credential
	for _, credential := range credentials {
		if credential.Name != name || credential.Type != credType {
			continue
		}
		if match != nil {
			// Ambiguous name: n8n keeps the id empty as well.
			return "", "", false
		}
		match = credential
	}
	if match == nil {
		return "", "", false
	}
	return match.ID, match.Name, true
}

// resolveTagID maps a tag name onto the tag id m9m stores for it.
func (s *APIServer) resolveTagID(name string) (string, bool) {
	tags, err := s.storage.ListTags()
	if err != nil {
		return "", false
	}
	for _, tag := range tags {
		if tag.Name == name {
			return tag.ID, true
		}
	}
	return "", false
}

// knownNodeTypes returns the node types m9m implements. It returns nil
// when the catalogue is unavailable, which disables the check.
func (s *APIServer) knownNodeTypes() map[string]bool {
	if s.engine == nil {
		return nil
	}
	infos := s.engine.GetRegisteredNodeTypes()
	if len(infos) == 0 {
		return nil
	}
	known := make(map[string]bool, len(infos))
	for _, info := range infos {
		known[info.TypeID] = true
	}
	return known
}

// nodeDisplayNames maps a node type onto its display name. n8n uses
// the display name to name nodes that arrive without a name.
func (s *APIServer) nodeDisplayNames() map[string]string {
	if s.engine == nil {
		return nil
	}
	infos := s.engine.GetRegisteredNodeTypes()
	if len(infos) == 0 {
		return nil
	}
	names := make(map[string]string, len(infos))
	for _, info := range infos {
		if info.DisplayName != "" {
			names[info.TypeID] = info.DisplayName
		}
	}
	return names
}

// parseExportIDs reads the requested workflow ids from the query string
// (GET) or from the JSON body (POST).
func (s *APIServer) parseExportIDs(r *http.Request) ([]string, error) {
	var raw []string

	if r.Method == http.MethodPost && r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxImportPayloadBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to read the request body: %w", err)
		}
		trimmed := bytes.TrimSpace(body)
		if len(trimmed) > 0 {
			var request WorkflowExportRequest
			if err := json.Unmarshal(trimmed, &request); err != nil {
				var list []string
				if listErr := json.Unmarshal(trimmed, &list); listErr != nil {
					return nil, fmt.Errorf("the request body must be {\"ids\": [...]} or [\"id\"]: %w", err)
				}
				request.IDs = list
			}
			raw = append(raw, request.IDs...)
		}
	}

	if query := r.URL.Query().Get("ids"); query != "" {
		raw = append(raw, strings.Split(query, ",")...)
	}

	ids := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// workflowSlug turns a workflow name into a safe file name.
func workflowSlug(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(trimmed) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if builder.Len() > 0 && !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return slug
}
