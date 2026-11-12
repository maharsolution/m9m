// Package database — MongoDB node implementation.
//
// m9m's MongoDB node uses the official `go.mongodb.org/mongo-driver`
// (already pinned in go.mod). It accepts a standard `mongodb://` or
// `mongodb+srv://` connection string, then runs the configured
// operation against the named database + collection.
//
// Why this rewrite (see bug-fix.MD BUG-MONGO-2026-10-12-001):
//   - The previous implementation only spoke the Atlas Data API
//     (HTTP, `/action/find`, etc.) and required a `baseUrl` +
//     `dataSource` + `apiKey` envelope that no standard MongoDB
//     deployment exposes.
//   - Real MongoDB deployments — including the Atlas cluster the
//     user is trying to reach with
//     `mongodb+srv://tdssolutiontech_db_user:erwan123@cluster0.aldwftp.mongodb.net/`
//     — speak the binary wire protocol on port 27017, not HTTP.
//   - The `mongo-driver` was already in go.mod, just unused.
//
// Operations supported (mirror n8n's `n8n-nodes-base.mongoDb`):
//
//	find        — `coll.Find(filter, opts).All(&docs)`
//	findOne     — `coll.FindOne(filter).Decode(&doc)`
//	insert      — `coll.InsertOne(doc)`
//	insertMany  — `coll.InsertMany(docs)`
//	update      — `coll.UpdateMany(filter, update, opts)`
//	delete      — `coll.DeleteMany(filter)` — refuses empty filter
//	aggregate   — `coll.Aggregate(pipeline).All(&docs)`
//	count       — `coll.CountDocuments(filter)`
//
// BSON-isms normalised on the way out:
//   - `primitive.ObjectID` → hex string
//   - `primitive.DateTime` → `time.Time`
//   - nested `bson.M` / `bson.D` / `bson.A` → plain Go shapes
//
// Security defaults:
//   - TLS enabled by default for `mongodb+srv://` URIs (the driver
//     does this automatically; we don't override).
//   - `delete` with an empty filter is rejected (would wipe the
//     collection; n8n's UI doesn't even expose the path, we don't
//     either).
//   - Connection timeout: 30 s. Per-operation context: 30 s. Tunable
//     later via the `INodeTypeDescription.properties` if needed.
package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// MongoDBNode implements `n8n-nodes-base.mongoDb` over the official
// MongoDB Go driver. One struct instance is reused per
// `RegisterAllNodes` call (mirroring every other node in this
// package); the connection is opened lazily on each `Execute` and
// closed before the function returns. For workflows that hammer
// MongoDB, this means a short-lived TCP+TLS handshake per Execute
// — acceptable for low/medium QPS; for high QPS the right fix is
// to lift the client to a process-wide pool, which is a 0.5.0
// milestone item (see m9m.MD §12).
// Run context is NEVER stored on the singleton: Finnet's Kafka
// consumer (Find and replace) and the webhook poller (Find Respond)
// execute concurrently on the same registered node, and a shared
// runCtx field would clobber $('Async Subworkflow').item.json.key
// mid-poll — leaving the webhook with an empty body.
type MongoDBNode struct {
	*base.BaseNode
}

type mongoRunContext struct {
	workflow  *model.Workflow
	runData   *expressions.RunExecutionData
	runIndex  int
	itemIndex int
}

// NewMongoDBNode constructs a fresh MongoDB node.
func NewMongoDBNode() *MongoDBNode {
	return &MongoDBNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "MongoDB",
			Description: "Perform operations on MongoDB collections via the official driver (supports mongodb:// and mongodb+srv://).",
			Category:    "Database",
			Group:       []string{"database"},
			Properties:  mongoProperties(),
			Inputs:      []string{"main"},
			Outputs:     []string{"main"},
			Credentials: []base.NodeCredentialSpec{
				{Type: "mongoDb", DisplayName: "MongoDB", Required: true},
			},
		}),
	}
}

