package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/storage"
)

func newTestServer(t *testing.T) *APIServer {
	t.Helper()
	store := storage.NewMemoryStorage()
	return &APIServer{
		storage: store,
		config:  DefaultAPIServerConfig(),
	}
}

// withVars attaches mux path variables to the request so handlers that
// call mux.Vars can find them. We rebuild the request rather than
// touching r in-place to keep the function side-effect free.
func withVars(req *http.Request, vars map[string]string) *http.Request {
	return mux.SetURLVars(req, vars)
}

func doJSON(t *testing.T, srv *APIServer, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	switch {
	case method == http.MethodGet && path == "/api/v1/credentials":
		srv.ListCredentials(w, req)
	case method == http.MethodPost && path == "/api/v1/credentials":
		srv.CreateCredential(w, req)
	case method == http.MethodGet && path == "/api/v1/credentials/types":
		srv.ListCredentialTypes(w, req)
	case method == http.MethodPost && path == "/api/v1/credentials/test":
		srv.TestCredentialPayload(w, req)
	case method == http.MethodGet && strings.HasPrefix(path, "/api/v1/credentials/schema/"):
		credType := strings.TrimPrefix(path, "/api/v1/credentials/schema/")
		srv.GetCredentialSchema(w, withVars(req, map[string]string{"type": credType}))
	case method == http.MethodGet && strings.HasPrefix(path, "/api/v1/credentials/") && !strings.Contains(path, "/test"):
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		srv.GetCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodPost && strings.HasSuffix(path, "/test"):
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		id = strings.TrimSuffix(id, "/test")
		srv.TestCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodPut && strings.HasSuffix(path, "/transfer"):
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		id = strings.TrimSuffix(id, "/transfer")
		srv.TransferCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodPatch:
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		srv.PatchCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodPut:
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		srv.UpdateCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodDelete:
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		srv.DeleteCredential(w, withVars(req, map[string]string{"id": id}))
	}
	return w
}

func TestCreateAndGetCredential(t *testing.T) {
	srv := newTestServer(t)

	createBody := map[string]any{
		"name": "mybasic",
		"type": "httpBasicAuth",
		"data": map[string]any{"user": "admin", "password": "secret"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", w.Code, w.Body.String())
	}

	var created CredentialResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID == "" || created.Name != "mybasic" || created.Type != "httpBasicAuth" {
		t.Fatalf("unexpected envelope: %+v", created)
	}

	// GET of a single credential returns the stored `data` so the UI's
	// edit form can be pre-filled with the real values (n8n editor
	// parity). The LIST endpoint stays data-free.
	w = doJSON(t, srv, http.MethodGet, "/api/v1/credentials/"+created.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: status %d", w.Code)
	}
	var fetched struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode fetched: %v", err)
	}
	if fetched.Data["user"] != "admin" || fetched.Data["password"] != "secret" {
		t.Fatalf("single GET must expose data fields, got %+v", fetched.Data)
	}

	// The list envelope must NOT carry the secrets.
	w = doJSON(t, srv, http.MethodGet, "/api/v1/credentials", nil)
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("data leaked in LIST response: %s", w.Body.String())
	}
}

// TestCredentialPatchMergesDataFields guards the "data fields are
// honoured" contract: a PATCH that only carries the fields the user
// touched must not wipe the fields they left alone.
func TestCredentialPatchMergesDataFields(t *testing.T) {
	srv := newTestServer(t)

	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", map[string]any{
		"name": "pg",
		"type": "postgres",
		"data": map[string]any{
			"host":     "db.example.com",
			"port":     5432,
			"database": "prod",
			"user":     "admin",
			"password": "secret",
		},
	})
	var created CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	// Patch only the password.
	w = doJSON(t, srv, http.MethodPatch, "/api/v1/credentials/"+created.ID,
		map[string]any{"data": map[string]any{"password": "new-secret"}})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: status %d body %s", w.Code, w.Body.String())
	}
	var patched struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched.Data["password"] != "new-secret" {
		t.Fatalf("password not updated: %+v", patched.Data)
	}
	if patched.Data["user"] != "admin" || patched.Data["host"] != "db.example.com" ||
		patched.Data["database"] != "prod" {
		t.Fatalf("untouched fields were wiped: %+v", patched.Data)
	}

	// An empty data map must not erase the stored secrets either.
	w = doJSON(t, srv, http.MethodPatch, "/api/v1/credentials/"+created.ID,
		map[string]any{"name": "pg-renamed", "data": map[string]any{}})
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched.Data["password"] != "new-secret" || patched.Data["user"] != "admin" {
		t.Fatalf("empty data patch wiped the credential: %+v", patched.Data)
	}
}

