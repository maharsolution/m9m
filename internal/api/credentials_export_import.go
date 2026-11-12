package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// maxBulkExportCredentials caps how many credentials a bulk export
// returns when no explicit id list is given. It mirrors the workflow
// bulk export cap (maxBulkExportWorkflows).
const maxBulkExportCredentials = 1000

// RegisterCredentialExportRoutes mounts the credential export/import
// routes on the /api/v1 subrouter.
//
// The routes are static paths and MUST be registered before
// "/credentials/{id}" — gorilla/mux matches routes in registration
// order, so "/credentials/{id}" would otherwise swallow
// "/credentials/export" and "/credentials/import".
//
// The intended call site is internal/api/routes.go, right before the
// "/credentials/{id}" route:
//
//	s.RegisterCredentialExportRoutes(api)
func (s *APIServer) RegisterCredentialExportRoutes(api *mux.Router) {
	api.HandleFunc("/credentials/export", s.ExportCredentials).Methods("GET", "POST", "OPTIONS")
	api.HandleFunc("/credentials/import", s.ImportCredentials).Methods("POST", "OPTIONS")
}

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

// ExportCredentials writes the credentials of this instance in the n8n
// export format (n8n CLI `export:credentials --separate` parity).
//
// GET  /api/v1/credentials/export?ids=id1,id2
// POST /api/v1/credentials/export  {"ids": ["id1", "id2"]}
//
// Without an id list every credential is exported (capped at
// maxBulkExportCredentials).
//
// The response is an ARRAY of n8n credential objects:
//
//	[{"id":"...","name":"...","type":"...","data":"<encrypted>"}]
//
// `data` holds the credential payload encrypted with the instance key
// in n8n's on-disk format (see internal/credentials/n8n_cipher.go and
// internal/credentials/export_import.go). The plaintext secrets never
// leave the instance: an m9m instance (or n8n) that is configured with
// the same N8N_ENCRYPTION_KEY can decrypt the file.
func (s *APIServer) ExportCredentials(w http.ResponseWriter, r *http.Request) {
	ids, err := s.parseExportIDs(r)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid export request", err)
		return
	}

	var credentialsList []*storage.Credential
	if len(ids) == 0 {
		listed, listErr := s.storage.ListCredentials()
		if listErr != nil {
			s.sendError(w, http.StatusInternalServerError, "Failed to list credentials", listErr)
			return
		}
		credentialsList = listed
	} else {
		var missing []string
		for _, id := range ids {
			credential, getErr := s.storage.GetCredential(id)
			if getErr != nil {
				missing = append(missing, id)
				continue
			}
			credentialsList = append(credentialsList, credential)
		}
		if len(missing) > 0 {
			s.sendError(w, http.StatusNotFound,
				fmt.Sprintf("Credential(s) not found: %s", strings.Join(missing, ", ")), nil)
			return
		}
	}

	if len(credentialsList) == 0 {
		s.sendError(w, http.StatusNotFound, "No credentials found", nil)
		return
	}
	if len(credentialsList) > maxBulkExportCredentials {
		credentialsList = credentialsList[:maxBulkExportCredentials]
	}

	cipher, err := credentials.InstanceCipher()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError,
			"The instance encryption key is not configured; credentials cannot be exported", err)
		return
	}

	exported := make([]credentials.ExportCredential, 0, len(credentialsList))
	for _, credential := range credentialsList {
		blob, encErr := credentials.EncryptCredentialData(cipher, credential.Data)
		if encErr != nil {
			s.sendError(w, http.StatusInternalServerError, "Failed to export credential", encErr)
			return
		}
		exported = append(exported, credentials.ExportCredential{
			ID:   credential.ID,
			Name: credential.Name,
			Type: credential.Type,
			Data: blob,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="credentials-export.json"`)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(exported)
}

// ---------------------------------------------------------------------------
// Import
// ---------------------------------------------------------------------------

// credentialImportEntry is one credential of an import request.
//
// `data` is either the encrypted n8n blob (a JSON string) or — only
// when the request explicitly asks for it — a plain JSON object.
type credentialImportEntry struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Decrypted bool            `json:"decrypted,omitempty"`
}

// credentialImportResult reports one imported credential. The secret
// payload is never part of the response.
type credentialImportResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// credentialImportError reports one rejected credential. A bad entry
// never aborts the rest of the batch.
type credentialImportError struct {
	Index   int    `json:"index"`
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
}

// credentialImportSummary is the response body of a credential import.
type credentialImportSummary struct {
	Imported    int                      `json:"imported"`
	Failed      int                      `json:"failed"`
	Credentials []credentialImportResult `json:"credentials"`
	Errors      []credentialImportError  `json:"errors"`
}

// importedCredentialSet remembers which credential of the imported
// bundle ended up under which id in this instance. It is what lets a
// workflow import re-link the node credential references of the bundle.
type importedCredentialSet struct {
	// BySourceID maps the id the credential carried in the bundle onto
	// the id it has in this instance.
	BySourceID map[string]credentialImportResult
	// ByNameType maps "type\x00name" onto the imported credential.
	ByNameType map[string]credentialImportResult
}

// credentialNameTypeKey builds the lookup key of a name+type pair.
func credentialNameTypeKey(credType, name string) string {
	return credType + "\x00" + name
}

// ImportCredentials imports credentials in the n8n format.
//
// POST /api/v1/credentials/import
//
// Accepted request bodies:
//
//	[{"id":"...","name":"...","type":"...","data":"<encrypted>"}]   (n8n array)
//	 {"id":"...","name":"...","type":"...","data":"<encrypted>"}   (single object)
//	 {"credentials":[...], "decrypted":false}                       (envelope)
//
// `data` is the n8n blob: the credential payload encrypted with the
// instance key. It is decrypted with N8N_ENCRYPTION_KEY before it is
// stored.
//
// A PLAIN JSON object in `data` is only accepted when the request
// explicitly asks for it (`"decrypted": true` in the envelope, on the
// entry itself, or `?decrypted=true` on the query string). An encrypted
// blob is never silently treated as plain JSON.
//
// The response lists the created credentials (id / name / type) so a
// workflow import can re-link the node credential references.
func (s *APIServer) ImportCredentials(w http.ResponseWriter, r *http.Request) {
	payload, err := s.readCredentialImportPayload(w, r)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Failed to read the import payload", err)
		return
	}

	entries, requestPlain, err := parseCredentialImportPayload(payload)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid import payload", err)
		return
	}

	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("decrypted")), "true") {
		requestPlain = true
	}

	summary, _ := s.importCredentialEntries(entries, requestPlain)

	if summary.Imported == 0 {
		message := "No credential was imported"
		if len(summary.Errors) > 0 {
			message = fmt.Sprintf("%s: %s", message, summary.Errors[0].Message)
		}
		s.sendJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":       true,
			"message":     message,
			"code":        http.StatusBadRequest,
			"imported":    summary.Imported,
			"failed":      summary.Failed,
			"credentials": summary.Credentials,
			"errors":      summary.Errors,
			"summary":     summary,
		})
		return
	}

	s.sendJSON(w, http.StatusCreated, summary)
}

// readCredentialImportPayload reads the request body of a credential
// import. The body is capped at maxImportPayloadBytes.
func (s *APIServer) readCredentialImportPayload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, errors.New("the request has no body")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportPayloadBytes)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read the request body: %w", err)
	}
	if int64(len(data)) > maxImportPayloadBytes {
		return nil, importPayloadTooLargeError()
	}
	return data, nil
}

// parseCredentialImportPayload splits the request body into the
// credential entries and reads the "decrypted" flag of the envelope
// form.
func parseCredentialImportPayload(data []byte) ([]credentialImportEntry, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, false, errors.New("the request body is empty")
	}

	switch trimmed[0] {
	case '[':
		var entries []credentialImportEntry
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, false, fmt.Errorf("the request body is not a valid JSON array: %w", err)
		}
		return entries, false, nil
	case '{':
		var envelope struct {
			Credentials []credentialImportEntry `json:"credentials"`
			Decrypted   bool                    `json:"decrypted"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err == nil && envelope.Credentials != nil {
			return envelope.Credentials, envelope.Decrypted, nil
		}

		var entry credentialImportEntry
		if err := json.Unmarshal(trimmed, &entry); err != nil {
			return nil, false, fmt.Errorf("the request body is not a valid credential object: %w", err)
		}
		if len(entry.Data) == 0 {
			return nil, false, errors.New("the credential has no data payload")
		}
		return []credentialImportEntry{entry}, entry.Decrypted, nil
	default:
		return nil, false, errors.New("the request body must be a credential object or an array of credential objects")
	}
}

// importCredentialEntries stores every entry of a credential bundle.
//
// The second return value maps the credentials of the bundle onto the
// credentials of this instance (by source id and by name+type) so a
// workflow import can re-link the node references.
func (s *APIServer) importCredentialEntries(entries []credentialImportEntry, allowPlain bool) (*credentialImportSummary, *importedCredentialSet) {
	summary := &credentialImportSummary{
		Credentials: []credentialImportResult{},
		Errors:      []credentialImportError{},
	}
	imported := &importedCredentialSet{
		BySourceID: map[string]credentialImportResult{},
		ByNameType: map[string]credentialImportResult{},
	}

	cipher, cipherErr := credentials.InstanceCipher()

	for index, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		credType := strings.TrimSpace(entry.Type)

		if name == "" {
			summary.Errors = append(summary.Errors, credentialImportError{
				Index:   index,
				Type:    credType,
				Message: "name is required",
			})
			continue
		}
		if credType == "" {
			summary.Errors = append(summary.Errors, credentialImportError{
				Index:   index,
				Name:    name,
				Message: "type is required",
			})
			continue
		}

		data, dataErr := decodeCredentialPayload(cipher, cipherErr, entry, allowPlain)
		if dataErr != nil {
			summary.Errors = append(summary.Errors, credentialImportError{
				Index:   index,
				Name:    name,
				Type:    credType,
				Message: dataErr.Error(),
			})
			continue
		}

		credential := &storage.Credential{
			ID:        strings.TrimSpace(entry.ID),
			Name:      name,
			Type:      credType,
			Data:      data,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}

		localID, idErr := s.resolveImportedCredentialID(credential.ID)
		if idErr != nil {
			summary.Errors = append(summary.Errors, credentialImportError{
				Index:   index,
				Name:    name,
				Type:    credType,
				Message: idErr.Error(),
			})
			continue
		}
		credential.ID = localID

		if err := s.storage.SaveCredential(credential); err != nil {
			summary.Errors = append(summary.Errors, credentialImportError{
				Index:   index,
				Name:    name,
				Type:    credType,
				Message: fmt.Sprintf("failed to store the credential: %v", err),
			})
			continue
		}

		// Mirror into the in-memory store the engine reads at execution
		// time, exactly like CreateCredential does.
		if s.credMgr != nil {
			s.credMgr.UpsertCredential(credential)
		}

		result := credentialImportResult{ID: credential.ID, Name: credential.Name, Type: credential.Type}
		summary.Credentials = append(summary.Credentials, result)
		if entry.ID != "" {
			imported.BySourceID[strings.TrimSpace(entry.ID)] = result
		}
		imported.ByNameType[credentialNameTypeKey(credType, name)] = result
	}

	summary.Imported = len(summary.Credentials)
	summary.Failed = len(summary.Errors)
	return summary, imported
}

