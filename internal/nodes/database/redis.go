package database

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// RedisHTTPClient abstracts the HTTP client for testing. Used by
// the HTTP backend so tests can inject a stub without standing up
// a real REST-to-Redis gateway.
type RedisHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// redisBackend is the abstraction the Redis node uses to dispatch
// commands. The two production implementations are:
//
//   - nativeBackend: speaks RESP directly over TCP (no proxy
//     required). Preferred in production and unit tests; this is
//     what `mode=native` selects.
//   - httpBackend:    POSTs a JSON-encoded argv to a REST gateway
//     (Upstash, a sidecar, or a self-hosted redis-rest). Used when
//     the operator must route through a managed / proxied service
//     (e.g. serverless).
//
// The backend is decided per-Execute call so a single Redis node
// can flip between modes without re-instantiation.
type redisBackend interface {
	Do(ctx context.Context, args []interface{}) (interface{}, error)
	Close() error
}

// RedisNode implements Redis operations. The node supports two
// transport modes (`mode: native | http`); the default is `native`
// so workflows no longer require a sidecar proxy. The HTTP mode
// is preserved for deployments behind a managed REST gateway
// (Upstash, AWS MemoryDB with a custom gateway, etc.).
type RedisNode struct {
	*base.BaseNode
	httpClient RedisHTTPClient

	mu      sync.Mutex
	backends map[string]redisBackend
}

// NewRedisNode creates a new Redis node with the native RESP
// backend pre-wired.
func NewRedisNode() *RedisNode {
	return &RedisNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Redis",
			Description: "Execute Redis commands",
			Category:    "Database",
			Properties:  redisProperties(),
			Inputs:      []string{"main"},
			Outputs:     []string{"main"},
		}),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		backends:   make(map[string]redisBackend),
	}
}

// NewRedisNodeWithClient creates a Redis node with a custom HTTP
// client (used by tests that inject a stub gateway).
func NewRedisNodeWithClient(client RedisHTTPClient) *RedisNode {
	n := NewRedisNode()
	n.httpClient = client
	return n
}