// mongoProperties returns the MongoDB node's property descriptors
// for the native-driver implementation. Mirrors n8n's
// `INodeTypeDescription.properties` for `n8n-nodes-base.mongoDb`:
// a `Connection String` field (the standard MongoDB URI), the
// database/collection selectors, the operation picker, and the
// operation-specific knobs (filter, document, pipeline, etc.).
//
// The connection string is intentionally a plain `stringProp` (not
// `password` flag) so users can paste `mongodb+srv://` URIs
// directly. The `password` flag on a stringProp masks the value in
// the UI but breaks the URI parser — the URI itself is the secret,
// not a single sub-field.
// mongoProperties describes the MongoDB node.
//
// database.MongoDBNode reads `connectionString` (or host / port),
// `database`, `collection`, `operation`, `filter`, `query`, `sort`,
// `limit`, `skip`, `document`, `documents`, `update`, `updateKey`,
// `fields`, `upsert` and `pipeline`. The operation-specific fields
// are only shown for the operations that consume them.
func mongoProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Find", Value: "find"},
		{Name: "Find one", Value: "findOne"},
		{Name: "Insert", Value: "insert"},
		{Name: "Insert many", Value: "insertMany"},
		{Name: "Update", Value: "update"},
		{Name: "Find one and replace", Value: "findOneAndReplace"},
		{Name: "Delete", Value: "delete"},
		{Name: "Aggregate", Value: "aggregate"},
		{Name: "Count", Value: "count"},
	}
	props := []base.NodeProperty{
		nodeprops.Str("Connection string", "connectionString", "",
			"mongodb:// or mongodb+srv:// URI used to reach the server.",
			"mongodb://localhost:27017", true),
		nodeprops.Str("Host", "host", "localhost",
			"Server host (used when no connection string is set).",
			"localhost", false),
		nodeprops.Num("Port", "port", 27017,
			"Server port (used when no connection string is set).", false),
		nodeprops.Str("Database", "database", "",
			"MongoDB database name.",
			"mydb", true),
		nodeprops.Str("Collection", "collection", "",
			"MongoDB collection name.",
			"users", true),
		nodeprops.Opt("Operation", "operation", "find",
			"MongoDB operation to run.",
			ops, true),
		nodeprops.JSON("Filter", "filter", "{}",
			"MongoDB filter expression (find / findOne / update / delete / count)."),
		nodeprops.JSON("Query", "query", "{}",
			"Alias of Filter — n8n's mongoDb parameter name."),
		nodeprops.JSON("Sort", "sort", "{}",
			"MongoDB sort specification (find).").
			WithDisplayOptions(nodeprops.Show("operation", "find")),
		nodeprops.Num("Limit", "limit", 0,
			"Maximum number of documents to return (0 = no limit, find).", false).
			WithDisplayOptions(nodeprops.Show("operation", "find")),
		nodeprops.Num("Skip", "skip", 0,
			"Number of documents to skip (find).", false).
			WithDisplayOptions(nodeprops.Show("operation", "find")),
		nodeprops.JSON("Document", "document", "{}",
			"Document to insert (insert). Falls back to the input item when empty.").
			WithDisplayOptions(nodeprops.Show("operation", "insert")),
		nodeprops.JSON("Documents", "documents", "[]",
			"Documents to insert (insertMany). Falls back to the input items.").
			WithDisplayOptions(nodeprops.Show("operation", "insertMany")),
		nodeprops.JSON("Update", "update", "{}",
			"MongoDB update specification, e.g. {\"$set\": {\"name\": \"x\"}} (update).").
			WithDisplayOptions(nodeprops.Show("operation", "update")),
		nodeprops.Str("Update key", "updateKey", "id",
			"Field used as the filter key (findOneAndReplace).",
			"id", false).
			WithDisplayOptions(nodeprops.Show("operation", "findOneAndReplace")),
		nodeprops.Str("Fields", "fields", "",
			"Comma-separated fields to keep in the replacement document (findOneAndReplace). Empty = all input fields.",
			"", false).
			WithDisplayOptions(nodeprops.Show("operation", "findOneAndReplace")),
		nodeprops.Bool("Upsert", "upsert", false,
			"Insert the document when no match is found (update / findOneAndReplace).").
			WithDisplayOptions(nodeprops.Show("operation", "update", "findOneAndReplace")),
		nodeprops.JSON("Pipeline", "pipeline", "[]",
			"Aggregation pipeline stages (aggregate).").
			WithDisplayOptions(nodeprops.Show("operation", "aggregate")),
	}
	return append(props, nodeprops.Settings()...)
}

