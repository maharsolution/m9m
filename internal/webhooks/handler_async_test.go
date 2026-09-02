package webhooks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHandlerRouter wires up a real gorilla mux the same way the server
// does in production — using the registered router is required because
// `HandleProductionWebhook` extracts the URL path through `mux.Vars`,
// which is only populated when the request flows through the mux. Hand-
// rolling an httptest.NewRequest without the router leaves the path as
// "/" and the lookup misses.
func newHandlerRouter(h *Handler) *mux.Router {
	r := mux.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// TestHandleWebhook_AsyncDefaultResponseMode pins the n8n fire-and-forget
// contract: when a webhook's ResponseMode is the default (empty
// string or `onReceived`), POSTing to the production webhook route must
// return 200 OK with body `{"message":"Workflow was started"}`
// immediately. This is the shape n8n returns from
// `http://<n8n>/webhook/...` and the behaviour external callers
// (Zapier, CI hooks, custom HTTP integrations) rely on.
func TestHandleWebhook_AsyncDefaultResponseMode(t *testing.T) {
	mgr, _, ws := newTestManager()

	// A trivial workflow so the engine has something to look up; an
	// empty workflow means the engine just passes input through, which
	// keeps the test deterministic.
	wf := &model.Workflow{
		ID:     "wf-async-default",
		Name:   "async-default",
		Active: true,
		Nodes:  []model.Node{},
	}
	require.NoError(t, ws.SaveWorkflow(wf))

	// responseMode intentionally left at the Go zero value to confirm
	// that the *legacy default* (no explicit mode) still acks
	// asynchronously — restoring compatibility with webhook records
	// that were created before we added responseMode.
	wh := &Webhook{
		ID:         "wh-async-default",
		WorkflowID: wf.ID,
		Path:       "/user-signup",
		Method:     "POST",
		Active:     true,
		// ResponseMode: "",  // implicit legacy default
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	body, _ := json.Marshal(map[string]interface{}{"email": "u@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/webhook/user-signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code,
		"default responseMode must return 200 OK, not block on engine errors")
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"),
		"async ack must use JSON content-type")

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp),
		"async ack body must be valid JSON")
	assert.Equal(t, "Workflow was started", resp["message"],
		"async ack body must match the n8n wire contract exactly")
}

// TestHandleWebhook_AsyncViaOnReceivedResponseMode explicitly sets
// ResponseMode="onReceived" (the name n8n's API returns) and asserts the
// same async ack contract holds.
func TestHandleWebhook_AsyncViaOnReceivedResponseMode(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := &model.Workflow{
		ID:     "wf-async-onreceived",
		Name:   "async-onreceived",
		Active: true,
		Nodes:  []model.Node{},
	}
	require.NoError(t, ws.SaveWorkflow(wf))

	wh := &Webhook{
		ID:           "wh-async-onreceived",
		WorkflowID:   wf.ID,
		Path:         "/on-received",
		Method:       "POST",
		Active:       true,
		ResponseMode: "onReceived",
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhook/on-received", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Workflow was started")
}

// TestHandleWebhook_Async_HandlerReturnsBeforeEngine runs through a slow
// engine to prove the HTTP handler returns BEFORE workflow execution
// completes. We swap the WebhookManager's engine with a custom one that
// blocks until we close a channel — if the handler waits on the engine
// the test will time out, if it doesn't the test will pass.
func TestHandleWebhook_Async_HandlerReturnsBeforeEngine(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := &model.Workflow{
		ID:     "wf-async-timing",
		Name:   "async-timing",
		Active: true,
		Nodes:  []model.Node{},
	}
	require.NoError(t, ws.SaveWorkflow(wf))

	wh := &Webhook{
		ID:           "wh-async-timing",
		WorkflowID:   wf.ID,
		Path:         "/slow",
		Method:       "POST",
		Active:       true,
		ResponseMode: "onReceived",
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	released := make(chan struct{})
	var execStarted atomic.Bool
	finishing := make(chan struct{})

	// Embed the existing engine so we only need to override
	// ExecuteWorkflow. Using its concrete type means the rest of the
	// WorkflowEngine interface is implemented by delegation.
	slow := &slowEngine{
		WorkflowEngine: engine.NewWorkflowEngine(),
		released:       released,
		started:        &execStarted,
		finished:       finishing,
	}
	mgr.engine = slow

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhook/slow", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()

	// Wait until the goroutine inside the handler reaches the engine.
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("engine was never invoked — async path did not dispatch to engine")
	}

	// The HTTP handler MUST return now even though the engine is still
	// running. We give it 2s to write the response and finish.
	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("handler blocked on engine execution — async contract broken")
	}

	assert.Equal(t, http.StatusOK, rec.Code, "async handler must have written 200 OK")
	assert.Contains(t, rec.Body.String(), "Workflow was started",
		"async handler must have written the n8n ack body")

	// Tear the slow engine down.
	execStarted.Store(true)
	close(finishing)
}