// redisProperties returns the Redis node's property descriptors.
// Mirrors n8n's INodeTypeDescription.properties: a Mode switch
// (native / http), a Host/Port pair for the native backend, a
// baseUrl for the HTTP backend, and operation / key / value
// inputs that follow n8n's existing shape.
func redisProperties() []base.NodeProperty {
	modes := []base.Option{
		{Name: "Native (TCP / RESP)", Value: "native"},
		{Name: "HTTP (REST gateway)", Value: "http"},
	}
	ops := []base.Option{
		{Name: "Get", Value: "get"},
		{Name: "Set", Value: "set"},
		{Name: "Delete", Value: "delete"},
		{Name: "Keys", Value: "keys"},
		{Name: "Hash Get", Value: "hget"},
		{Name: "Hash Set", Value: "hset"},
		{Name: "List Push Left", Value: "lpush"},
		{Name: "List Push Right", Value: "rpush"},
		{Name: "List Range", Value: "lrange"},
		{Name: "Increment", Value: "incr"},
		{Name: "Expire", Value: "expire"},
		{Name: "Publish", Value: "publish"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Mode", "mode", "native", "Transport for Redis commands.", modes, true),
		base.StringProp("Host", "host", "localhost", "Redis server host (native mode).", "127.0.0.1", false),
		base.NumberProp("Port", "port", 6379, "Redis server port (native mode).", false),
		base.StringProp("Username", "username", "", "Optional ACL username (Redis 6+).", "default", false),
		base.StringProp("Password", "password", "", "AUTH password (or token for the HTTP gateway).", "********", false),
		base.NumberProp("Database", "db", 0, "Logical database index.", false),
		base.BoolProp("TLS", "tls", false, "Use TLS for the native connection."),
		base.StringProp("HTTP base URL", "baseUrl", "", "REST gateway URL (http mode).", "http://localhost:6380", false),
		base.StringOpt("Operation", "operation", "get", "Redis command to run.", ops, true),
		base.StringProp("Key", "key", "", "The Redis key to operate on.", "session:1234", false),
		base.StringProp("Field", "field", "", "Hash field (hget / hset).", "name", false),
		base.StringProp("Value", "value", "", "Value to write (set / hset / lpush / rpush).", "", false),
		base.StringProp("Pattern", "pattern", "", "Glob pattern (keys).", "*", false),
		base.NumberProp("Start", "start", 0, "LRANGE start index.", false),
		base.NumberProp("Stop", "stop", -1, "LRANGE stop index (-1 = end).", false),
		base.NumberProp("Seconds", "seconds", 60, "EXPIRE TTL in seconds.", false),
		base.StringProp("Channel", "channel", "", "Pub/Sub channel (publish).", "events", false),
	}
	return append(props, base.CommonSettings()...)
}

// backend resolves (and caches) the per-execution backend. The
// cache key is the mode + host + port + db + tls + user so the
// same connection is reused across items in a workflow without
// paying the TCP handshake cost on every call.
func (n *RedisNode) backend(params map[string]interface{}) (redisBackend, error) {
	mode := strings.ToLower(strings.TrimSpace(n.GetStringParameter(params, "mode", "native")))
	switch mode {
	case "http", "":
		return newHTTPBackend(n.httpClient, params), nil
	case "native":
		host := n.GetStringParameter(params, "host", "localhost")
		port := n.GetIntParameter(params, "port", 6379)
		db := n.GetIntParameter(params, "db", 0)
		tls := n.GetBoolParameter(params, "tls", false)
		user := n.GetStringParameter(params, "username", "")
		pass := n.GetStringParameter(params, "password", "")
		key := fmt.Sprintf("native|%s|%d|%d|%v|%s", host, port, db, tls, user)
		if pass != "" {
			key = fmt.Sprintf("%s|p:%d", key, len(pass))
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		if b, ok := n.backends[key]; ok {
			return b, nil
		}
		b, err := newNativeBackend(host, port, pass, user, db, tls)
		if err != nil {
			return nil, err
		}
		n.backends[key] = b
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported redis mode: %q (expected 'native' or 'http')", mode)
	}
}

// Execute runs the configured Redis operation.
func (n *RedisNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	operation := n.GetStringParameter(nodeParams, "operation", "get")
	be, err := n.backend(nodeParams)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}

	args, err := redisArgsForOperation(operation, nodeParams)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}

	result, err := be.Do(context.Background(), args)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("redis %s failed: %v", operation, err), nil)
	}

	return []model.DataItem{{
		JSON: map[string]interface{}{
			"operation": operation,
			"result":    result,
		},
	}}, nil
}

// redisArgsForOperation builds the RESP argv for a high-level
// operation. Centralised so the operation-to-RESP mapping has one
// source of truth.
func redisArgsForOperation(op string, params map[string]interface{}) ([]interface{}, error) {
	key := func() (string, error) {
		k := strings.TrimSpace(stringParam(params, "key"))
		if k == "" {
			return "", fmt.Errorf("key is required for %s", strings.ToUpper(op))
		}
		return k, nil
	}
	switch op {
	case "get":
		k, err := key()
		if err != nil {
			return nil, err
		}
		return []interface{}{"GET", k}, nil
	case "set":
		k, err := key()
		if err != nil {
			return nil, err
		}
		if _, ok := params["value"]; !ok {
			return nil, fmt.Errorf("value is required for SET")
		}
		return []interface{}{"SET", k, params["value"]}, nil
	case "delete":
		k, err := key()
		if err != nil {
			return nil, err
		}
		return []interface{}{"DEL", k}, nil
	case "keys":
		pat := strings.TrimSpace(stringParam(params, "pattern"))
		if pat == "" {
			pat = "*"
		}
		return []interface{}{"KEYS", pat}, nil
	case "hget":
		k, err := key()
		if err != nil {
			return nil, err
		}
		f := stringParam(params, "field")
		if f == "" {
			return nil, fmt.Errorf("field is required for HGET")
		}
		return []interface{}{"HGET", k, f}, nil
	case "hset":
		k, err := key()
		if err != nil {
			return nil, err
		}
		f := stringParam(params, "field")
		if f == "" {
			return nil, fmt.Errorf("field is required for HSET")
		}
		if _, ok := params["value"]; !ok {
			return nil, fmt.Errorf("value is required for HSET")
		}
		return []interface{}{"HSET", k, f, params["value"]}, nil
	case "lpush", "rpush":
		k, err := key()
		if err != nil {
			return nil, err
		}
		if _, ok := params["value"]; !ok {
			return nil, fmt.Errorf("value is required for %s", strings.ToUpper(op))
		}
		return []interface{}{strings.ToUpper(op), k, params["value"]}, nil
	case "lrange":
		k, err := key()
		if err != nil {
			return nil, err
		}
		return []interface{}{"LRANGE", k, intParam(params, "start", 0), intParam(params, "stop", -1)}, nil
	case "incr":
		k, err := key()
		if err != nil {
			return nil, err
		}
		return []interface{}{"INCR", k}, nil
	case "expire":
		k, err := key()
		if err != nil {
			return nil, err
		}
		return []interface{}{"EXPIRE", k, intParam(params, "seconds", 60)}, nil
	case "publish":
		ch := stringParam(params, "channel")
		if ch == "" {
			return nil, fmt.Errorf("channel is required for PUBLISH")
		}
		if _, ok := params["value"]; !ok {
			return nil, fmt.Errorf("value is required for PUBLISH")
		}
		return []interface{}{"PUBLISH", ch, params["value"]}, nil
	default:
		return nil, fmt.Errorf("unsupported operation: %s", op)
	}
}

