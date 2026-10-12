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
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
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
type MongoDBNode struct {
	*base.BaseNode
}

// NewMongoDBNode constructs a fresh MongoDB node.
func NewMongoDBNode() *MongoDBNode {
	return &MongoDBNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "MongoDB",
			Description: "Perform operations on MongoDB collections via the official driver (supports mongodb:// and mongodb+srv://).",
			Category:    "Database",
			Properties:  mongoProperties(),
			Inputs:      []string{"main"},
			Outputs:     []string{"main"},
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
func mongoProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Find", Value: "find"},
		{Name: "Find One", Value: "findOne"},
		{Name: "Insert", Value: "insert"},
		{Name: "Insert Many", Value: "insertMany"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
		{Name: "Aggregate", Value: "aggregate"},
		{Name: "Count", Value: "count"},
	}
	props := []base.NodeProperty{
		base.StringProp(
			"Connection String",
			"connectionString",
			"",
			"mongodb:// or mongodb+srv:// URI (e.g. mongodb+srv://user:pass@cluster0.x.mongodb.net/?appName=Cluster0).",
			"mongodb://localhost:27017",
			true,
		),
		base.StringProp("Database", "database", "", "MongoDB database name.", "mydb", true),
		base.StringProp("Collection", "collection", "", "MongoDB collection name.", "users", true),
		base.StringOpt("Operation", "operation", "find", "MongoDB operation to run.", ops, true),
		base.JsonProp("Filter", "filter", "{}", "MongoDB filter expression (find / findOne / update / delete / count)."),
		base.JsonProp("Sort", "sort", "{}", "MongoDB sort spec (find)."),
		base.NumberProp("Limit", "limit", 0, "Max documents to return (find). 0 = no limit.", false),
		base.NumberProp("Skip", "skip", 0, "Documents to skip (find).", false),
		base.JsonProp("Document", "document", "{}", "Single document (insert). Falls back to input data when empty."),
		base.JsonProp("Documents", "documents", "[]", "Array of documents (insertMany). Falls back to input items."),
		base.JsonProp("Update", "update", "{}", "MongoDB update spec (update). e.g. {\"$set\": {\"name\": \"x\"}}."),
		base.BoolProp("Upsert", "upsert", false, "Insert if no match (update).").WithDisplayOptions(base.NodeProperty_DisplayOptions_Show("operation", []string{"update"})),
		base.JsonProp("Pipeline", "pipeline", "[]", "Aggregation pipeline (aggregate)."),
	}
	return append(props, base.CommonSettings()...)
}

// Execute runs the configured MongoDB operation.
func (n *MongoDBNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	connStr := strings.TrimSpace(n.GetStringParameter(nodeParams, "connectionString", ""))
	if connStr == "" {
		return nil, n.CreateError("connectionString is required (e.g. mongodb://user:pass@host:27017 or mongodb+srv://user:pass@cluster0.x.mongodb.net/)", nil)
	}
	database := n.GetStringParameter(nodeParams, "database", "")
	collection := n.GetStringParameter(nodeParams, "collection", "")
	operation := n.GetStringParameter(nodeParams, "operation", "find")

	if database == "" {
		return nil, n.CreateError("database is required", nil)
	}
	if collection == "" {
		return nil, n.CreateError("collection is required", nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo connect: %v", err), nil)
	}
	defer func() {
		// Use a fresh context for Disconnect — the per-op context
		// may already be cancelled by the time we get here.
		dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dcancel()
		_ = client.Disconnect(dctx)
	}()

	// Ping to verify the URI resolved AND the cluster is reachable.
	// Connect() above only initialises the client; Ping is what
	// actually round-trips to the server.
	if err := client.Ping(ctx, nil); err != nil {
		return nil, n.CreateError(fmt.Sprintf("mongo ping (check connectionString, network ACL, credentials): %v", err), nil)
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
	return toBSON(params["filter"])
}

func (n *MongoDBNode) doFind(ctx context.Context, coll *mongo.Collection, params map[string]interface{}) ([]model.DataItem, error) {
	filter, err := n.parseFilter(params)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("filter: %v", err), nil)
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
	if strings.TrimSpace(n.GetStringParameter(params, "connectionString", "")) == "" {
		return n.CreateError("connectionString is required", nil)
	}
	operation := n.GetStringParameter(params, "operation", "find")
	validOps := map[string]bool{
		"find": true, "findOne": true, "insert": true, "insertMany": true,
		"update": true, "delete": true, "aggregate": true, "count": true,
	}
	if !validOps[operation] {
		return n.CreateError(fmt.Sprintf("invalid operation: %s", operation), nil)
	}
	if n.GetStringParameter(params, "database", "") == "" {
		return n.CreateError("database is required", nil)
	}
	if n.GetStringParameter(params, "collection", "") == "" {
		return n.CreateError("collection is required", nil)
	}
	return nil
}
