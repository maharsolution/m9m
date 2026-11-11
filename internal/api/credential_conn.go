package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
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
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
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

// testKafkaConnection authenticates with the stored credentials —
// against the Kafka brokers (native SASL) or against a
// Confluent-compatible REST Proxy (HTTP Basic auth / API key).
//
// A plain TCP dial would not prove anything about the password, so the
// SASL handshake runs and a metadata request must be answered. The
// username / password come from kafkaCredentialUserPass: the canonical
// keys are `username` / `password` (n8n parity), the legacy
// `saslUsername` / `saslPassword` spellings are honoured as aliases.
func testKafkaConnection(data map[string]interface{}) (testCredentialResult, bool) {
	// The login comes from the canonical `username` / `password` keys
	// (n8n parity) with the legacy `saslUsername` / `saslPassword`
	// spellings honoured as aliases.
	user, password := kafkaCredentialUserPass(data)
	mechanism := kafkaSASLMechanism(data)
	brokers := kafkaBrokerList(data)
	restURL := strings.TrimSpace(kafkaExactString(data, "restProxyUrl"))
	authMode, explicitAuth := kafkaRESTAuthMode(data)

	// REST-proxy mode: the credential selects HTTP auth (Basic / API
	// key) or only carries a REST Proxy URL. The single
	// username / password pair is then exercised as HTTP Basic auth.
	if explicitAuth {
		if restURL == "" {
			return testCredentialResult{
				Status:  "Error",
				Message: "Kafka REST Proxy URL (restProxyUrl) is required when the authentication mode is " + string(authMode),
			}, true
		}
		return testKafkaRESTConnection(restURL, authMode, user, password, data)
	}
	if restURL != "" && len(brokers) == 0 {
		return testKafkaRESTConnection(restURL, authMode, user, password, data)
	}
	if len(brokers) == 0 {
		return testCredentialResult{}, false
	}

	tlsCfg, err := kafkaTLSConfig(data)
	if err != nil {
		return testCredentialResult{Status: "Error", Message: err.Error()}, true
	}

	// Nothing configured — no mechanism, no login. The broker may or
	// may not allow anonymous metadata requests; that is a legitimate
	// configuration, so probe without SASL.
	if mechanism == "" && user == "" && password == "" {
		count, err := kafkaMetadataFunc(brokers, tlsCfg, nil, user, password)
		if err != nil {
			return testCredentialResult{
				Status:  "Error",
				Message: "Kafka connection failed: " + scrubKafkaSecrets(err.Error(), user, password),
			}, true
		}
		return testCredentialResult{
			Status:  "OK",
			Message: fmt.Sprintf("Connected to Kafka broker(s) %s (%d broker(s) reported)", strings.Join(brokers, ", "), count),
		}, true
	}

	// SASL is expected — a mechanism was picked or a login was stored —
	// but there is no username. Say so explicitly instead of silently
	// falling back to an unauthenticated probe the broker happens to
	// allow: that reported a misleading OK for a wrong username.
	if user == "" {
		return testCredentialResult{
			Status:  "Error",
			Message: "Kafka authentication failed: username is required for SASL authentication",
		}, true
	}

	mechanisms, err := kafkaSASLMechanisms(mechanism, user, password)
	if err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "Kafka authentication failed: " + scrubKafkaSecrets(err.Error(), user, password),
		}, true
	}

	var lastErr error
	for _, mech := range mechanisms {
		count, err := kafkaMetadataFunc(brokers, tlsCfg, mech, user, password)
		if err == nil {
			return testCredentialResult{
				Status: "OK",
				Message: fmt.Sprintf("Connected to Kafka broker(s) %s (authenticated as %s, %d broker(s) reported)",
					strings.Join(brokers, ", "), user, count),
			}, true
		}
		lastErr = err
		if !isKafkaAuthError(err) {
			break
		}
	}

	if isKafkaAuthError(lastErr) {
		return testCredentialResult{
			Status:  "Error",
			Message: "Kafka authentication failed: " + scrubKafkaSecrets(lastErr.Error(), user, password),
		}, true
	}
	return testCredentialResult{
		Status:  "Error",
		Message: "Kafka connection failed: " + scrubKafkaSecrets(lastErr.Error(), user, password),
	}, true
}

// kafkaAuthMode is the REST Proxy authentication mode of a Kafka
// credential.
type kafkaAuthMode string

const (
	kafkaAuthNone   kafkaAuthMode = "none"
	kafkaAuthBasic  kafkaAuthMode = "basic"
	kafkaAuthAPIKey kafkaAuthMode = "apiKey"
)

