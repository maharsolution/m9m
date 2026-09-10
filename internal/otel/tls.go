package otel

import (
	"crypto/tls"

	"google.golang.org/grpc/credentials"
)

// grpcTLSCreds wraps a *tls.Config in the gRPC TransportCredentials
// interface expected by otlptracegrpc.WithTLSCredentials. Kept in its own
// file so adding other transport-security helpers (e.g. a future
// InsecureSkipVerify escape hatch) doesn't bloat exporter.go.
func grpcTLSCreds(cfg *tls.Config) credentials.TransportCredentials {
	return credentials.NewTLS(cfg)
}
