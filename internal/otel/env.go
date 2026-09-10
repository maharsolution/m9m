package otel

import (
	"os"
	"strconv"
	"strings"
)

// LoadConfigFromEnv parses the M9M_OTEL_* and M9M_AGENTS_TRACING_* env
// vars into a Config. The function never returns an error: unparseable
// numbers fall back to defaults, unknown protocols fall back to
// http/protobuf. A Config with Enabled=false is returned when
// M9M_OTEL_ENABLED is not "true" so the rest of the codebase can treat
// tracing as off without nil-checking Config.
//
// The variable names are aligned with n8n's N8N_OTEL_* / N8N_AGENTS_TRACING_*
// names to keep muscle memory portable, but the M9M_ prefix avoids
// collisions on hosts that co-deploy n8n and m9m.
func LoadConfigFromEnv() Config {
	cfg := Config{
		Enabled:            envBool("M9M_OTEL_ENABLED", false),
		Endpoint:           strings.TrimSpace(os.Getenv("M9M_OTEL_EXPORTER_OTLP_ENDPOINT")),
		Protocol:           strings.ToLower(strings.TrimSpace(os.Getenv("M9M_OTEL_EXPORTER_OTLP_PROTOCOL"))),
		Headers:            strings.TrimSpace(os.Getenv("M9M_OTEL_EXPORTER_OTLP_HEADERS")),
		HeadersFile:        strings.TrimSpace(os.Getenv("M9M_OTEL_EXPORTER_OTLP_HEADERS_FILE")),
		SampleRate:         envFloat("M9M_OTEL_TRACES_SAMPLE_RATE", 1.0),
		ProductionOnly:     envBool("M9M_OTEL_TRACES_PRODUCTION_ONLY", true),
		IncludeNodeSpans:   envBool("M9M_OTEL_TRACES_INCLUDE_NODE_SPANS", true),
		InjectOutbound:     envBool("M9M_OTEL_TRACES_INJECT_OUTBOUND", true),
		ServiceName:        strings.TrimSpace(os.Getenv("M9M_OTEL_SERVICE_NAME")),
		ServiceVersion:     strings.TrimSpace(os.Getenv("M9M_OTEL_SERVICE_VERSION")),
		InstanceID:         strings.TrimSpace(os.Getenv("M9M_OTEL_INSTANCE_ID")),
		AgentsEnabled:      envBool("M9M_AGENTS_TRACING_ENABLED", false),
		AgentsRecordInputs: envBool("M9M_AGENTS_TRACING_RECORD_INPUTS", true),
		AgentsRecordOutputs: envBool("M9M_AGENTS_TRACING_RECORD_OUTPUTS", true),
	}
	cfg.ApplyDefaults()
	return cfg
}

// envBool parses an env var as a bool. Empty / unparseable values fall
// back to def. Accepted truthy values: "1", "t", "true", "yes", "y"
// (case-insensitive). Anything else is false.
func envBool(name string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "t", "true", "yes", "y", "on":
		return true
	case "0", "f", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

// envFloat parses an env var as a float64. Empty / unparseable values
// fall back to def.
func envFloat(name string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// generateInstanceID returns a stable per-process identifier. We don't
// need cryptographic strength here — just enough that a SigNoz query for
// m9m.instance.id surfaces the right host. The host:pid shape matches
// what most agents produce (e.g. the otel-collector itself) so backend
// operators see a familiar pattern.
func generateInstanceID() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return host + ":" + strconv.Itoa(os.Getpid())
}
