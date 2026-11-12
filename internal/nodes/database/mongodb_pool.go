/*
Package database — MongoDB client pool.

Every MongoDB node execution used to open a fresh client, ping the
cluster and disconnect again:

	client, _ := mongo.Connect(ctx, uri)
	defer client.Disconnect(ctx)
	client.Ping(ctx, nil)   // round-trip on every poll

On a remote Atlas cluster that costs ~0.5 s per execution — and a
retry loop executes the Mongo lookup on every iteration, so the
`POST /webhook/post_balance` workflow paid the handshake ten times.

The pool keeps one client per (connection string + options
fingerprint) alive for the lifetime of the process:

  - Connect happens once per key.
  - Ping happens once per key, right after the connect (it proves the
    URI resolves and the cluster is reachable).
  - Node executions never disconnect a shared client.
  - CloseMongoClients disconnects everything on shutdown.

The ServerSelectionTimeout is explicit (10 s by default, tunable via
M9M_MONGO_SERVER_SELECTION_TIMEOUT) so a broken URI fails fast
instead of blocking the driver's default 30 s.
*/
package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// DefaultMongoServerSelectionTimeout is how long the driver
	// waits for a suitable server before failing an operation.
	DefaultMongoServerSelectionTimeout = 10 * time.Second

	// MongoServerSelectionTimeoutEnv overrides the server selection
	// timeout.
	MongoServerSelectionTimeoutEnv = "M9M_MONGO_SERVER_SELECTION_TIMEOUT"

	// mongoConnectTimeout bounds the initial connect + ping.
	mongoConnectTimeout = 15 * time.Second
)

// mongoPoolError carries the stage that failed so the node can keep
// its precise error messages ("mongo connect" vs "mongo ping").
type mongoPoolError struct {
	stage string
	err   error
}

func (e *mongoPoolError) Error() string {
	return fmt.Sprintf("%s: %v", e.stage, e.err)
}

func (e *mongoPoolError) Unwrap() error { return e.err }

// mongoClientPool caches *mongo.Client instances per key.
//
// connectFn / pingFn are injectable so the pool can be unit-tested
// without a MongoDB server.
type mongoClientPool struct {
	mu      sync.Mutex
	clients map[string]*mongo.Client

	connectFn func(ctx context.Context, opts *options.ClientOptions) (*mongo.Client, error)
	pingFn    func(ctx context.Context, client *mongo.Client) error

	// disconnectFn is the shutdown counterpart of connectFn. It is
	// injectable so the pool can be unit-tested without a live
	// cluster (a client that was never connected cannot be
	// disconnected cleanly).
	disconnectFn func(ctx context.Context, client *mongo.Client) error

	// connectCount counts the clients this pool created. Test hook.
	connectCount int
}

// mongoPool is the process-wide pool used by the MongoDB node.
var mongoPool = newMongoClientPool()

// newMongoClientPool creates a pool with the production connect /
// ping implementations.
func newMongoClientPool() *mongoClientPool {
	return &mongoClientPool{
		clients: map[string]*mongo.Client{},
		connectFn: func(ctx context.Context, opts *options.ClientOptions) (*mongo.Client, error) {
			return mongo.Connect(ctx, opts)
		},
		pingFn: func(ctx context.Context, client *mongo.Client) error {
			return client.Ping(ctx, nil)
		},
		disconnectFn: func(ctx context.Context, client *mongo.Client) error {
			return client.Disconnect(ctx)
		},
	}
}

// disconnectClient disconnects `client` through the pool's injectable
// disconnect hook.
func (p *mongoClientPool) disconnectClient(ctx context.Context, client *mongo.Client) error {
	if p.disconnectFn != nil {
		return p.disconnectFn(ctx, client)
	}
	return client.Disconnect(ctx)
}

// get returns the cached client for `key`, creating (and pinging)
// it on first use.
func (p *mongoClientPool) get(ctx context.Context, key string, opts *options.ClientOptions) (*mongo.Client, error) {
	p.mu.Lock()
	client, ok := p.clients[key]
	p.mu.Unlock()
	if ok {
		return client, nil
	}

	// The client outlives this call, so it must not be bound to the
	// caller's (per-operation) context — cancelling that context
	// after the operation must not tear the client down.
	connectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mongoConnectTimeout)
	defer cancel()

	created, err := p.connectFn(connectCtx, opts)
	if err != nil {
		return nil, &mongoPoolError{stage: "connect", err: err}
	}
	// Ping once, at creation time. Later executions reuse the
	// client without a round-trip.
	if err := p.pingFn(connectCtx, created); err != nil {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = p.disconnectClient(disconnectCtx, created)
		return nil, &mongoPoolError{stage: "ping", err: err}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.clients[key]; ok {
		// Another goroutine created the same client meanwhile —
		// keep the cached one and drop ours.
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = p.disconnectClient(disconnectCtx, created)
		return existing, nil
	}
	p.clients[key] = created
	p.connectCount++
	return created, nil
}

// closeAll disconnects every cached client.
func (p *mongoClientPool) closeAll(ctx context.Context) error {
	p.mu.Lock()
	clients := make([]*mongo.Client, 0, len(p.clients))
	for key, client := range p.clients {
		clients = append(clients, client)
		delete(p.clients, key)
	}
	p.mu.Unlock()

	var errs []error
	for _, client := range clients {
		if client == nil {
			continue
		}
		if err := p.disconnectClient(ctx, client); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// reset drops every cached client without disconnecting it. Test
// hook only.
func (p *mongoClientPool) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients = map[string]*mongo.Client{}
	p.connectCount = 0
}

// CloseMongoClients disconnects every cached MongoDB client.
//
// Node executions never disconnect a shared client, so this is the
// explicit shutdown hook — call it when the process stops serving
// (or when the credentials/connection string rotate).
func CloseMongoClients(ctx context.Context) error {
	return mongoPool.closeAll(ctx)
}

// mongoClientKey builds the pool key: the connection string plus the
// options fingerprint, so two connections that differ only in their
// server selection timeout do not share a client.
func mongoClientKey(connStr string, timeout time.Duration) string {
	return connStr + "|serverSelectionTimeout=" + strconv.FormatInt(int64(timeout), 10)
}

// mongoClientOptions builds the driver options for a connection
// string. The server selection timeout is explicit so a broken URI
// fails in 10 s instead of the driver's 30 s default.
func mongoClientOptions(connStr string) *options.ClientOptions {
	return options.Client().
		ApplyURI(connStr).
		SetServerSelectionTimeout(mongoServerSelectionTimeout())
}

// mongoServerSelectionTimeout reads the server selection timeout
// from the environment, falling back to the 10 s default.
func mongoServerSelectionTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv(MongoServerSelectionTimeoutEnv))
	if raw == "" {
		return DefaultMongoServerSelectionTimeout
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	return DefaultMongoServerSelectionTimeout
}