// ExecuteWithRun lets $('OtherNode') resolve inside query/filter
// expressions (Finnet Find Respond uses
// `$('Async Subworkflow').item.json.key`).
//
// The run context is passed by value into Execute — never stored on
// the singleton — so concurrent Kafka + webhook pollers cannot
// clobber each other's `$('…')` lookups.
func (n *MongoDBNode) ExecuteWithRun(workflow *model.Workflow, runData *expressions.RunExecutionData, runIndex, itemIndex int, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return n.execute(inputData, nodeParams, &mongoRunContext{
		workflow:  workflow,
		runData:   runData,
		runIndex:  runIndex,
		itemIndex: itemIndex,
	})
}

// Execute runs the configured MongoDB operation.
func (n *MongoDBNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return n.execute(inputData, nodeParams, nil)
}

func (n *MongoDBNode) execute(inputData []model.DataItem, nodeParams map[string]interface{}, runCtx *mongoRunContext) ([]model.DataItem, error) {
	resolvedParams, err := n.resolveQueryExpressions(nodeParams, inputData, runCtx)
	if err != nil {
		return nil, err
	}
	connStr, database, collection := n.resolveConnParams(resolvedParams)
	if connStr == "" {
		return nil, n.CreateError("connectionString is required (e.g. mongodb://user:pass@host:27017 or mongodb+srv://user:pass@cluster0.x.mongodb.net/)", nil)
	}
	operation := n.GetStringParameter(resolvedParams, "operation", "find")
	nodeParams = resolvedParams

	if database == "" {
		return nil, n.CreateError("database is required", nil)
	}
	if collection == "" {
		return nil, n.CreateError("collection is required", nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Reuse a pooled client per (connection string + options). The
	// old code connected, pinged and disconnected on every single
	// execution, which cost ~0.5 s per poll against a remote Atlas
	// cluster — and a retry loop polls once per iteration.
	client, err := mongoPool.get(ctx, mongoClientKey(connStr, mongoServerSelectionTimeout()), mongoClientOptions(connStr))
	if err != nil {
		var poolErr *mongoPoolError
		if errors.As(err, &poolErr) && poolErr.stage == "ping" {
			return nil, n.CreateError(fmt.Sprintf("mongo ping (check connectionString, network ACL, credentials): %v", poolErr.err), nil)
		}
		return nil, n.CreateError(fmt.Sprintf("mongo connect: %v", err), nil)
	}

	coll := client.Database(database).Collection(collection)

	switch operation {
	case "find":
		return n.doFind(ctx, coll, nodeParams)
	case "findOne":
		return n.doFindOne(ctx, coll, nodeParams)
	case "insert":
		return n.doInsert(ctx, coll, nodeParams, inputData)
	case "insertMany":
		return n.doInsertMany(ctx, coll, nodeParams, inputData)
	case "update":
		return n.doUpdate(ctx, coll, nodeParams)
	case "findOneAndReplace":
		return n.doFindOneAndReplace(ctx, coll, nodeParams, inputData)
	case "delete":
		return n.doDelete(ctx, coll, nodeParams)
	case "aggregate":
		return n.doAggregate(ctx, coll, nodeParams)
	case "count":
		return n.doCount(ctx, coll, nodeParams)
	default:
		return nil, n.CreateError(fmt.Sprintf("unsupported operation: %s", operation), nil)
	}
}

// toBSON accepts either a JSON string or a map[string]interface{}
// from the workflow params and converts to bson.M. Empty / "{}"
// yields an empty filter (matches everything in find/count; n8n
// behaves the same way).
func toBSON(v interface{}) (bson.M, error) {
	if v == nil {
		return bson.M{}, nil
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" || s == "{}" || s == "null" {
			return bson.M{}, nil
		}
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return convertMapToBSON(out), nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		return convertMapToBSON(m), nil
	}
	return nil, fmt.Errorf("expected JSON object or string, got %T", v)
}

func convertMapToBSON(m map[string]interface{}) bson.M {
	out := make(bson.M, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// convertBSONToJSON flattens mongo-driver types to shapes that
// serialise cleanly via encoding/json. ObjectID → hex string.
// DateTime → time.Time. Nested bson.M / bson.D / bson.A → plain
// Go maps / slices. The result is safe to put inside a
// model.DataItem and to log or return over the API.
func convertBSONToJSON(m bson.M) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = normalizeBSONValue(v)
	}
	return out
}

func normalizeBSONValue(v interface{}) interface{} {
	switch t := v.(type) {
	case primitive.ObjectID:
		return t.Hex()
	case primitive.DateTime:
		return t.Time().UTC()
	case bson.M:
		return convertBSONToJSON(t)
	case bson.D:
		out := make(map[string]interface{}, len(t))
		for _, e := range t {
			out[e.Key] = normalizeBSONValue(e.Value)
		}
		return out
	case bson.A:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = normalizeBSONValue(e)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = normalizeBSONValue(e)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = normalizeBSONValue(val)
		}
		return out
	}
	return v
}