// TestCredentialTestPayloadEndpoint validates an unsaved payload —
// the UI's "Test connection" button must check the values currently in
// the form, not the ones still in storage.
func TestCredentialTestPayloadEndpoint(t *testing.T) {
	srv := newTestServer(t)

	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials/test", map[string]any{
		"type": "httpBasicAuth",
		"data": map[string]any{
			"user":     "admin",
			"password": "secret",
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("test payload: status %d body %s", w.Code, w.Body.String())
	}
	var result testCredentialResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "OK" {
		t.Fatalf("expected OK, got %+v", result)
	}

	// Missing required field -> Error.
	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials/test", map[string]any{
		"type": "httpBasicAuth",
		"data": map[string]any{"user": "admin"},
	})
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "Error" {
		t.Fatalf("expected Error for missing password, got %+v", result)
	}
}

// TestListCredentialTypes makes sure the UI's type chooser is driven
// by the backend — mongoDb and kafka were missing from the hardcoded
// frontend list, so those credentials could not be created at all.
func TestListCredentialTypes(t *testing.T) {
	srv := newTestServer(t)

	w := doJSON(t, srv, http.MethodGet, "/api/v1/credentials/types", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("types: status %d body %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []credentialTypeSummary `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, t := range resp.Data {
		seen[t.Type] = true
	}
	for _, want := range []string{"mongoDb", "kafka", "postgres", "mysql", "apiKey"} {
		if !seen[want] {
			t.Fatalf("credential type %q missing from types list: %+v", want, resp.Data)
		}
	}
}

func TestListCredentialsEnvelope(t *testing.T) {
	srv := newTestServer(t)

	createBody := map[string]any{
		"name": "cred-a",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k-1"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d", w.Code)
	}

	w = doJSON(t, srv, http.MethodGet, "/api/v1/credentials", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: status %d", w.Code)
	}

	var resp struct {
		Data       []CredentialResponse `json:"data"`
		NextCursor any                  `json:"nextCursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Name != "cred-a" {
		t.Fatalf("expected 1 credential named cred-a, got: %+v", resp.Data)
	}
}

// TestCredentialSchemaAliasesAndMissingTypes covers credential types
// that exist in the store but had no form definition — the legacy n8n
// spellings mySql / openAiApi plus rabbitmq and redis. Without them the
// edit modal renders no fields and the user sees an empty form.
func TestCredentialSchemaAliasesAndMissingTypes(t *testing.T) {
	srv := newTestServer(t)

	for _, typ := range []string{"mySql", "openAiApi", "rabbitmq", "redis"} {
		w := doJSON(t, srv, http.MethodGet, "/api/v1/credentials/schema/"+typ, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("schema %s: status %d body %s", typ, w.Code, w.Body.String())
		}
		var schema CredentialTypeSchema
		if err := json.Unmarshal(w.Body.Bytes(), &schema); err != nil {
			t.Fatalf("decode %s: %v", typ, err)
		}
		if len(schema.Properties) == 0 {
			t.Fatalf("schema %s has no properties", typ)
		}
	}
}

func TestCredentialSchema(t *testing.T) {
	srv := newTestServer(t)

	w := doJSON(t, srv, http.MethodGet, "/api/v1/credentials/schema/httpBasicAuth", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("schema: status %d, body %s", w.Code, w.Body.String())
	}
	var schema CredentialTypeSchema
	if err := json.Unmarshal(w.Body.Bytes(), &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if schema.Type != "httpBasicAuth" {
		t.Fatalf("schema type: got %s", schema.Type)
	}
	wantRequired := map[string]bool{"user": true, "password": true}
	for _, r := range schema.Required {
		delete(wantRequired, r)
	}
	if len(wantRequired) > 0 {
		t.Fatalf("missing required: %+v, full schema: %+v", wantRequired, schema)
	}

	// Unknown type -> 404
	w = doJSON(t, srv, http.MethodGet, "/api/v1/credentials/schema/thisTypeDoesNotExist", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown type: status %d, want 404", w.Code)
	}
}

func TestCredentialTestEndpoint(t *testing.T) {
	srv := newTestServer(t)

	// httpBasicAuth is validated statically — no outbound connection is
	// attempted for it, so the test stays hermetic.
	createBody := map[string]any{
		"name": "basic-valid",
		"type": "httpBasicAuth",
		"data": map[string]any{
			"user":     "admin",
			"password": "secret",
		},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d", w.Code)
	}
	var created CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials/"+created.ID+"/test", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("test: status %d", w.Code)
	}
	var result testCredentialResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "OK" {
		t.Fatalf("expected OK, got %+v", result)
	}

	createBody2 := map[string]any{
		"name": "basic-invalid",
		"type": "httpBasicAuth",
		"data": map[string]any{
			"password": "secret",
		},
	}
	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody2)
	if w.Code != http.StatusCreated {
		t.Fatalf("create2: status %d", w.Code)
	}
	var created2 CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created2)

	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials/"+created2.ID+"/test", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "Error" {
		t.Fatalf("expected Error for missing user, got %+v", result)
	}

	// A postgres credential with an out-of-range port is rejected by the
	// static port check — before any connection attempt.
	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials", map[string]any{
		"name": "pg-bad-port",
		"type": "postgres",
		"data": map[string]any{
			"host":     "db.example.com",
			"port":     70000,
			"database": "prod",
			"user":     "admin",
			"password": "secret",
		},
	})
	var created3 CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created3)
	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials/"+created3.ID+"/test", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "Error" || !strings.Contains(result.Message, "Port out of range") {
		t.Fatalf("expected port range error, got %+v", result)
	}
}

// ---------------------------------------------------------------------------
// Fake AMQP broker helpers
// ---------------------------------------------------------------------------

// amqpFrame builds one AMQP 0-9-1 frame.
func amqpFrame(frameType byte, payload []byte) []byte {
	frame := make([]byte, 7+len(payload)+1)
	frame[0] = frameType
	// channel 0
	size := len(payload)
	frame[3] = byte(size >> 24)
	frame[4] = byte(size >> 16)
	frame[5] = byte(size >> 8)
	frame[6] = byte(size)
	copy(frame[7:], payload)
	frame[len(frame)-1] = 0xCE
	return frame
}

// amqpLongStr encodes a long string (4 byte length prefix).
func amqpLongStr(s string) []byte {
	out := []byte{byte(len(s) >> 24), byte(len(s) >> 16), byte(len(s) >> 8), byte(len(s))}
	return append(out, s...)
}

// amqpShortStr encodes a short string (1 byte length prefix).
func amqpShortStr(s string) []byte {
	return append([]byte{byte(len(s))}, s...)
}

// readAmqpFrame reads one frame and returns its payload.
func readAmqpFrame(conn net.Conn) (byte, []byte, error) {
	header := make([]byte, 7)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, nil, err
	}
	size := int(header[3])<<24 | int(header[4])<<16 | int(header[5])<<8 | int(header[6])
	payload := make([]byte, size)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, nil, err
	}
	end := make([]byte, 1)
	if _, err := io.ReadFull(conn, end); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}

// amqpStartOKResponse extracts the SASL response from a
// connection.start-ok frame payload.
func amqpStartOKResponse(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	pos := 4 // class + method
	if len(payload) < pos+4 {
		return ""
	}
	tableLen := int(payload[pos])<<24 | int(payload[pos+1])<<16 | int(payload[pos+2])<<8 | int(payload[pos+3])
	pos += 4 + tableLen
	if len(payload) < pos+1 {
		return ""
	}
	mechLen := int(payload[pos])
	pos += 1 + mechLen
	if len(payload) < pos+4 {
		return ""
	}
	respLen := int(payload[pos])<<24 | int(payload[pos+1])<<16 | int(payload[pos+2])<<8 | int(payload[pos+3])
	pos += 4
	if len(payload) < pos+respLen {
		return ""
	}
	return string(payload[pos : pos+respLen])
}

// TestRabbitMQCredentialTestSendsCredentialsAndReportsAuthFailure proves
// the RabbitMQ test performs a real AMQP handshake: the credentials are
// sent to the broker and a refusal is reported as an authentication
// failure.
func TestRabbitMQCredentialTestSendsCredentialsAndReportsAuthFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	creds := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

		header := make([]byte, 8)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}

		// connection.start
		start := []byte{0, 10, 0, 10, 0, 9, 0, 0, 0, 0}
		start = append(start, amqpLongStr("PLAIN")...)
		start = append(start, amqpLongStr("en_US")...)
		if _, err := conn.Write(amqpFrame(1, start)); err != nil {
			return
		}

		// connection.start-ok -> capture the SASL response
		_, payload, err := readAmqpFrame(conn)
		if err != nil {
			return
		}
		creds <- amqpStartOKResponse(payload)

		// connection.close with ACCESS_REFUSED
		closed := []byte{0, 10, 0, 50, 0x01, 0x93}
		closed = append(closed, amqpShortStr("ACCESS_REFUSED - Login was refused using authentication mechanism PLAIN")...)
		closed = append(closed, 0, 10, 0, 11)
		_, _ = conn.Write(amqpFrame(1, closed))
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port := 0
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	res, attempted := testCredentialConnection("rabbitmq", map[string]any{
		"host":     host,
		"port":     port,
		"user":     "user",
		"password": "secret",
		"vhost":    "/",
	})
	if !attempted {
		t.Fatal("rabbitmq test must attempt a connection")
	}
	if res.Status != "Error" {
		t.Fatalf("expected Error, got %+v", res)
	}
	if !strings.Contains(strings.ToLower(res.Message), "authentication failed") {
		t.Fatalf("expected an authentication failure, got %+v", res)
	}
	if strings.Contains(res.Message, "secret") {
		t.Fatalf("password must not appear in the message: %+v", res)
	}

	select {
	case got := <-creds:
		if got != "\x00user\x00secret" {
			t.Fatalf("credentials were not sent to the broker, got %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("broker did not receive connection.start-ok")
	}
}

// TestCredentialValidateAcceptsAliasedFields proves a synced credential
// that stores `hostname` / `username` (instead of `host` / `user`) is not
// reported as missing its required fields.
func TestCredentialValidateAcceptsAliasedFields(t *testing.T) {
	res := validateCredentialData("rabbitmq", map[string]any{
		"hostname": "127.0.0.1",
		"port":     1,
		"username": "user",
		"password": "pass",
		"vhost":    "/",
	})
	if strings.Contains(res.Message, "Missing or empty required fields") {
		t.Fatalf("aliased fields must satisfy the required check, got %+v", res)
	}
	if !strings.Contains(res.Message, "RabbitMQ connection failed") {
		t.Fatalf("expected the connection attempt to run, got %+v", res)
	}
}

// TestCredentialTestRejectsWrongPassword proves the test endpoint now
// performs a REAL connection attempt: a wrong Redis password must be
// reported as an error, not as "valid". A fake RESP server stands in
// for Redis so the test stays hermetic.
func TestCredentialTestRejectsWrongPassword(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 512)
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				if _, err := c.Read(buf); err != nil {
					return
				}
				// Reply to AUTH with an error, like Redis does for a
				// wrong password.
				_, _ = c.Write([]byte("-WRONGPASS invalid username-password pair\r\n"))
			}(conn)
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port := 0
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	srv := newTestServer(t)
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials/test", map[string]any{
		"type": "redis",
		"data": map[string]any{
			"host":     host,
			"port":     port,
			"password": "wrong-password",
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("test payload: status %d body %s", w.Code, w.Body.String())
	}
	var result testCredentialResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "Error" {
		t.Fatalf("wrong password must not be reported as valid, got %+v", result)
	}
	if !strings.Contains(strings.ToLower(result.Message), "auth") {
		t.Fatalf("expected an authentication error, got %+v", result)
	}
}

func TestCredentialTransferEndpoint(t *testing.T) {
	srv := newTestServer(t)
	createBody := map[string]any{
		"name": "x",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	var c CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &c)

	w = doJSON(t, srv, http.MethodPut, "/api/v1/credentials/"+c.ID+"/transfer",
		map[string]any{"destinationProjectId": "p-2"})
	if w.Code != http.StatusOK {
		t.Fatalf("transfer: status %d body %s", w.Code, w.Body.String())
	}
	var resp CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID != c.ID {
		t.Fatalf("transfer returned different id: %s vs %s", resp.ID, c.ID)
	}
}

func TestCredentialPatchPartial(t *testing.T) {
	srv := newTestServer(t)
	createBody := map[string]any{
		"name": "old",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k1"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	var c CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &c)

	w = doJSON(t, srv, http.MethodPatch, "/api/v1/credentials/"+c.ID,
		map[string]any{"name": "renamed"})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: status %d body %s", w.Code, w.Body.String())
	}
	var updated CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Name != "renamed" {
		t.Fatalf("expected renamed, got %s", updated.Name)
	}
	if updated.Type != "apiKey" {
		t.Fatalf("type changed unexpectedly: %s", updated.Type)
	}
}

// TestCredentialWriteThroughToEngine verifies the fix for the
// user-reported bug: credentials created/updated/deleted via the
// UI must be visible to the workflow engine immediately, without
// requiring a server restart or a manual LoadFromStorage refresh.
// We stand up a tiny CredentialManager (the same one the engine
// reads), wire it through SetCredentialManager, and assert that
// UpsertCredential / RemoveCredential are invoked by the handlers.
func TestCredentialWriteThroughToEngine(t *testing.T) {
	// The credential store requires M9M_DEV_MODE=true (or a
	// real N8N_ENCRYPTION_KEY in the environment) to spin up
	// without prod-mode encryption-key gating. Tests run in dev
	// mode so we set it explicitly here and clean it up.
	t.Setenv("M9M_DEV_MODE", "true")

	srv := newTestServer(t)
	cm, err := credentials.NewCredentialManager()
	if err != nil {
		t.Fatalf("NewCredentialManager: %v", err)
	}
	srv.SetCredentialManager(cm)

	// Create — manager should now have the credential.
	createBody := map[string]any{
		"name": "write-thru",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k-create"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d body %s", w.Code, w.Body.String())
	}
	var created CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if _, err := cm.GetCredential(created.ID); err != nil {
		t.Fatalf("credential not in manager after Create: %v", err)
	}

	// Update — manager's data must reflect the new value.
	updateBody := map[string]any{
		"name": "write-thru",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k-updated"},
	}
	w = doJSON(t, srv, http.MethodPut, "/api/v1/credentials/"+created.ID, updateBody)
	if w.Code != http.StatusOK {
		t.Fatalf("update: status %d body %s", w.Code, w.Body.String())
	}
	c, err := cm.GetCredential(created.ID)
	if err != nil {
		t.Fatalf("GetCredential after update: %v", err)
	}
	if got := c.Data["apiKey"]; got != "k-updated" {
		t.Fatalf("expected apiKey=k-updated, got %v", got)
	}

	// Delete — manager must drop it.
	w = doJSON(t, srv, http.MethodDelete, "/api/v1/credentials/"+created.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d body %s", w.Code, w.Body.String())
	}
	if _, err := cm.GetCredential(created.ID); err == nil {
		t.Fatalf("credential still in manager after Delete")
	}
}