// resolveImportedCredentialID keeps the id of the source credential
// when it is free in this instance and mints a new one otherwise.
//
// Keeping the id is what makes the n8n workflow references (which carry
// the source id) resolve without a lookup table; the collision case is
// covered by the id mapping of importedCredentialSet.
func (s *APIServer) resolveImportedCredentialID(sourceID string) (string, error) {
	if sourceID == "" {
		return "", nil
	}
	if _, err := s.storage.GetCredential(sourceID); err == nil {
		// The id is taken by another credential of this instance.
		return "", nil
	}
	return sourceID, nil
}

// decodeCredentialPayload turns the `data` field of an import entry
// into the credential data map.
//
// A JSON string is the encrypted n8n blob and is always decrypted. A
// JSON object is plain data and is only accepted when the request
// explicitly asked for it.
func decodeCredentialPayload(cipher *credentials.N8NCipher, cipherErr error, entry credentialImportEntry, allowPlain bool) (map[string]interface{}, error) {
	raw := bytes.TrimSpace(entry.Data)
	if len(raw) == 0 {
		return nil, errors.New("the credential has no data payload")
	}

	switch raw[0] {
	case '"':
		var blob string
		if err := json.Unmarshal(raw, &blob); err != nil {
			return nil, fmt.Errorf("failed to read the encrypted data payload: %w", err)
		}
		if cipherErr != nil {
			return nil, cipherErr
		}
		data, err := credentials.DecryptCredentialData(cipher, blob)
		if err != nil {
			return nil, err
		}
		return data, nil
	case '{':
		var data map[string]interface{}
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("failed to read the data payload: %w", err)
		}
		if !allowPlain && !entry.Decrypted {
			return nil, errors.New("the data payload is a plain JSON object; set \"decrypted\": true to import it as plaintext")
		}
		return data, nil
	default:
		return nil, errors.New("the data payload must be an encrypted string or a JSON object")
	}
}