func (n *MongoDBNode) parseFilter(params map[string]interface{}) (bson.M, error) {
	// n8n's mongoDb node uses `query`; m9m historically used `filter`.
	raw := params["filter"]
	if raw == nil || raw == "" || raw == "{}" {
		if q := params["query"]; q != nil && q != "" {
			raw = q
		}
	}
	return toBSON(raw)
}

// resolveQueryExpressions evaluates n8n `={{...}}` / `{{...}}`
// expressions in `query` and `filter` so Find Respond can use
// `$('Async Subworkflow').item.json.key`.
func (n *MongoDBNode) resolveQueryExpressions(params map[string]interface{}, inputData []model.DataItem, runCtx *mongoRunContext) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		out[k] = v
	}
	evaluator := expressions.NewGojaExpressionEvaluator(expressions.DefaultEvaluatorConfig())
	ctx := &expressions.ExpressionContext{
		ConnectionInputData: inputData,
		ActiveNodeName:      "MongoDB",
		Mode:                expressions.ModeManual,
	}
	if runCtx != nil {
		ctx.Workflow = runCtx.workflow
		ctx.RunExecutionData = runCtx.runData
		ctx.RunIndex = runCtx.runIndex
		ctx.ItemIndex = runCtx.itemIndex
	}
	for _, key := range []string{"query", "filter"} {
		raw, ok := out[key].(string)
		if !ok || raw == "" {
			continue
		}
		if !expressions.IsExpression(raw) {
			continue
		}
		toEval := raw
		if strings.HasPrefix(raw, "=") {
			toEval = strings.TrimPrefix(raw, "=")
		}
		resolved, err := evaluator.EvaluateExpression(toEval, ctx)
		if err != nil {
			return nil, n.CreateError(fmt.Sprintf("failed to evaluate %s expression: %v", key, err), nil)
		}
		// A nil/empty resolution of a real query expression must not
		// fall through to toBSON(nil) → bson.M{} (match-all). That
		// made Finnet Find Respond intermittently return a stale
		// Atlas doc when $('Async Subworkflow').item.json.key failed
		// open, producing a lucky balance instead of an empty body.
		if resolved == nil {
			return nil, n.CreateError(fmt.Sprintf("%s expression resolved to nil", key), nil)
		}
		if s, ok := resolved.(string); ok {
			if strings.TrimSpace(s) == "" {
				return nil, n.CreateError(fmt.Sprintf("%s expression resolved to empty string", key), nil)
			}
			// When $('Async Subworkflow').item.json.key is missing,
			// the template becomes {"key":"undefined"} and Find
			// quietly misses forever. Recover the live key from
			// runExecutionData before that happens.
			if strings.Contains(s, `"key": "undefined"`) || strings.Contains(s, `"key":"undefined"`) ||
				strings.Contains(s, `"key": ""`) || strings.Contains(s, `"key":""`) {
				if recovered := lookupRunDataKey(runCtx); recovered != "" {
					s = strings.ReplaceAll(s, `"key": "undefined"`, fmt.Sprintf(`"key": %q`, recovered))
					s = strings.ReplaceAll(s, `"key":"undefined"`, fmt.Sprintf(`"key":%q`, recovered))
					s = strings.ReplaceAll(s, `"key": ""`, fmt.Sprintf(`"key": %q`, recovered))
					s = strings.ReplaceAll(s, `"key":""`, fmt.Sprintf(`"key":%q`, recovered))
				}
			}
			resolved = s
		}
		out[key] = resolved
	}
	return out, nil
}

