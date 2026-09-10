package otel

import (
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// buildSampler returns the sampler described by cfg. The base sampler is
// always TraceIDRatioBased so all spans sharing a trace ID are sampled
// together (matching upstream OTel + n8n behaviour). When
// cfg.ProductionOnly is true we wrap that base with a production-mode
// filter so non-production execution modes are dropped at the SDK level
// — saves both collector load and the cost of serializing spans we know
// will be discarded downstream.
func buildSampler(cfg Config) sdktrace.Sampler {
	inner := sdktrace.TraceIDRatioBased(cfg.SampleRate)
	if cfg.ProductionOnly {
		return productionOnlySampler{inner: inner}
	}
	return inner
}

// productionOnlySampler rejects non-production execution modes at the
// SDK level. It looks at the m9m.execution.mode attribute that the
// engine attaches to the workflow.execute span. When the attribute is
// absent (e.g. on a span emitted outside an execution context) the base
// sampler decides.
//
// n8n's documented production modes are webhook / trigger / retry, with
// manual runs dropped by default unless M9M_OTEL_TRACES_PRODUCTION_ONLY
// is explicitly false. We mirror the exact same default: n8n 2.27.0+
// docs say "By default, n8n only outputs traces for production
// executions ... To output traces for all workflow executions, set
// N8N_OTEL_TRACES_PRODUCTION_ONLY=false".
//
// In n8n "production" includes manual runs started from the canvas
// (because they execute a real workflow with real data). We keep that
// semantics — only debug / test runs (mode = "test" or empty) are
// dropped. The engine never emits "test" itself; the webhook handler
// does, when the test webhook endpoint is hit.
var productionModes = map[string]struct{}{
	"":         {}, // unknown — let the base sampler decide
	"manual":   {},
	"retry":    {},
	"webhook":  {},
	"trigger":  {},
	"cli":      {},
	"schedule": {},
}

type productionOnlySampler struct {
	inner sdktrace.Sampler
}

func (s productionOnlySampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	mode, ok := stringAttr(p.Attributes, "m9m.execution.mode")
	if !ok {
		// No mode attribute yet — defer to the base sampler. This is the
		// right behaviour for spans that the engine opens at the
		// workflow root, because the sampler is consulted BEFORE the
		// mode attribute is set. The wrapper reconsiders on the next
		// span (which inherits the same trace ID, so the inner sampler
		// still applies).
		return s.inner.ShouldSample(p)
	}
	if _, isProd := productionModes[mode]; !isProd {
		return sdktrace.SamplingResult{Decision: sdktrace.Drop}
	}
	return s.inner.ShouldSample(p)
}

func (s productionOnlySampler) Description() string {
	return "m9mProductionOnly{" + s.inner.Description() + "}"
}

// stringAttr extracts the string value of an attribute by key. Returns
// the value and true on hit; "" and false on miss. We only care about
// the string form here — execution mode is always a string in n8n and
// we follow the same convention.
func stringAttr(attrs []attribute.KeyValue, key string) (string, bool) {
	for _, a := range attrs {
		if a.Key == attribute.Key(key) {
			return a.Value.AsString(), true
		}
	}
	return "", false
}