// ---------------------------------------------------------------------------
// Workflow bundle helpers
// ---------------------------------------------------------------------------

// parseImportBundlePayload splits an import payload into the workflow
// entries, the credential entries of a bundle envelope and the
// optional activeState / decrypted flags.
//
// Accepted shapes:
//
//	{...}                                             one workflow
//	[{...}, {...}]                                    an array of workflows
//	{"workflows":[...], "activeState":"fromJson"}     the plain envelope
//	{"workflows":[...], "credentials":[...], ...}     the bundle envelope
//
// The credential entries are the n8n credential objects; their `data`
// field is decrypted by importCredentialEntries.
func parseImportBundlePayload(data []byte) (entries []json.RawMessage, credentialEntries []credentialImportEntry, honourActive bool, plainCredentials bool, err error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil, false, false, errors.New("the request body is empty")
	}

	switch trimmed[0] {
	case '[':
		var rawEntries []json.RawMessage
		if unmarshalErr := json.Unmarshal(trimmed, &rawEntries); unmarshalErr != nil {
			return nil, nil, false, false, fmt.Errorf("the request body is not a valid JSON array: %w", unmarshalErr)
		}
		return rawEntries, nil, false, false, nil
	case '{':
		var envelope struct {
			Workflows   []json.RawMessage       `json:"workflows"`
			Credentials []credentialImportEntry `json:"credentials"`
			ActiveState string                  `json:"activeState"`
			Active      string                  `json:"active"`
			Decrypted   bool                    `json:"decrypted"`
		}
		if unmarshalErr := json.Unmarshal(trimmed, &envelope); unmarshalErr == nil && envelope.Workflows != nil {
			honour := isFromJson(envelope.ActiveState) || isFromJson(envelope.Active)
			return envelope.Workflows, envelope.Credentials, honour, envelope.Decrypted, nil
		}
		return []json.RawMessage{json.RawMessage(trimmed)}, nil, false, false, nil
	default:
		return nil, nil, false, false, errors.New("the request body must be a workflow object or an array of workflow objects")
	}
}