// hasNonEmptyQueryParam reports whether a raw query/filter param was
// configured (string non-empty, or non-nil object). Used to distinguish
// intentional match-all from a evaporated expression.
func hasNonEmptyQueryParam(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		s := strings.TrimSpace(t)
		return s != "" && s != "{}" && s != "null"
	case map[string]interface{}:
		return len(t) > 0
	default:
		return true
	}
}

// lookupRunDataKey finds a `key` field on a prior node output in the
// live run. Finnet's Find Respond depends on
// $('Async Subworkflow').item.json.key; when that expression yields
// JS undefined we still have the key on Async Subworkflow / Payload /
// Input in runExecutionData.
func lookupRunDataKey(runCtx *mongoRunContext) string {
	if runCtx == nil || runCtx.runData == nil {
		return ""
	}
	if runCtx.runData.ExecutionData != nil && runCtx.runData.ExecutionData.ContextData != nil {
		if k, ok := runCtx.runData.ExecutionData.ContextData["subworkflowKey"].(string); ok && k != "" && k != "undefined" {
			return k
		}
	}
	if runCtx.runData.ResultData == nil {
		return ""
	}
	prefer := []string{"Async Subworkflow", "Payload", "Input"}
	for _, name := range prefer {
		slots := runCtx.runData.ResultData.NodeData[name]
		for i := len(slots) - 1; i >= 0; i-- {
			for _, item := range slots[i].Data {
				if k, ok := item.JSON["key"].(string); ok && k != "" && k != "undefined" {
					return k
				}
			}
		}
	}
	for _, slots := range runCtx.runData.ResultData.NodeData {
		for i := len(slots) - 1; i >= 0; i-- {
			for _, item := range slots[i].Data {
				if k, ok := item.JSON["key"].(string); ok && k != "" && k != "undefined" {
					return k
				}
			}
		}
	}
	return ""
}

// resolveConnParams reads connectionString / database / collection from
// either bare node parameters or the engine's credential-injected
// `mongoDb_<field>` keys (InjectCredentialsIntoNodeParameters).
func (n *MongoDBNode) resolveConnParams(params map[string]interface{}) (connStr, database, collection string) {
	connStr = strings.TrimSpace(n.GetStringParameter(params, "connectionString", ""))
	if connStr == "" {
		connStr = strings.TrimSpace(n.GetStringParameter(params, "mongoDb_connectionString", ""))
	}
	if connStr == "" {
		// n8n values-form credentials: host + port + user + password.
		host := strings.TrimSpace(n.GetStringParameter(params, "mongoDb_host", ""))
		if host == "" {
			host = strings.TrimSpace(n.GetStringParameter(params, "host", ""))
		}
		if host != "" {
			port := strings.TrimSpace(n.GetStringParameter(params, "mongoDb_port", ""))
			if port == "" {
				port = strings.TrimSpace(n.GetStringParameter(params, "port", "27017"))
			}
			if port == "" {
				port = "27017"
			}
			user := n.GetStringParameter(params, "mongoDb_user", "")
			if user == "" {
				user = n.GetStringParameter(params, "user", "")
			}
			pass := n.GetStringParameter(params, "mongoDb_password", "")
			if pass == "" {
				pass = n.GetStringParameter(params, "password", "")
			}
			if user != "" {
				connStr = fmt.Sprintf("mongodb://%s:%s@%s:%s", user, pass, host, port)
			} else {
				connStr = fmt.Sprintf("mongodb://%s:%s", host, port)
			}
		}
	}
	database = n.GetStringParameter(params, "database", "")
	if database == "" {
		database = n.GetStringParameter(params, "mongoDb_database", "")
	}
	collection = n.GetStringParameter(params, "collection", "")
	return connStr, database, collection
}

