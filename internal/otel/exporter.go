package otel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// buildExporter constructs the OTLP exporter matching cfg.Protocol. The
// exporter type selection mirrors the upstream OTel Go default: http/protobuf
// first, gRPC when the operator explicitly asks for it.
//
// The endpoint string is parsed leniently:
//   - scheme-less inputs ("localhost:4317") default to insecure http on
//     the gRPC transport and plain http on the http transport. This matches
//     the upstream behaviour of otlptracehttp / otlptracegrpc.
//   - https:// triggers TLS. We pick up any of the standard
//     OTEL_EXPORTER_OTLP_CERTIFICATE / *_CLIENT_KEY / *_CLIENT_CERTIFICATE
//     env vars by passing a *tls.Config into the upstream constructors.
//
// mTLS / custom CA env vars are honoured by passing them through to the
// upstream constructors rather than reinventing the wheel. See
// https://opentelemetry.io/docs/specs/otel/protocol/exporter/ for the
// variable list.
func buildExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	switch cfg.Protocol {
	case "grpc":
		return buildGRPCExporter(ctx, cfg)
	case "http/protobuf", "http":
		return buildHTTPExporter(ctx, cfg)
	default:
		return nil, fmt.Errorf("otel: unknown OTLP protocol %q (expected http/protobuf or grpc)", cfg.Protocol)
	}
}

func buildHTTPExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(stripScheme(cfg.Endpoint))}

	if isInsecure(cfg.Endpoint) {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	if cfg.Headers != "" {
		opts = append(opts, otlptracehttp.WithHeaders(parseHeaders(cfg.Headers)))
	}

	tlsCfg, err := loadMTLSConfig()
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		opts = append(opts, otlptracehttp.WithTLSClientConfig(tlsCfg))
	}

	client := otlptracehttp.NewClient(opts...)
	return otlptrace.New(ctx, client)
}

func buildGRPCExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(stripScheme(cfg.Endpoint))}

	if isInsecure(cfg.Endpoint) {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if cfg.Headers != "" {
		opts = append(opts, otlptracegrpc.WithHeaders(parseHeaders(cfg.Headers)))
	}

	tlsCfg, err := loadMTLSConfig()
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		// The upstream constructor signature is WithTLSCredentials
		// (credentials.TransportCredentials), not WithTLSClientConfig.
		// Build credentials from the *tls.Config using NewTLS.
		opts = append(opts, otlptracegrpc.WithTLSCredentials(grpcTLSCreds(tlsCfg)))
	}

	client := otlptracegrpc.NewClient(opts...)
	return otlptrace.New(ctx, client)
}

// stripScheme removes the http:// or https:// scheme from a URL. The
// upstream constructors want the host:port form, not a full URL.
func stripScheme(endpoint string) string {
	for _, p := range []string{"http://", "https://", "grpc://", "grpcs://"} {
		if strings.HasPrefix(endpoint, p) {
			return strings.TrimPrefix(endpoint, p)
		}
	}
	return endpoint
}

// isInsecure reports whether the endpoint URL opted out of TLS. We treat
// grpc:// the same as http:// for parity with upstream OTel Go.
func isInsecure(endpoint string) bool {
	return strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "grpc://")
}

// parseHeaders turns "k1=v1,k2=v2" into a map. Whitespace around keys and
// values is trimmed; empty entries are skipped. Values may contain "="
// themselves; only the first "=" splits.
func parseHeaders(raw string) map[string]string {
	out := make(map[string]string, 4)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		val := strings.TrimSpace(part[eq+1:])
		if key == "" {
			continue
		}
		out[key] = val
	}
	return out
}

// loadMTLSConfig loads the optional OTEL_EXPORTER_OTLP_CERTIFICATE /
// *_CLIENT_KEY / *_CLIENT_CERTIFICATE env vars into a *tls.Config. Returns
// (nil, nil) when none of the env vars are set so the caller can skip TLS
// config entirely.
func loadMTLSConfig() (*tls.Config, error) {
	caPath := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_CERTIFICATE"))
	keyPath := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_CLIENT_KEY"))
	certPath := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE"))

	if caPath == "" && keyPath == "" && certPath == "" {
		return nil, nil
	}

	cfg := &tls.Config{}

	if caPath != "" {
		pem, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("otel: read CA bundle %q: %w", caPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("otel: no certificates found in %q", caPath)
		}
		cfg.RootCAs = pool
	}

	if keyPath != "" && certPath != "" {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("otel: load client key pair: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	} else if keyPath != "" || certPath != "" {
		return nil, errors.New("otel: OTEL_EXPORTER_OTLP_CLIENT_KEY and OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE must be set together")
	}

	return cfg, nil
}