func stringParam(p map[string]interface{}, k string) string {
	if v, ok := p[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func intParam(p map[string]interface{}, k string, def int) int {
	if v, ok := p[k]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int32:
			return int(n)
		case int64:
			return int(n)
		case float64:
			return int(n)
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				return i
			}
		}
	}
	return def
}

// nativeBackend speaks RESP over TCP. The implementation is
// minimal (a single connection guarded by a mutex) but covers the
// shape of every command the Redis node issues: simple strings,
// bulk strings, integers, arrays, errors, and nil. Authentication
// is performed on the first call; subsequent calls reuse the same
// authenticated connection until the operator restarts the node.
type nativeBackend struct {
	addr     string
	password string
	username string
	db       int
	tls      bool

	mu   sync.Mutex
	conn net.Conn
	rd   *bufio.Reader
}

func newNativeBackend(host string, port int, password, username string, db int, useTLS bool) (*nativeBackend, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	b := &nativeBackend{
		addr:     addr,
		password: password,
		username: username,
		db:       db,
		tls:      useTLS,
	}
	if err := b.connect(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *nativeBackend) connect() error {
	var (
		conn net.Conn
		err  error
	)
	d := net.Dialer{Timeout: 5 * time.Second}
	if b.tls {
		conn, err = tls.DialWithDialer(&d, "tcp", b.addr, &tls.Config{})
	} else {
		conn, err = d.Dial("tcp", b.addr)
	}
	if err != nil {
		return fmt.Errorf("connect %s: %w", b.addr, err)
	}
	b.conn = conn
	b.rd = bufio.NewReader(conn)

	if b.password != "" {
		var authArgs []interface{}
		if b.username != "" {
			authArgs = []interface{}{"AUTH", b.username, b.password}
		} else {
			authArgs = []interface{}{"AUTH", b.password}
		}
		if err := b.writeCommand(authArgs); err != nil {
			return fmt.Errorf("auth write: %w", err)
		}
		v, err := b.readReply()
		if err != nil {
			return fmt.Errorf("auth reply: %w", err)
		}
		if e, ok := v.(error); ok {
			return fmt.Errorf("auth: %w", e)
		}
	}
	if b.db != 0 {
		if err := b.writeCommand([]interface{}{"SELECT", b.db}); err != nil {
			return err
		}
		if _, err := b.readReply(); err != nil {
			return err
		}
	}
	return nil
}

func (b *nativeBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		err := b.conn.Close()
		b.conn = nil
		return err
	}
	return nil
}

func (b *nativeBackend) Do(ctx context.Context, args []interface{}) (interface{}, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		if err := b.connect(); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := b.writeCommand(args); err != nil {
		_ = b.conn.Close()
		b.conn = nil
		return nil, err
	}
	v, err := b.readReply()
	if err != nil {
		_ = b.conn.Close()
		b.conn = nil
		return nil, err
	}
	return v, nil
}