func (n *MongoDBNode) doFind(ctx context.Context, coll *mongo.Collection, params map[string]interface{}) ([]model.DataItem, error) {
	filter, err := n.parseFilter(params)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("filter: %v", err), nil)
	}
	// Reject match-all when the node was configured with a query /
	// filter that evaporated during expression evaluation. An empty
	// filter is only valid when the workflow author left query empty.
	if len(filter) == 0 {
		rawQuery := params["query"]
		rawFilter := params["filter"]
		if hasNonEmptyQueryParam(rawQuery) || hasNonEmptyQueryParam(rawFilter) {
			return nil, n.CreateError("mongo find: query/filter resolved to empty match-all filter", nil)
		}
	}
	opts := options.Find()
	if raw, ok := params["sort"]; ok {
		s, sErr := toBSON(raw)
		if sErr != nil {
			return nil, n.CreateError(fmt.Sprintf("sort: %v", sErr), nil)
		}
		if len(s) > 0 {
			opts.SetSort(s)
		}
	}
	if limit := n.GetIntParameter(params, "limit", 0); limit > 0 {
		opts.SetLimit(int64(limit))
	}
	if skip := n.GetIntParameter(params, "skip", 0); skip > 0 {
		opts.SetSkip(int64(skip))
	}

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo find: %v", err), nil)
	}
	defer cur.Close(ctx)

	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo cursor.All: %v", err), nil)
	}

	// Match n8n's behaviour: emit one DataItem per matching doc,
	// with the document fields flattened to the top level (not
	// nested under a `documents` key — that's the Atlas Data API
	// shape, which the old broken implementation used).
	out := make([]model.DataItem, 0, len(docs))
	for _, d := range docs {
		out = append(out, model.DataItem{JSON: convertBSONToJSON(d)})
	}
	return out, nil
}

func (n *MongoDBNode) doFindOne(ctx context.Context, coll *mongo.Collection, params map[string]interface{}) ([]model.DataItem, error) {
	filter, err := n.parseFilter(params)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("filter: %v", err), nil)
	}
	var doc bson.M
	err = coll.FindOne(ctx, filter).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		// Match n8n: surface "no match" as a single item with
		// `found: false` rather than an error, so downstream
		// nodes can branch on it.
		return []model.DataItem{{JSON: map[string]interface{}{"found": false}}}, nil
	}
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo findOne: %v", err), nil)
	}
	return []model.DataItem{{JSON: convertBSONToJSON(doc)}}, nil
}

func (n *MongoDBNode) doInsert(ctx context.Context, coll *mongo.Collection, params map[string]interface{}, inputData []model.DataItem) ([]model.DataItem, error) {
	doc, err := n.firstDocument(params["document"], inputData)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	res, err := coll.InsertOne(ctx, doc)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo insert: %v", err), nil)
	}
	out := map[string]interface{}{"insertedCount": 1}
	if res != nil && res.InsertedID != nil {
		out["insertedId"] = objectIDToString(res.InsertedID)
	}
	return []model.DataItem{{JSON: out}}, nil
}

func (n *MongoDBNode) doInsertMany(ctx context.Context, coll *mongo.Collection, params map[string]interface{}, inputData []model.DataItem) ([]model.DataItem, error) {
	docs, err := n.manyDocuments(params["documents"], inputData)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}
	if len(docs) == 0 {
		return nil, n.CreateError("documents or input data is required", nil)
	}
	res, err := coll.InsertMany(ctx, docs)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo insertMany: %v", err), nil)
	}
	ids := make([]string, 0, len(res.InsertedIDs))
	for _, id := range res.InsertedIDs {
		ids = append(ids, objectIDToString(id))
	}
	return []model.DataItem{{JSON: map[string]interface{}{
		"insertedCount": len(res.InsertedIDs),
		"insertedIds":   ids,
	}}}, nil
}

