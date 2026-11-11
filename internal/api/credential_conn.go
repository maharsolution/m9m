package api

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	// Database drivers, registered for the credential connection test.
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// credentialConnectTimeout bounds every outbound connection attempt.
// A credential test must never hang the API request.
const credentialConnectTimeout = 10 * time.Second

// testCredentialConnection performs a REAL connection attempt for the
// credential types where that is meaningful, so a wrong password is
// reported as a failure instead of "valid".
//
// The second return value tells the caller whether a connection was
// actually attempted. Types we cannot verify over the wire (api keys
// without a known endpoint, JWT secrets, Discord webhooks, …) keep the
// static validation only.
//
// The message never contains the password: driver errors are scrubbed
// before they are returned to the caller.
func testCredentialConnection(credType string, data map[string]interface{}) (testCredentialResult, bool) {
	switch credType {
	case "postgres", "postgresql":
		return testPostgresConnection(data)
	case "mysql", "mySql":
		return testMySQLConnection(data)
	case "mongoDb", "mongodb":
		return testMongoConnection(data)
	case "redis":
		return testRedisConnection(data)
	case "kafka":
		return testKafkaConnection(data)
	case "rabbitmq":
		return testRabbitMQConnection(data)
	case "openai", "openAiApi":
		return testOpenAIConnection(data)
	case "anthropic":
		return testAnthropicConnection(data)
	case "slack":
		return testSlackConnection(data)
	}
	return testCredentialResult{}, false
}

// ---------------------------------------------------------------------------
// Database credentials
// ---------------------------------------------------------------------------

func testPostgresConnection(data map[string]interface{}) (testCredentialResult, bool) {
	host := dataString(data, "host")
	if host == "" {
		return testCredentialResult{}, false
	}
	port := dataInt(data, "port", 5432)
	database := dataString(data, "database")
	user := dataString(data, "user")
	password := dataString(data, "password")

	sslMode := "disable"
	if s := dataString(data, "ssl"); s != "" {
		sslMode = s
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/" + database,
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()

	db, err := sql.Open("postgres", u.String())
	if err != nil {
		return postgresResult(err, password), true
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), credentialConnectTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return postgresResult(err, password), true
	}
	return testCredentialResult{
		Status:  "OK",
		Message: fmt.Sprintf("Connected to PostgreSQL %s/%s as %s", host, database, user),
	}, true
}

func postgresResult(err error, password string) testCredentialResult {
	return testCredentialResult{
		Status:  "Error",
		Message: "PostgreSQL connection failed: " + scrubSecret(err.Error(), password),
	}
}

func testMySQLConnection(data map[string]interface{}) (testCredentialResult, bool) {
	host := dataString(data, "host")
	if host == "" {
		return testCredentialResult{}, false
	}
	port := dataInt(data, "port", 3306)
	database := dataString(data, "database")
	user := dataString(data, "user")
	password := dataString(data, "password")

	tlsParam := ""
	switch v := data["ssl"].(type) {
	case bool:
		if v {
			tlsParam = "&tls=true"
		} else {
			tlsParam = "&tls=false"
		}
	case string:
		if v != "" {
			tlsParam = "&tls=" + url.QueryEscape(v)
		}
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?timeout=8s&readTimeout=8s&writeTimeout=8s%s",
		url.QueryEscape(user),
		url.QueryEscape(password),
		net.JoinHostPort(host, strconv.Itoa(port)),
		database,
		tlsParam,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "MySQL connection failed: " + scrubSecret(err.Error(), password),
		}, true
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), credentialConnectTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "MySQL connection failed: " + scrubSecret(err.Error(), password),
		}, true
	}
	return testCredentialResult{
		Status:  "OK",
		Message: fmt.Sprintf("Connected to MySQL %s/%s as %s", host, database, user),
	}, true
}

