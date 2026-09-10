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
//
// We also accept the standard OpenTelemetry SDK env vars
// (OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_TRACES_SAMPLER, etc.) so operators
// who already wire their backend via the OTel-SDK standard don't have
// to copy the values across. The M9M_-prefixed variable (when set)
// wins over the bare OTel-SDK name so an operator can override a
// global OTEL config in the same shell.
func LoadConfigFromEnv() Config {
	cfg := Config{
		Enabled:            envBool("M9M_OTEL_ENABLED", false),
		Endpoint:           firstNonEmpty("M9M_OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_ENDPOINT"),
		Protocol:           strings.ToLower(firstNonEmpty("M9M_OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_PROTOCOL")),
		Headers:            firstNonEmpty("M9M_OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_HEADERS"),
		HeadersFile:        firstNonEmpty("M9M_OTEL_EXPORTER_OTLP_HEADERS_FILE", "OTEL_EXPORTER_OTLP_HEADERS_FILE"),
		SampleRate:         firstFloat([]string{"M9M_OTEL_TRACES_SAMPLE_RATE", "OTEL_TRACES_SAMPLER_ARG"}, 1.0),
		ProductionOnly:     firstBool([]string{"M9M_OTEL_TRACES_PRODUCTION_ONLY"}, true),
		IncludeNodeSpans:   firstBool([]string{"M9M_OTEL_TRACES_INCLUDE_NODE_SPANS"}, true),
		InjectOutbound:     firstBool([]string{"M9M_OTEL_TRACES_INJECT_OUTBOUND"}, true),
		ServiceName:        firstNonEmpty("M9M_OTEL_SERVICE_NAME", "OTEL_SERVICE_NAME"),
		ServiceVersion:     firstNonEmpty("M9M_OTEL_SERVICE_VERSION", "OTEL_SERVICE_VERSION"),
		InstanceID:         strings.TrimSpace(os.Getenv("M9M_OTEL_INSTANCE_ID")),
		AgentsEnabled:      firstBool([]string{"M9M_AGENTS_TRACING_ENABLED"}, false),
		AgentsRecordInputs: firstBool([]string{"M9M_AGENTS_TRACING_RECORD_INPUTS"}, true),
		AgentsRecordOutputs: firstBool([]string{"M9M_AGENTS_TRACING_RECORD_OUTPUTS"}, true),
	}
	cfg.ApplyDefaults()

	// Standard OTel SDK shape: OTEL_TRACES_SAMPLER=traceidratio +
	// OTEL_TRACES_SAMPLER_ARG=0.1 — convert the *_SAMPLER pair into our
	// single SampleRate field when the operator didn't set the m9m var.
	if sampler := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER"))); sampler != "" {
		switch sampler {
		case "always_on":
			cfg.SampleRate = 1.0
		case "always_off":
			cfg.SampleRate = 0.0
		case "traceidratio":
			// cfg.SampleRate already loaded from OTEL_TRACES_SAMPLER_ARG
			// in the call above (or default 1.0). Nothing to do.
		}
	}

	return cfg
}

// firstNonEmpty returns the trimmed value of the first env var in the
// list that is non-empty. Used to layer an m9m-specific override on
// top of the standard OTel SDK name.
func firstNonEmpty(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// firstFloat returns the parsed float of the first non-empty env var
// in the list, falling back to def when none parse.
func firstFloat(names []string, def float64) float64 {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	return def
}

// firstBool returns the parsed bool of the first non-empty env var in
// the list, falling back to def when none parse.
func firstBool(names []string, def bool) bool {
	for _, name := range names {
		if v := strings.ToLower(strings.TrimSpace(os.Getenv(name))); v != "" {
			switch v {
			case "1", "t", "true", "yes", "y", "on":
				return true
			case "0", "f", "false", "no", "n", "off":
				return false
			}
		}
	}
	return def
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