// doFindOneAndReplace mirrors n8n findOneAndReplace: filter by updateKey
// from the input item, replace with selected fields (or the whole item),
// optionally upsert. Finnet Kafka consumer depends on this.
func (n *MongoDBNode) doFindOneAndReplace(ctx context.Context, coll *mongo.Collection, params map[string]interface{}, inputData []model.DataItem) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}
	updateKey := n.GetStringParameter(params, "updateKey", "id")
	if updateKey == "" {
		updateKey = "id"
	}
	item := inputData[0].JSON
	if item == nil {
		return nil, n.CreateError("findOneAndReplace requires an input item", nil)
	}
	keyVal, ok := item[updateKey]
	if !ok || keyVal == nil || keyVal == "" {
		return nil, n.CreateError(fmt.Sprintf("findOneAndReplace: updateKey %q missing or empty on input", updateKey), nil)
	}
	filter := bson.M{updateKey: keyVal}
	fieldsStr := strings.TrimSpace(n.GetStringParameter(params, "fields", ""))
	replacement := bson.M{}
	if fieldsStr == "" {
		replacement = convertMapToBSON(item)
	} else {
		for _, f := range strings.Split(fieldsStr, ",") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if v, exists := item[f]; exists {
				replacement[f] = v
			}
		}
	}
	if len(replacement) == 0 {
		return nil, n.CreateError("findOneAndReplace: replacement document is empty", nil)
	}
	opts := options.FindOneAndReplace().SetReturnDocument(options.After)
	if n.GetBoolParameter(params, "upsert", false) {
		opts.SetUpsert(true)
	}
	var doc bson.M
	err := coll.FindOneAndReplace(ctx, filter, replacement, opts).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return []model.DataItem{{JSON: map[string]interface{}{"found": false}}}, nil
	}
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo findOneAndReplace: %v", err), nil)
	}
	return []model.DataItem{{JSON: convertBSONToJSON(doc)}}, nil
}

func (n *MongoDBNode) doUpdate(ctx context.Context, coll *mongo.Collection, params map[string]interface{}) ([]model.DataItem, error) {
	filter, err := n.parseFilter(params)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("filter: %v", err), nil)
	}
	update, err := toBSON(params["update"])
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("update: %v", err), nil)
	}
	if len(update) == 0 {
		return nil, n.CreateError("update spec is required (e.g. {\"$set\": {\"name\": \"x\"}})", nil)
	}
	opts := options.Update()
	if n.GetBoolParameter(params, "upsert", false) {
		opts.SetUpsert(true)
	}
	res, err := coll.UpdateMany(ctx, filter, update, opts)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo update: %v", err), nil)
	}
	out := map[string]interface{}{
		"matchedCount":  res.MatchedCount,
		"modifiedCount": res.ModifiedCount,
	}
	if res.UpsertedCount > 0 {
		out["upsertedCount"] = res.UpsertedCount
		if res.UpsertedID != nil {
			out["upsertedId"] = objectIDToString(res.UpsertedID)
		}
	}
	return []model.DataItem{{JSON: out}}, nil
}

func (n *MongoDBNode) doDelete(ctx context.Context, coll *mongo.Collection, params map[string]interface{}) ([]model.DataItem, error) {
	filter, err := n.parseFilter(params)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("filter: %v", err), nil)
	}
	if len(filter) == 0 {
		// Refuse to delete with an empty filter — that would
		// wipe the entire collection. n8n's UI doesn't expose
		// this path either; the MongoDB shell requires it
		// explicitly (`db.coll.deleteMany({})`) to make the
		// danger visible.
		return nil, n.CreateError("filter is required for delete (refusing to delete with empty filter — would wipe the collection)", nil)
	}
	res, err := coll.DeleteMany(ctx, filter)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo delete: %v", err), nil)
	}
	return []model.DataItem{{JSON: map[string]interface{}{
		"deletedCount": res.DeletedCount,
	}}}, nil
}

func (n *MongoDBNode) doAggregate(ctx context.Context, coll *mongo.Collection, params map[string]interface{}) ([]model.DataItem, error) {
	var pipeline []interface{}
	if raw, ok := params["pipeline"]; ok && raw != nil {
		var perr error
		pipeline, perr = parsePipeline(raw)
		if perr != nil {
			return nil, n.CreateError(fmt.Sprintf("pipeline: %v", perr), nil)
		}
	}
	if len(pipeline) == 0 {
		return nil, n.CreateError("pipeline is required (e.g. [{\"$match\": {\"active\": true}}, {\"$count\": \"n\"}])", nil)
	}
	cur, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo aggregate: %v", err), nil)
	}
	defer cur.Close(ctx)
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo aggregate cursor: %v", err), nil)
	}
	out := make([]model.DataItem, 0, len(docs))
	for _, d := range docs {
		out = append(out, model.DataItem{JSON: convertBSONToJSON(d)})
	}
	return out, nil
}

