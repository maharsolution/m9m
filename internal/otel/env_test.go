package otel

import (
	"testing"
)

// TestLoadConfigFromEnv_OTelSDKStandard verifies that an operator can
// configure m9m using the bare OpenTelemetry SDK env vars (the same
// names documented in the OTel spec) without needing the M9M_ prefix.
func TestLoadConfigFromEnv_OTelSDKStandard(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel-collector.example.com:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_SERVICE_NAME", "my-m9m-instance")
	t.Setenv("OTEL_SERVICE_VERSION", "1.2.3")
	t.Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")

	cfg := LoadConfigFromEnv()

	if cfg.Endpoint != "http://otel-collector.example.com:4318" {
		t.Fatalf("Endpoint=%q want %q", cfg.Endpoint, "http://otel-collector.example.com:4318")
	}
	if cfg.Protocol != "http/protobuf" {
		t.Fatalf("Protocol=%q want %q", cfg.Protocol, "http/protobuf")
	}
	if cfg.ServiceName != "my-m9m-instance" {
		t.Fatalf("ServiceName=%q want %q", cfg.ServiceName, "my-m9m-instance")
	}
	if cfg.ServiceVersion != "1.2.3" {
		t.Fatalf("ServiceVersion=%q want %q", cfg.ServiceVersion, "1.2.3")
	}
	if cfg.SampleRate != 0.25 {
		t.Fatalf("SampleRate=%v want 0.25", cfg.SampleRate)
	}
}

// TestLoadConfigFromEnv_M9MPrefixWins verifies that an operator can
// override the standard OTel SDK env var with the M9M_-prefixed one
// in the same shell (e.g. to point a single host at a different
// collector without editing the deploy-wide OTEL config).
func TestLoadConfigFromEnv_M9MPrefixWins(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://default-collector:4318")
	t.Setenv("M9M_OTEL_EXPORTER_OTLP_ENDPOINT", "http://prod-collector:4317")
	t.Setenv("OTEL_SERVICE_NAME", "m9m")
	t.Setenv("M9M_OTEL_SERVICE_NAME", "m9m-prod")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")
	t.Setenv("M9M_OTEL_TRACES_SAMPLE_RATE", "0.9")

	cfg := LoadConfigFromEnv()

	if cfg.Endpoint != "http://prod-collector:4317" {
		t.Fatalf("Endpoint=%q want M9M_-prefixed value to win", cfg.Endpoint)
	}
	if cfg.ServiceName != "m9m-prod" {
		t.Fatalf("ServiceName=%q want M9M_-prefixed value to win", cfg.ServiceName)
	}
	if cfg.SampleRate != 0.9 {
		t.Fatalf("SampleRate=%v want M9M_-prefixed value 0.9 to win over OTel-SDK 0.1", cfg.SampleRate)
	}
}

// TestLoadConfigFromEnv_AlwaysOnAlwaysOff verifies the OTel-SDK
// "always_on" / "always_off" sampler shortcuts translate to the
// correct ratio.
func TestLoadConfigFromEnv_AlwaysOnAlwaysOff(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")
	cfg := LoadConfigFromEnv()
	if cfg.SampleRate != 0 {
		t.Fatalf("SampleRate=%v want 0 for always_off", cfg.SampleRate)
	}

	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.42") // ignored in always_on mode
	cfg = LoadConfigFromEnv()
	if cfg.SampleRate != 1 {
		t.Fatalf("SampleRate=%v want 1 for always_on", cfg.SampleRate)
	}
}

// TestLoadConfigFromEnv_NoVarsSet returns a Config with Enabled=false
// (not Enabled=true) — the absence of env vars must never silently
// turn tracing on.
func TestLoadConfigFromEnv_NoVarsSet(t *testing.T) {
	// Unset all known vars so this test is hermetic even if the
	// developer shell has OTEL_* already exported.
	names := []string{
		"M9M_OTEL_ENABLED",
		"M9M_OTEL_EXPORTER_OTLP_ENDPOINT",
		"M9M_OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_SERVICE_NAME",
		"OTEL_SERVICE_VERSION",
		"OTEL_TRACES_SAMPLER",
		"OTEL_TRACES_SAMPLER_ARG",
	}
	for _, n := range names {
		t.Setenv(n, "")
	}
	cfg := LoadConfigFromEnv()
	if cfg.Enabled {
		t.Fatalf("Enabled=true with no env vars set — must default to false")
	}
}
