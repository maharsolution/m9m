/*
Package database — tests for the MongoDB client pool.

Every MongoDB node execution used to open a client, ping the
cluster and disconnect again — ~0.5 s per poll against a remote
cluster, paid on every iteration of the `POST /webhook/post_balance`
retry loop. The pool keeps one client per (connection string +
options fingerprint) alive for the lifetime of the process.

These tests use the pool's injectable connect / ping / disconnect
hooks, so no MongoDB server is needed: the clients handed out are
created with `mongo.NewClient`, which builds a client (and its
topology) without any network I/O.
*/
package database

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// newTestClient returns a client that was never connected — it has
// no server behind it, but it is a distinct *mongo.Client instance,
// which is all the pool needs.
func newTestClient(t *testing.T, uri string) *mongo.Client {
	t.Helper()
	client, err := mongo.NewClient(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("NewClient(%q): %v", uri, err)
	}
	return client
}

// testPool returns a pool whose connect / ping / disconnect hooks are
// recorded instead of talking to a server.
func testPool(t *testing.T) (*mongoClientPool, *int, *[]string) {
	t.Helper()
	pool := newMongoClientPool()
	connects := 0
	var disconnected []string
	pool.connectFn = func(ctx context.Context, opts *options.ClientOptions) (*mongo.Client, error) {
		connects++
		// The client is never connected to a server — it only has
		// to be a distinct instance per connect call so the test
		// can tell a reused client from a fresh one.
		return newTestClient(t, "mongodb://127.0.0.1:27017"), nil
	}
	pool.pingFn = func(ctx context.Context, client *mongo.Client) error { return nil }
	pool.disconnectFn = func(ctx context.Context, client *mongo.Client) error {
		disconnected = append(disconnected, "client")
		return nil
	}
	return pool, &connects, &disconnected
}

// TestMongoClientPool_SameKeyReusesClient proves a second execution
// with the same connection string and options returns the SAME client
// and does not connect (or ping) a second time — the whole point of
// the pool.
func TestMongoClientPool_SameKeyReusesClient(t *testing.T) {
	pool, connects, _ := testPool(t)
	ctx := context.Background()
	uri := "mongodb://127.0.0.1:27017"

	first, err := pool.get(ctx, mongoClientKey(uri, mongoServerSelectionTimeout()), mongoClientOptions(uri))
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	second, err := pool.get(ctx, mongoClientKey(uri, mongoServerSelectionTimeout()), mongoClientOptions(uri))
	if err != nil {
		t.Fatalf("second get: %v", err)
	}

	if first != second {
		t.Fatal("expected the pooled client to be reused for the same connection string")
	}
	if *connects != 1 {
		t.Fatalf("connect calls: want 1, got %d", *connects)
	}
	if pool.connectCount != 1 {
		t.Fatalf("pool.connectCount: want 1, got %d", pool.connectCount)
	}
	if len(pool.clients) != 1 {
		t.Fatalf("cached clients: want 1, got %d", len(pool.clients))
	}
}