// TestHandleWebhook_BlockingResponseMode_StillWaitsForEngine asserts
// that `responseMode: lastNode` retains the pre-existing *blocking*
// semantics: the handler waits for the engine, then writes the
// serialised engine output. This protects callers that explicitly opt
// in to reading the workflow output inline.
func TestHandleWebhook_BlockingResponseMode_StillWaitsForEngine(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := &model.Workflow{
		ID:     "wf-blocking",
		Name:   "blocking",
		Active: true,
		Nodes:  []model.Node{},
	}
	require.NoError(t, ws.SaveWorkflow(wf))

	wh := &Webhook{
		ID:           "wh-blocking",
		WorkflowID:   wf.ID,
		Path:         "/blocking",
		Method:       "POST",
		Active:       true,
		ResponseMode: "lastNode",
		// firstEntryJson is the default, but pin it explicitly so the
		// shape assertion below is unambiguous.
		ResponseData: "firstEntryJson",
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhook/blocking", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()

	// Most important assertion: the blocking path does NOT return the
	// async ack. If we ever regress to fire-and-forget semantics for
	// every webhook, this assertion fires.
	require.NotEqual(t, `{"message":"Workflow was started"}`, body,
		"blocking responseMode must not return the async ack")

	// An empty-workflow engine passes its input through, so with
	// firstEntryJson the body is the first item's JSON emitted as a
	// bare object — mirroring n8n's wire shape for the default
	// responseData. The shape check pins the bare-object (NOT array)
	// representation.
	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &obj),
		"blocking path must serialise the workflow output as a bare JSON object (n8n firstEntryJson contract)")
	require.NotEmpty(t, obj, "empty workflow with one input item yields one output item with the upstream fields")
}

// TestIsAsyncResponseMode locks down the rules engine that decides which
// ResponseMode values get the async ack. Any future webhook vendor that
// adds a new mode is forced to update this matrix deliberately.
func TestIsAsyncResponseMode(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		{"", true},                  // legacy default
		{"onReceived", true},        // named async
		{"lastNode", false},         // blocking — read last node output
		{"responseNode", false},     // blocking — read Response Node
		{"someUnknownMode", true},   // unknown defaults to async (safe)
	}
	for _, tc := range cases {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			assert.Equal(t, tc.want, IsAsyncResponseMode(tc.mode))
		})
	}
}

// ---------------------------------------------------------------------------
// Custom slow engine — used only by the timing-sensitive test above.
// ---------------------------------------------------------------------------

// slowEngine embeds the real engine.WorkflowEngine and overrides only
// ExecuteWorkflow so the slow path can block; all other interface
// methods stay valid by delegating to the embedded engine.
type slowEngine struct {
	engine.WorkflowEngine
	released chan struct{}
	started  *atomic.Bool
	finished chan struct{}
}

func (s *slowEngine) ExecuteWorkflow(wf *model.Workflow, input []model.DataItem) (*engine.ExecutionResult, error) {
	// Signal we've been called, then park on finished so the test can
	// observe "engine is running, handler hasn't returned yet".
	close(s.released)
	<-s.finished
	s.started.Store(true)

	// When released, behave like an empty-workflow engine (input → input).
	return &engine.ExecutionResult{Data: input}, nil
}