// workflowCredentialBundle is the envelope of a workflow export that
// carries the credentials of the workflow.
//
// It is the closest m9m equivalent of the n8n CLI export with
// `--separate`: n8n writes two files (workflows + credentials), m9m
// writes both in one envelope so a single POST can restore a workflow
// on another instance.
type workflowCredentialBundle struct {
	// Workflows holds the n8n workflow objects (an array, exactly like
	// the n8n workflow export file).
	Workflows []json.RawMessage `json:"workflows"`
	// Credentials holds the encrypted n8n credential objects of the
	// credentials the workflow references.
	Credentials []credentials.ExportCredential `json:"credentials"`
}

// buildWorkflowCredentialBundle assembles the export envelope of one
// workflow: the n8n workflow document plus the encrypted credentials
// the workflow references.
func (s *APIServer) buildWorkflowCredentialBundle(workflow *model.Workflow, workflowJSON []byte) ([]byte, error) {
	cipher, err := credentials.InstanceCipher()
	if err != nil {
		return nil, err
	}

	exported, err := s.exportReferencedCredentials(workflow, cipher)
	if err != nil {
		return nil, err
	}

	bundle := workflowCredentialBundle{
		Workflows:   []json.RawMessage{json.RawMessage(workflowJSON)},
		Credentials: exported,
	}
	return json.MarshalIndent(bundle, "", "  ")
}