// TestMongoClientPool_DifferentConnStringYieldsDifferentClient proves
// two different clusters never share a client.
func TestMongoClientPool_DifferentConnStringYieldsDifferentClient(t *testing.T) {
	pool, connects, _ := testPool(t)
	ctx := context.Background()

	first, err := pool.get(ctx, mongoClientKey("mongodb://127.0.0.1:27017", mongoServerSelectionTimeout()), mongoClientOptions("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	second, err := pool.get(ctx, mongoClientKey("mongodb://127.0.0.1:27018", mongoServerSelectionTimeout()), mongoClientOptions("mongodb://127.0.0.1:27018"))
	if err != nil {
		t.Fatalf("second get: %v", err)
	}

	if first == second {
		t.Fatal("expected a different client for a different connection string")
	}
	if *connects != 2 {
		t.Fatalf("connect calls: want 2, got %d", *connects)
	}
	if len(pool.clients) != 2 {
		t.Fatalf("cached clients: want 2, got %d", len(pool.clients))
	}
}

// TestMongoClientPool_OptionsFingerprintIsPartOfTheKey proves two
// connections that differ only in their server selection timeout do
// not share a client.
func TestMongoClientPool_OptionsFingerprintIsPartOfTheKey(t *testing.T) {
	pool, connects, _ := testPool(t)
	ctx := context.Background()
	uri := "mongodb://127.0.0.1:27017"

	first, err := pool.get(ctx, mongoClientKey(uri, 5*time.Second), mongoClientOptions(uri))
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	second, err := pool.get(ctx, mongoClientKey(uri, 10*time.Second), mongoClientOptions(uri))
	if err != nil {
		t.Fatalf("second get: %v", err)
	}

	if first == second {
		t.Fatal("expected a different client for a different options fingerprint")
	}
	if *connects != 2 {
		t.Fatalf("connect calls: want 2, got %d", *connects)
	}
}

// TestMongoClientPool_CloseAllReleasesClients proves the shutdown
// hook disconnects every cached client and empties the pool, so a
// later execution connects again instead of handing out a dead
// client.
func TestMongoClientPool_CloseAllReleasesClients(t *testing.T) {
	pool, connects, disconnected := testPool(t)
	ctx := context.Background()
	uri := "mongodb://127.0.0.1:27017"

	if _, err := pool.get(ctx, mongoClientKey(uri, mongoServerSelectionTimeout()), mongoClientOptions(uri)); err != nil {
		t.Fatalf("get: %v", err)
	}
	if _, err := pool.get(ctx, mongoClientKey("mongodb://127.0.0.1:27018", mongoServerSelectionTimeout()), mongoClientOptions("mongodb://127.0.0.1:27018")); err != nil {
		t.Fatalf("get: %v", err)
	}

	if err := pool.closeAll(ctx); err != nil {
		t.Fatalf("closeAll: %v", err)
	}
	if len(pool.clients) != 0 {
		t.Fatalf("cached clients after close: want 0, got %d", len(pool.clients))
	}
	if len(*disconnected) != 2 {
		t.Fatalf("disconnect calls: want 2, got %d", len(*disconnected))
	}

	// A fresh execution after the shutdown must connect again.
	if _, err := pool.get(ctx, mongoClientKey(uri, mongoServerSelectionTimeout()), mongoClientOptions(uri)); err != nil {
		t.Fatalf("get after close: %v", err)
	}
	if *connects != 3 {
		t.Fatalf("connect calls: want 3, got %d", *connects)
	}
}

// TestMongoClientPool_PingFailureDisconnectsAndDoesNotCache proves a
// broken cluster fails fast (no client is cached) and the failed
// client is disconnected.
func TestMongoClientPool_PingFailureDisconnectsAndDoesNotCache(t *testing.T) {
	pool, connects, disconnected := testPool(t)
	pool.pingFn = func(ctx context.Context, client *mongo.Client) error {
		return context.DeadlineExceeded
	}
	ctx := context.Background()
	uri := "mongodb://127.0.0.1:27017"

	_, err := pool.get(ctx, mongoClientKey(uri, mongoServerSelectionTimeout()), mongoClientOptions(uri))
	if err == nil {
		t.Fatal("expected a ping failure to be reported")
	}
	var poolErr *mongoPoolError
	if !errorsAs(err, &poolErr) || poolErr.stage != "ping" {
		t.Fatalf("error stage: want %q, got %v", "ping", err)
	}
	if len(pool.clients) != 0 {
		t.Fatalf("cached clients after a failed ping: want 0, got %d", len(pool.clients))
	}
	if len(*disconnected) != 1 {
		t.Fatalf("disconnect calls: want 1, got %d", len(*disconnected))
	}
	if *connects != 1 {
		t.Fatalf("connect calls: want 1, got %d", *connects)
	}
}

// TestMongoClientKey_IncludesTimeout proves the pool key carries the
// options fingerprint.
func TestMongoClientKey_IncludesTimeout(t *testing.T) {
	uri := "mongodb://127.0.0.1:27017"
	if mongoClientKey(uri, time.Second) == mongoClientKey(uri, 2*time.Second) {
		t.Fatal("expected the pool key to differ for a different server selection timeout")
	}
	if mongoClientKey(uri, time.Second) != mongoClientKey(uri, time.Second) {
		t.Fatal("expected the pool key to be stable for the same inputs")
	}
	if mongoClientKey("mongodb://a", time.Second) == mongoClientKey("mongodb://b", time.Second) {
		t.Fatal("expected the pool key to differ for a different connection string")
	}
}

// TestMongoServerSelectionTimeout proves the timeout comes from the
// environment and falls back to the 10 s default.
func TestMongoServerSelectionTimeout(t *testing.T) {
	t.Setenv(MongoServerSelectionTimeoutEnv, "")
	if got := mongoServerSelectionTimeout(); got != DefaultMongoServerSelectionTimeout {
		t.Fatalf("default timeout: want %s, got %s", DefaultMongoServerSelectionTimeout, got)
	}

	t.Setenv(MongoServerSelectionTimeoutEnv, "3s")
	if got := mongoServerSelectionTimeout(); got != 3*time.Second {
		t.Fatalf("env timeout: want 3s, got %s", got)
	}

	// A bare number is read as seconds.
	t.Setenv(MongoServerSelectionTimeoutEnv, "7")
	if got := mongoServerSelectionTimeout(); got != 7*time.Second {
		t.Fatalf("numeric env timeout: want 7s, got %s", got)
	}

	t.Setenv(MongoServerSelectionTimeoutEnv, "nonsense")
	if got := mongoServerSelectionTimeout(); got != DefaultMongoServerSelectionTimeout {
		t.Fatalf("invalid env timeout: want %s, got %s", DefaultMongoServerSelectionTimeout, got)
	}
}

// errorsAs is a tiny wrapper so the test file does not need the
// errors import in two places.
func errorsAs(err error, target **mongoPoolError) bool {
	for err != nil {
		if pe, ok := err.(*mongoPoolError); ok {
			*target = pe
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}