func testMongoConnection(data map[string]interface{}) (testCredentialResult, bool) {
	uri := dataString(data, "connectionString")
	if uri == "" {
		host := dataString(data, "host")
		if host == "" {
			return testCredentialResult{}, false
		}
		port := dataInt(data, "port", 27017)
		user := dataString(data, "user")
		password := dataString(data, "password")

		var userInfo *url.Userinfo
		if user != "" {
			userInfo = url.UserPassword(user, password)
		}
		u := &url.URL{
			Scheme: "mongodb",
			User:   userInfo,
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		}
		uri = u.String()
	}

	ctx, cancel := context.WithTimeout(context.Background(), credentialConnectTimeout)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "MongoDB connection failed: " + scrubSecret(err.Error(), dataString(data, "password")),
		}, true
	}
	defer func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = client.Disconnect(disconnectCtx)
	}()

	if err := client.Ping(ctx, nil); err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "MongoDB connection failed: " + scrubSecret(err.Error(), dataString(data, "password")),
		}, true
	}
	return testCredentialResult{
		Status:  "OK",
		Message: "Connected to MongoDB" + mongoTarget(data),
	}, true
}

func mongoTarget(data map[string]interface{}) string {
	if db := dataString(data, "database"); db != "" {
		return " database " + db
	}
	return ""
}

// testRedisConnection speaks RESP directly (the same way the Redis node
// does) to validate the password with a real AUTH round-trip.
func testRedisConnection(data map[string]interface{}) (testCredentialResult, bool) {
	host := dataString(data, "host")
	if host == "" {
		return testCredentialResult{}, false
	}
	port := dataInt(data, "port", 6379)
	password := dataString(data, "password")
	user := dataString(data, "user")

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "Redis connection failed: " + scrubSecret(err.Error(), password),
		}, true
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(credentialConnectTimeout))

	if password != "" {
		var args []string
		if user != "" && user != "default" {
			args = []string{"AUTH", user, password}
		} else {
			args = []string{"AUTH", password}
		}
		reply, err := redisCommand(conn, args...)
		if err != nil {
			return testCredentialResult{
				Status:  "Error",
				Message: "Redis connection failed: " + scrubSecret(err.Error(), password),
			}, true
		}
		if strings.HasPrefix(reply, "-") {
			return testCredentialResult{
				Status:  "Error",
				Message: "Redis authentication failed: " + scrubSecret(strings.TrimPrefix(reply, "-"), password),
			}, true
		}
	}

	reply, err := redisCommand(conn, "PING")
	if err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "Redis connection failed: " + scrubSecret(err.Error(), password),
		}, true
	}
	if strings.HasPrefix(reply, "-") {
		return testCredentialResult{
			Status:  "Error",
			Message: "Redis check failed: " + scrubSecret(strings.TrimPrefix(reply, "-"), password),
		}, true
	}
	return testCredentialResult{
		Status:  "OK",
		Message: fmt.Sprintf("Connected to Redis %s", addr),
	}, true
}

// redisCommand writes one RESP command and reads the reply.
func redisCommand(conn net.Conn, args ...string) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&sb, "$%d\r\n%s\r\n", len(arg), arg)
	}
	if _, err := conn.Write([]byte(sb.String())); err != nil {
		return "", err
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(buf[:n])), nil
}

// ---------------------------------------------------------------------------
// Broker credentials
// ---------------------------------------------------------------------------

// testKafkaConnection checks that the bootstrap brokers are reachable.
// SASL credentials are not verifiable with a plain dial, so the message
// says so explicitly instead of pretending the password was checked.
func testKafkaConnection(data map[string]interface{}) (testCredentialResult, bool) {
	brokers := dataString(data, "brokers")
	if brokers == "" {
		return testCredentialResult{}, false
	}
	var first string
	for _, b := range strings.Split(brokers, ",") {
		if b = strings.TrimSpace(b); b != "" {
			first = b
			break
		}
	}
	if first == "" {
		return testCredentialResult{}, false
	}
	if !strings.Contains(first, ":") {
		first = net.JoinHostPort(first, "9092")
	}
	conn, err := net.DialTimeout("tcp", first, 5*time.Second)
	if err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "Kafka connection failed: " + err.Error(),
		}, true
	}
	_ = conn.Close()
	return testCredentialResult{
		Status:  "OK",
		Message: fmt.Sprintf("Kafka broker %s reachable (SASL credentials not verified)", first),
	}, true
}

// testRabbitMQConnection checks that the AMQP broker is reachable.
func testRabbitMQConnection(data map[string]interface{}) (testCredentialResult, bool) {
	host := dataString(data, "host")
	if host == "" {
		return testCredentialResult{}, false
	}
	port := dataInt(data, "port", 5672)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "RabbitMQ connection failed: " + err.Error(),
		}, true
	}
	_ = conn.Close()
	return testCredentialResult{
		Status:  "OK",
		Message: fmt.Sprintf("RabbitMQ broker %s reachable (credentials not verified)", addr),
	}, true
}