func (b *nativeBackend) writeCommand(args []interface{}) error {
	var sb strings.Builder
	sb.WriteByte('*')
	sb.WriteString(strconv.Itoa(len(args)))
	sb.WriteString("\r\n")
	for _, a := range args {
		var s string
		switch v := a.(type) {
		case string:
			s = v
		case []byte:
			s = string(v)
		case int:
			s = strconv.Itoa(v)
		case int64:
			s = strconv.FormatInt(v, 10)
		case float64:
			s = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			if v {
				s = "1"
			} else {
				s = "0"
			}
		case nil:
			s = ""
		default:
			s = fmt.Sprintf("%v", v)
		}
		sb.WriteByte('$')
		sb.WriteString(strconv.Itoa(len(s)))
		sb.WriteString("\r\n")
		sb.WriteString(s)
		sb.WriteString("\r\n")
	}
	_ = b.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := b.conn.Write([]byte(sb.String()))
	return err
}

// readReply consumes one RESP frame and returns it as a Go value.
// Simple Strings -> string, Errors -> error, Integers -> int64,
// Bulk Strings -> string (or nil), Arrays -> []interface{}.
func (b *nativeBackend) readReply() (interface{}, error) {
	_ = b.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	line, err := b.rd.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 {
		return nil, fmt.Errorf("short resp frame: %q", line)
	}
	prefix := line[0]
	payload := strings.TrimSuffix(strings.TrimSuffix(line[1:], "\n"), "\r")
	switch prefix {
	case '+':
		return payload, nil
	case '-':
		return errors.New(payload), nil
	case ':':
		n, err := strconv.ParseInt(payload, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int reply: %q", payload)
		}
		return n, nil
	case '$':
		size, err := strconv.Atoi(payload)
		if err != nil {
			return nil, fmt.Errorf("invalid bulk length: %q", payload)
		}
		if size < 0 {
			return nil, nil
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(b.rd, buf); err != nil {
			return nil, err
		}
		return string(buf[:size]), nil
	case '*':
		count, err := strconv.Atoi(payload)
		if err != nil {
			return nil, fmt.Errorf("invalid array length: %q", payload)
		}
		if count < 0 {
			return nil, nil
		}
		out := make([]interface{}, count)
		for i := 0; i < count; i++ {
			v, err := b.readReply()
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown resp prefix %q in frame %q", prefix, line)
	}
}

// httpBackend POSTs the argv as JSON to a REST gateway. Kept for
// managed / proxied Redis deployments where TCP is not reachable
// from the m9m container.
type httpBackend struct {
	client   RedisHTTPClient
	baseURL  string
	password string
}

func newHTTPBackend(client RedisHTTPClient, params map[string]interface{}) *httpBackend {
	baseURL := strings.TrimRight(stringParam(params, "baseUrl"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:6380"
	}
	return &httpBackend{
		client:   client,
		baseURL:  baseURL,
		password: stringParam(params, "password"),
	}
}

func (b *httpBackend) Do(ctx context.Context, args []interface{}) (interface{}, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", b.baseURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.password != "" {
		req.Header.Set("Authorization", "Bearer "+b.password)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		result = string(respBody)
	}
	return result, nil
}

func (b *httpBackend) Close() error { return nil }

// ValidateParameters validates Redis node parameters.
func (n *RedisNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return n.CreateError("parameters cannot be nil", nil)
	}

	operation := n.GetStringParameter(params, "operation", "get")
	validOps := map[string]bool{
		"get": true, "set": true, "delete": true, "keys": true,
		"hget": true, "hset": true, "lpush": true, "rpush": true,
		"lrange": true, "incr": true, "expire": true, "publish": true,
	}
	if !validOps[operation] {
		return n.CreateError(fmt.Sprintf("invalid operation: %s", operation), nil)
	}

	mode := strings.ToLower(strings.TrimSpace(n.GetStringParameter(params, "mode", "native")))
	if mode != "native" && mode != "http" {
		return n.CreateError(fmt.Sprintf("invalid mode: %s (expected native or http)", mode), nil)
	}

	return nil
}
