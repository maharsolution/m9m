package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// buildResource composes the OTel resource that gets attached to every
// emitted span. The shape mirrors what n8n exports:
//
//   - service.name = "m9m"
//   - service.version = build version
//   - m9m.instance.id = <host>:<pid>
//   - m9m.instance.role = main | webhook
//
// Plus the standard semconv attributes from the v1.26.0 schema (the
// version pinned by the SDK go.opentelemetry.io/otel@v1.38.0). The
// resource is merged with the SDK's defaults so platform-specific
// detectors (e.g. process.runtime.name, host.name) stay in place.
func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		attribute.String("m9m.instance.id", cfg.InstanceID),
		attribute.String("m9m.instance.role", RoleMain),
	}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	custom, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}
	merged, err := resource.Merge(resource.Default(), custom)
	if err != nil {
		return nil, fmt.Errorf("otel: merge resource: %w", err)
	}
	return merged, nil
}