// ---------------------------------------------------------------------------
// HTTP API credentials
// ---------------------------------------------------------------------------

func testOpenAIConnection(data map[string]interface{}) (testCredentialResult, bool) {
	key := dataString(data, "apiKey")
	if key == "" {
		return testCredentialResult{}, false
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return testCredentialResult{Status: "Error", Message: "OpenAI test failed: " + err.Error()}, true
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, body, err := doCredentialHTTP(req)
	if err != nil {
		return testCredentialResult{Status: "Error", Message: "OpenAI request failed: " + scrubSecret(err.Error(), key)}, true
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return testCredentialResult{Status: "OK", Message: "OpenAI API key is valid"}, true
	}
	return testCredentialResult{
		Status:  "Error",
		Message: fmt.Sprintf("OpenAI rejected the API key (HTTP %d): %s", resp.StatusCode, truncate(body, 180)),
	}, true
}

func testAnthropicConnection(data map[string]interface{}) (testCredentialResult, bool) {
	key := dataString(data, "apiKey")
	if key == "" {
		return testCredentialResult{}, false
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return testCredentialResult{Status: "Error", Message: "Anthropic test failed: " + err.Error()}, true
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, body, err := doCredentialHTTP(req)
	if err != nil {
		return testCredentialResult{Status: "Error", Message: "Anthropic request failed: " + scrubSecret(err.Error(), key)}, true
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return testCredentialResult{Status: "OK", Message: "Anthropic API key is valid"}, true
	}
	return testCredentialResult{
		Status:  "Error",
		Message: fmt.Sprintf("Anthropic rejected the API key (HTTP %d): %s", resp.StatusCode, truncate(body, 180)),
	}, true
}

func testSlackConnection(data map[string]interface{}) (testCredentialResult, bool) {
	token := dataString(data, "accessToken")
	if token == "" {
		return testCredentialResult{}, false
	}
	req, err := http.NewRequest(http.MethodGet, "https://slack.com/api/auth.test", nil)
	if err != nil {
		return testCredentialResult{Status: "Error", Message: "Slack test failed: " + err.Error()}, true
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, body, err := doCredentialHTTP(req)
	if err != nil {
		return testCredentialResult{Status: "Error", Message: "Slack request failed: " + scrubSecret(err.Error(), token)}, true
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.Contains(body, "\"ok\":true") {
		return testCredentialResult{Status: "OK", Message: "Slack token is valid"}, true
	}
	return testCredentialResult{
		Status:  "Error",
		Message: fmt.Sprintf("Slack rejected the token (HTTP %d): %s", resp.StatusCode, truncate(body, 180)),
	}, true
}

func doCredentialHTTP(req *http.Request) (*http.Response, string, error) {
	client := &http.Client{Timeout: credentialConnectTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	return resp, string(buf[:n]), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// credentialFieldAliases maps a schema field to the alternative keys a
// synced n8n credential may use (rabbitmq stores `hostname` /
// `username` where the schema declares `host` / `user`).
var credentialFieldAliases = map[string][]string{
	"host":     {"hostname", "server", "serverName"},
	"user":     {"username", "login", "loginName"},
	"database": {"db", "dbName", "databaseName"},
	"password": {"pass", "pwd"},
}

// credentialFieldValue returns the value of a schema field, falling back
// to the alias keys used by synced credentials.
func credentialFieldValue(data map[string]interface{}, field string) (interface{}, bool) {
	if v, ok := data[field]; ok {
		return v, true
	}
	for _, alias := range credentialFieldAliases[field] {
		if v, ok := data[alias]; ok {
			return v, true
		}
	}
	return nil, false
}

// dataString reads a string-ish field from the credential payload.
// Alias keys are honoured so a synced credential is not reported as
// missing its host or user.
func dataString(data map[string]interface{}, key string) string {
	v, ok := credentialFieldValue(data, key)
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

// dataInt reads an int-ish field from the credential payload.
func dataInt(data map[string]interface{}, key string, def int) int {
	switch v := data[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// scrubSecret removes the password from a driver error message so the
// secret never leaks back to the browser.
func scrubSecret(msg, secret string) string {
	if secret == "" {
		return msg
	}
	return strings.ReplaceAll(msg, secret, "***")
}

// truncate shortens an API error body for display.
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
