package database

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/neul-labs/m9m/internal/model"
)

// parityC11SSLRequestMagic is the PostgreSQL SSLRequest message header
// (length 8, code 80877103). lib/pq sends it before the StartupMessage
// when sslmode requires TLS.
var parityC11SSLRequestMagic = []byte{0x00, 0x00, 0x00, 0x08, 0x04, 0xD2, 0x16, 0x2F}

// parityC11StartupMagic is the PostgreSQL protocol 3.0 StartupMessage
// header. lib/pq sends it directly when TLS is disabled.
var parityC11StartupMagic = []byte{0x00, 0x03, 0x00, 0x00}

// parityC11FakePostgres starts a TCP listener that behaves like a
// PostgreSQL server for the first client message only. The channel
// receives the first 8 bytes the client sent.
func parityC11FakePostgres(t *testing.T) (string, int, <-chan []byte) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot start fake postgres listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("cannot parse listener address: %v", err)
	}
	port := 0
	for _, digit := range portText {
		port = port*10 + int(digit-'0')
	}

	first := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			first <- nil
			return
		}
		defer conn.Close()

		header := make([]byte, 8)
		n, _ := io.ReadFull(conn, header)
		first <- header[:n]
		// Give the client a moment to fail on the incomplete handshake.
		time.Sleep(100 * time.Millisecond)
	}()

	return host, port, first
}

// TestParityC11PostgresSSLModeDefaultDisable is the regression lock for
// GAP-2026-09-10-2: the PostgreSQL node must default to
// sslmode=disable when the workflow does not set sslMode, matching n8n.
//
// The check is behavioural, not a source-code inspection: the node is
// pointed at a fake PostgreSQL listener and the first bytes the driver
// sends are inspected. A TLS handshake starts with SSLRequest, a plain
// connection starts with the StartupMessage.
func TestParityC11PostgresSSLModeDefaultDisable(t *testing.T) {
	host, port, first := parityC11FakePostgres(t)

	node := NewPostgresNode()
	params := map[string]interface{}{
		"host":      host,
		"port":      port,
		"database":  "parity",
		"user":      "parity",
		"password":  "parity",
		"operation": "executeQuery",
		"query":     "SELECT 1",
	}

	_, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{"id": 1}}}, params)
	if err == nil {
		t.Fatal("expected the fake postgres handshake to fail")
	}

	select {
	case header := <-first:
		if len(header) < 8 {
			t.Fatalf("fake postgres received %d bytes, want 8: %v", len(header), header)
		}
		if bytes.Equal(header, parityC11SSLRequestMagic) {
			t.Fatalf("default sslMode requested TLS (SSLRequest sent): %v", header)
		}
		if !bytes.Equal(header[4:8], parityC11StartupMagic[0:4]) {
			t.Fatalf("unexpected first postgres message: %v", header)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fake postgres never received a startup message")
	}
}

// TestParityC11PostgresSSLModeExplicitRequire locks the other half of
// the contract: an explicit sslMode=require must make the driver start
// a TLS handshake.
func TestParityC11PostgresSSLModeExplicitRequire(t *testing.T) {
	host, port, first := parityC11FakePostgres(t)

	node := NewPostgresNode()
	params := map[string]interface{}{
		"host":      host,
		"port":      port,
		"database":  "parity",
		"user":      "parity",
		"password":  "parity",
		"operation": "executeQuery",
		"query":     "SELECT 1",
		"sslMode":   "require",
	}

	_, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{"id": 1}}}, params)
	if err == nil {
		t.Fatal("expected the fake postgres handshake to fail")
	}

	select {
	case header := <-first:
		if len(header) < 8 {
			t.Fatalf("fake postgres received %d bytes, want 8: %v", len(header), header)
		}
		if !bytes.Equal(header, parityC11SSLRequestMagic) {
			t.Fatalf("sslMode=require did not start a TLS handshake: %v", header)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fake postgres never received an SSLRequest")
	}
}