// kafkaRESTAuthMode reads the credential's `authentication` selector.
//
// m9m's REST-proxy mode uses `none` / `basic` / `apiKey`; n8n stores a
// boolean in the same field (`true` means "the username / password are
// required"). Only an explicit Basic / API-key selection puts the
// credential into REST-proxy mode — an n8n-synced credential keeps the
// native SASL path, which is what n8n itself does.
func kafkaRESTAuthMode(data map[string]interface{}) (kafkaAuthMode, bool) {
	raw := strings.ToLower(strings.TrimSpace(kafkaExactString(data, "authentication")))
	switch raw {
	case "basic":
		return kafkaAuthBasic, true
	case "apikey", "api-key", "api_key":
		return kafkaAuthAPIKey, true
	}
	return kafkaAuthNone, false
}

// testKafkaRESTConnection probes a Confluent-compatible REST Proxy with
// the credential's authentication settings. A wrong username or
// password comes back as HTTP 401 / 403 and is reported as an
// authentication failure — never as a successful connection.
func testKafkaRESTConnection(restURL string, mode kafkaAuthMode, user, password string, data map[string]interface{}) (testCredentialResult, bool) {
	apiKey := strings.TrimSpace(kafkaExactString(data, "apiKey"))
	switch mode {
	case kafkaAuthBasic:
		if user == "" {
			return testCredentialResult{
				Status:  "Error",
				Message: "Kafka authentication failed: username is required for Basic authentication against the REST Proxy",
			}, true
		}
	case kafkaAuthAPIKey:
		if apiKey == "" {
			return testCredentialResult{
				Status:  "Error",
				Message: "Kafka authentication failed: an API key is required for API key authentication against the REST Proxy",
			}, true
		}
	}

	endpoint := strings.TrimRight(restURL, "/") + "/topics"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return testCredentialResult{Status: "Error", Message: "Kafka connection failed: " + err.Error()}, true
	}
	switch mode {
	case kafkaAuthBasic:
		req.SetBasicAuth(user, password)
	case kafkaAuthAPIKey:
		req.Header.Set("X-Api-Key", apiKey)
	}

	resp, body, err := kafkaRESTProbe(req)
	if err != nil {
		return testCredentialResult{
			Status:  "Error",
			Message: "Kafka connection failed: " + scrubKafkaSecrets(err.Error(), user, password),
		}, true
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return testCredentialResult{
			Status:  "Error",
			Message: fmt.Sprintf("Kafka authentication failed: the REST Proxy rejected the credentials (HTTP %d)", resp.StatusCode),
		}, true
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return testCredentialResult{
			Status:  "OK",
			Message: fmt.Sprintf("Connected to the Kafka REST Proxy at %s", restURL),
		}, true
	}
	return testCredentialResult{
		Status:  "Error",
		Message: fmt.Sprintf("Kafka connection failed: the REST Proxy returned HTTP %d: %s", resp.StatusCode, truncate(body, 180)),
	}, true
}

// kafkaCredentialUserPass resolves the single username / password pair
// of a Kafka credential.
//
// The canonical keys are `username` / `password` — n8n parity, see
// n8n/packages/nodes-base/credentials/Kafka.credentials.ts (the n8n
// Kafka credential stores the login as `username` / `password`). The
// legacy m9m spellings `saslUsername` / `saslPassword` and the generic
// `user` alias are honoured as fallbacks so credentials stored by an
// older build keep working.
//
// The canonical key always wins when it carries a value: a stale alias
// key left over from an earlier save must never shadow the value the
// operator just edited — otherwise the test authenticates with the old,
// correct login and reports a misleading OK for a wrong username.
func kafkaCredentialUserPass(data map[string]interface{}) (string, string) {
	user := kafkaExactString(data, "username")
	if user == "" {
		user = kafkaExactString(data, "saslUsername")
	}
	if user == "" {
		user = kafkaExactString(data, "user")
	}
	password := kafkaExactString(data, "password")
	if password == "" {
		password = kafkaExactString(data, "saslPassword")
	}
	return user, password
}

// kafkaSASLMechanism normalises the credential's SASL mechanism.
//
// n8n stores the lowercase spelling (`plain`, `scram-sha-256`,
// `scram-sha-512` — see
// n8n/packages/nodes-base/credentials/Kafka.credentials.ts) while the
// m9m schema uses the uppercase one. Both are accepted.
func kafkaSASLMechanism(data map[string]interface{}) string {
	raw := strings.TrimSpace(kafkaExactString(data, "saslMechanism"))
	switch strings.ToLower(raw) {
	case "", "none":
		return ""
	case "plain":
		return "PLAIN"
	case "scram-sha-256", "scram-sha256":
		return "SCRAM-SHA-256"
	case "scram-sha-512", "scram-sha512":
		return "SCRAM-SHA-512"
	}
	return strings.ToUpper(raw)
}