func (n *MongoDBNode) doCount(ctx context.Context, coll *mongo.Collection, params map[string]interface{}) ([]model.DataItem, error) {
	filter, err := n.parseFilter(params)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("filter: %v", err), nil)
	}
	n64, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo count: %v", err), nil)
	}
	return []model.DataItem{{JSON: map[string]interface{}{"count": n64}}}, nil
}

// firstDocument extracts the single document for an `insert` from
// the `document` parameter (which may be a JSON string, a
// map[string]interface{}, or an empty/null default) with a
// fallback to the first input item.
func (n *MongoDBNode) firstDocument(raw interface{}, inputData []model.DataItem) (bson.M, error) {
	if raw != nil {
		switch t := raw.(type) {
		case string:
			s := strings.TrimSpace(t)
			if s != "" && s != "{}" {
				var m map[string]interface{}
				if err := json.Unmarshal([]byte(s), &m); err != nil {
					return nil, fmt.Errorf("document: %w", err)
				}
				return convertMapToBSON(m), nil
			}
		case map[string]interface{}:
			return convertMapToBSON(t), nil
		}
	}
	if len(inputData) > 0 {
		return convertMapToBSON(inputData[0].JSON), nil
	}
	return nil, fmt.Errorf("document or input data is required")
}

// manyDocuments is the insertMany equivalent. Accepts a JSON
// string, a []interface{}, or falls back to the input items.
func (n *MongoDBNode) manyDocuments(raw interface{}, inputData []model.DataItem) ([]interface{}, error) {
	if raw != nil {
		switch t := raw.(type) {
		case string:
			s := strings.TrimSpace(t)
			if s != "" && s != "[]" {
				var arr []interface{}
				if err := json.Unmarshal([]byte(s), &arr); err != nil {
					return nil, fmt.Errorf("documents: %w", err)
				}
				return arr, nil
			}
		case []interface{}:
			return t, nil
		}
	}
	if len(inputData) > 0 {
		out := make([]interface{}, 0, len(inputData))
		for _, item := range inputData {
			out = append(out, item.JSON)
		}
		return out, nil
	}
	return nil, fmt.Errorf("documents or input data is required")
}

func parsePipeline(raw interface{}) ([]interface{}, error) {
	switch t := raw.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" || s == "[]" {
			return nil, nil
		}
		var arr []interface{}
		if err := json.Unmarshal([]byte(s), &arr); err != nil {
			return nil, err
		}
		return arr, nil
	case []interface{}:
		return t, nil
	}
	return nil, fmt.Errorf("pipeline must be a JSON array or string")
}

// objectIDToString renders any inserted ID as a string. ObjectIDs
// become their hex form; everything else stringifies.
func objectIDToString(id interface{}) string {
	if oid, ok := id.(primitive.ObjectID); ok {
		return oid.Hex()
	}
	return fmt.Sprintf("%v", id)
}

// ValidateParameters validates MongoDB node parameters. Mirrors
// the new parameter surface (connectionString + database +
// collection + operation) and refuses unknown operations.
func (n *MongoDBNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return n.CreateError("parameters cannot be nil", nil)
	}
	connStr, database, collection := n.resolveConnParams(params)
	if connStr == "" {
		return n.CreateError("connectionString is required", nil)
	}
	operation := n.GetStringParameter(params, "operation", "find")
	validOps := map[string]bool{
		"find": true, "findOne": true, "insert": true, "insertMany": true,
		"update": true, "findOneAndReplace": true, "delete": true,
		"aggregate": true, "count": true,
	}
	if !validOps[operation] {
		return n.CreateError(fmt.Sprintf("invalid operation: %s", operation), nil)
	}
	if database == "" {
		return n.CreateError("database is required", nil)
	}
	if collection == "" {
		return n.CreateError("collection is required", nil)
	}
	return nil
}