// exportReferencedCredentials encrypts every credential the workflow
// references. A credential that cannot be found in this instance is
// skipped — the workflow export keeps its reference and the import on
// the other side reports the usual "could not be resolved" warning.
func (s *APIServer) exportReferencedCredentials(workflow *model.Workflow, cipher *credentials.N8NCipher) ([]credentials.ExportCredential, error) {
	exported := []credentials.ExportCredential{}
	seen := map[string]bool{}

	for _, node := range workflow.Nodes {
		for credType, ref := range node.Credentials {
			key := credentialNameTypeKey(credType, ref.Name)
			if ref.ID != "" {
				key = credType + "\x00" + ref.ID
			}
			if seen[key] {
				continue
			}
			seen[key] = true

			credential, err := s.lookupCredentialForExport(ref.ID, ref.Name, credType)
			if err != nil || credential == nil {
				continue
			}

			blob, encErr := credentials.EncryptCredentialData(cipher, credential.Data)
			if encErr != nil {
				return nil, encErr
			}
			exported = append(exported, credentials.ExportCredential{
				ID:   credential.ID,
				Name: credential.Name,
				Type: credential.Type,
				Data: blob,
			})
		}
	}

	return exported, nil
}

// lookupCredentialForExport finds a credential by id (and type) and
// falls back to a unique name+type match.
func (s *APIServer) lookupCredentialForExport(id, name, credType string) (*storage.Credential, error) {
	if id != "" {
		credential, err := s.storage.GetCredential(id)
		if err == nil && credential.Type == credType {
			return credential, nil
		}
	}

	if name == "" {
		return nil, nil
	}

	listed, err := s.storage.ListCredentials()
	if err != nil {
		return nil, err
	}
	var match *storage.Credential
	for _, credential := range listed {
		if credential.Name != name || credential.Type != credType {
			continue
		}
		if match != nil {
			// Ambiguous name — n8n would not pick one either.
			return nil, nil
		}
		match = credential
	}
	return match, nil
}

// resolveCredentialRefWithImported returns a credential resolver that
// first consults the credentials of an imported bundle and falls back
// to the local credential store.
//
// The bundle lookup comes first because the id of an imported
// credential can differ from the id it carried in the source instance
// (id collision) — the local store would then resolve the reference to
// the wrong credential.
func (s *APIServer) resolveCredentialRefWithImported(imported *importedCredentialSet) func(credType, name, id string) (string, string, bool) {
	return func(credType, name, id string) (string, string, bool) {
		if imported != nil {
			if id != "" {
				if result, ok := imported.BySourceID[id]; ok && result.Type == credType {
					return result.ID, result.Name, true
				}
			}
			if name != "" {
				if result, ok := imported.ByNameType[credentialNameTypeKey(credType, name)]; ok {
					return result.ID, result.Name, true
				}
			}
		}
		return s.resolveCredentialRef(credType, name, id)
	}
}