// kafkaSASLMechanisms builds the SASL mechanism list used by the
// connection test.
//
// An explicitly selected mechanism is used on its own: the operator
// asked for PLAIN / SCRAM-SHA-256 / SCRAM-SHA-512 and the test must not
// silently succeed with a different one. Only when no mechanism is
// configured does the test fall back to PLAIN, then SCRAM-SHA-256, then
// SCRAM-SHA-512 — the order a managed broker most likely uses.
func kafkaSASLMechanisms(mechanism, user, password string) ([]sasl.Mechanism, error) {
	switch mechanism {
	case "PLAIN":
		return []sasl.Mechanism{plain.Mechanism{Username: user, Password: password}}, nil
	case "SCRAM-SHA-256":
		m, err := scram.Mechanism(scram.SHA256, user, password)
		if err != nil {
			return nil, err
		}
		return []sasl.Mechanism{m}, nil
	case "SCRAM-SHA-512":
		m, err := scram.Mechanism(scram.SHA512, user, password)
		if err != nil {
			return nil, err
		}
		return []sasl.Mechanism{m}, nil
	case "":
		out := []sasl.Mechanism{plain.Mechanism{Username: user, Password: password}}
		if m, err := scram.Mechanism(scram.SHA256, user, password); err == nil {
			out = append(out, m)
		}
		if m, err := scram.Mechanism(scram.SHA512, user, password); err == nil {
			out = append(out, m)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported SASL mechanism %q (supported: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)", mechanism)
}

// kafkaExactString reads a credential field by its exact key, bypassing
// the generic alias map. `dataString(data, "user")` resolves to the
// `username` alias, so a stale `username` key would shadow the value
// the operator just edited.
func kafkaExactString(data map[string]interface{}, key string) string {
	v, ok := data[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

// scrubKafkaSecrets removes the Kafka username and the password from an
// error message. The password is always scrubbed; the username is
// scrubbed too, but only when it is long enough to be a real
// identifier — scrubbing a one or two character username would mangle
// unrelated words in the message.
func scrubKafkaSecrets(msg, user, password string) string {
	out := scrubSecret(msg, password)
	if len(user) >= 3 {
		out = scrubSecret(out, user)
	}
	return out
}

// kafkaBrokerList parses the comma separated bootstrap broker list.
func kafkaBrokerList(data map[string]interface{}) []string {
	var brokers []string
	for _, b := range strings.Split(dataString(data, "brokers"), ",") {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if !strings.Contains(b, ":") {
			b = net.JoinHostPort(b, "9092")
		}
		brokers = append(brokers, b)
	}
	return brokers
}

// kafkaTLSConfig builds the TLS configuration, adding the credential's
// CA certificate to the system trust store when present.
func kafkaTLSConfig(data map[string]interface{}) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	ca := dataString(data, "ca")
	if ca == "" {
		return cfg, nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM([]byte(normalizePEM(ca))) {
		return nil, fmt.Errorf("Kafka CA certificate could not be parsed")
	}
	cfg.RootCAs = pool
	return cfg, nil
}

// normalizePEM rebuilds a PEM block. Synced credentials often store the
// certificate flattened onto a single line (newlines replaced by
// spaces) or with literal "\n" sequences, which x509 cannot parse.
func normalizePEM(raw string) string {
	s := strings.ReplaceAll(raw, `\n`, "\n")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	begin := strings.Index(s, "-----BEGIN")
	if begin < 0 {
		return s
	}
	labelEnd := begin + len("-----BEGIN")
	dash := strings.Index(s[labelEnd:], "-----")
	if dash < 0 {
		return s
	}
	headerEnd := labelEnd + dash + len("-----")
	header := s[begin:headerEnd]

	end := strings.Index(s[headerEnd:], "-----END")
	if end < 0 {
		return s
	}
	end += headerEnd
	dash = strings.Index(s[end+len("-----END"):], "-----")
	if dash < 0 {
		return s
	}
	footerEnd := end + len("-----END") + dash + len("-----")
	footer := s[end:footerEnd]

	var body strings.Builder
	for _, r := range s[headerEnd:end] {
		switch r {
		case ' ', '\t', '\n', '\r':
			continue
		}
		body.WriteRune(r)
	}
	encoded := body.String()
	if encoded == "" {
		return s
	}

	var out strings.Builder
	out.WriteString(header + "\n")
	for i := 0; i < len(encoded); i += 64 {
		endIdx := i + 64
		if endIdx > len(encoded) {
			endIdx = len(encoded)
		}
		out.WriteString(encoded[i:endIdx] + "\n")
	}
	out.WriteString(footer + "\n")
	return out.String()
}

// kafkaMetadataFunc is the metadata probe used by the Kafka credential
// connection test. It is a package variable so the unit tests can
// substitute a stub instead of talking to a live broker.
//
// The resolved login is passed alongside the mechanism so a stub can
// assert which username / password the SASL handshake is built from —
// the SCRAM mechanisms do not expose the credentials themselves.
var kafkaMetadataFunc = func(brokers []string, tlsCfg *tls.Config, mech sasl.Mechanism, user, password string) (int, error) {
	return kafkaMetadata(brokers, tlsCfg, mech)
}

// kafkaRESTProbe performs the REST Proxy HTTP probe used by the Kafka
// credential connection test. It is a package variable so the unit
// tests can substitute a stub.
var kafkaRESTProbe = func(req *http.Request) (*http.Response, string, error) {
	return doCredentialHTTP(req)
}

// kafkaMetadata performs an authenticated metadata request and returns
// the number of brokers the cluster reported.
func kafkaMetadata(brokers []string, tlsCfg *tls.Config, mech sasl.Mechanism) (int, error) {
	transport := &kafka.Transport{TLS: tlsCfg}
	if mech != nil {
		transport.SASL = mech
	}
	defer transport.CloseIdleConnections()

	client := &kafka.Client{
		Addr:      kafka.TCP(strings.Join(brokers, ",")),
		Transport: transport,
		Timeout:   credentialConnectTimeout,
	}
	ctx, cancel := context.WithTimeout(context.Background(), credentialConnectTimeout)
	defer cancel()

	resp, err := client.Metadata(ctx, &kafka.MetadataRequest{Addr: kafka.TCP(strings.Join(brokers, ","))})
	if err != nil {
		return 0, err
	}
	return len(resp.Brokers), nil
}

// isKafkaAuthError reports whether the broker rejected the credentials.
func isKafkaAuthError(err error) bool {
	if err == nil {
		return false
	}
	var kafkaErr kafka.Error
	if errors.As(err, &kafkaErr) && kafkaErr == kafka.SASLAuthenticationFailed {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sasl") || strings.Contains(msg, "authentication failed")
}

// testRabbitMQConnection performs a full AMQP handshake with the stored
// credentials, so a wrong user or password fails the test.
func testRabbitMQConnection(data map[string]interface{}) (testCredentialResult, bool) {
	host := dataString(data, "host")
	if host == "" {
		return testCredentialResult{}, false
	}
	port := dataInt(data, "port", 5672)
	user := dataString(data, "user")
	password := dataString(data, "password")
	vhost := dataString(data, "vhost")
	if vhost == "" {
		vhost = "/"
	}

	u := &url.URL{
		Scheme: "amqp",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	if user != "" {
		u.User = url.UserPassword(user, password)
	}

	cfg := amqp.Config{
		Vhost:    vhost,
		Locale:   "en_US",
		Heartbeat: 0,
		Dial:     amqp.DefaultDial(5 * time.Second),
	}
	if user != "" {
		cfg.SASL = []amqp.Authentication{&amqp.PlainAuth{Username: user, Password: password}}
	}

	conn, err := amqp.DialConfig(u.String(), cfg)
	if err != nil {
		msg := scrubSecret(err.Error(), password)
		if isAMQPAuthError(err) {
			return testCredentialResult{
				Status:  "Error",
				Message: "RabbitMQ authentication failed: " + msg,
			}, true
		}
		return testCredentialResult{
			Status:  "Error",
			Message: "RabbitMQ connection failed: " + msg,
		}, true
	}
	_ = conn.Close()

	message := fmt.Sprintf("Connected to RabbitMQ %s:%d (vhost %s", host, port, vhost)
	if user != "" {
		message += fmt.Sprintf(", authenticated as %s", user)
	}
	return testCredentialResult{
		Status:  "OK",
		Message: message + ")",
	}, true
}

// isAMQPAuthError reports whether RabbitMQ refused the credentials.
func isAMQPAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "access_refused") ||
		strings.Contains(msg, "access refused") ||
		strings.Contains(msg, "authentication") ||
		strings.Contains(msg, "login was refused") ||
		strings.Contains(msg, "not allowed")
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
