package webhooks

// Tests for BUG-ARCH-2026-10-12-007 — streaming response
// support.

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestStreamResponse_WriteFlushes(t *testing.T) {
	rec := newFlusherRecorder()
	stream, err := NewStreamResponse(rec)
	if err != nil {
		t.Fatalf("NewStreamResponse: %v", err)
	}
	stream.WriteString("hello ")
	stream.WriteString("world")
	if string(rec.written) != "hello world" {
		t.Errorf("written = %q, want 'hello world'", rec.written)
	}
	if rec.flushed != 2 {
		t.Errorf("flushed = %d, want 2", rec.flushed)
	}
}

func TestStreamResponse_SetsProxyHeaders(t *testing.T) {
	rec := newFlusherRecorder()
	if _, err := NewStreamResponse(rec); err != nil {
		t.Fatalf("NewStreamResponse: %v", err)
	}
	if got := rec.header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", got)
	}
	if got := rec.header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}

func TestStreamResponse_SSE(t *testing.T) {
	rec := newFlusherRecorder()
	stream, _ := NewStreamResponse(rec)
	if err := stream.WriteSSE("chunk", `{"i":1}`); err != nil {
		t.Fatalf("WriteSSE: %v", err)
	}
	want := "event: chunk\ndata: {\"i\":1}\n\n"
	if string(rec.written) != want {
		t.Errorf("SSE wire = %q, want %q", rec.written, want)
	}
}

func TestStreamResponse_SSE_NoEvent(t *testing.T) {
	rec := newFlusherRecorder()
	stream, _ := NewStreamResponse(rec)
	if err := stream.WriteSSE("", "raw data"); err != nil {
		t.Fatalf("WriteSSE: %v", err)
	}
	if !strings.HasPrefix(string(rec.written), "data: ") {
		t.Errorf("wire = %q, want 'data: ' prefix", rec.written)
	}
	if strings.Contains(string(rec.written), "event:") {
		t.Errorf("wire contains 'event:' line: %q", rec.written)
	}
}

func TestStreamResponse_SSE_MultilineData(t *testing.T) {
	rec := newFlusherRecorder()
	stream, _ := NewStreamResponse(rec)
	if err := stream.WriteSSE("x", "line1\nline2"); err != nil {
		t.Fatalf("WriteSSE: %v", err)
	}
	want := "event: x\ndata: line1\ndata: line2\n\n"
	if string(rec.written) != want {
		t.Errorf("multiline SSE = %q, want %q", rec.written, want)
	}
}

func TestStreamResponse_UnsupportedWriter(t *testing.T) {
	// http.ResponseWriter that does NOT implement
	// http.Flusher.
	w := nonFlusherWriter{}
	if _, err := NewStreamResponse(w); err == nil {
		t.Fatal("expected error on non-Flusher ResponseWriter")
	}
}

func TestStreamResponse_NilWriter(t *testing.T) {
	if _, err := NewStreamResponse(nil); err == nil {
		t.Fatal("expected error on nil ResponseWriter")
	}
}

func TestStreamResponse_Hijack(t *testing.T) {
	// http.ResponseWriter that DOES implement
	// http.Hijacker (e.g. the real http server conn).
	rec := &hijackableRecorder{flusherRecorder: newFlusherRecorder()}
	stream, _ := NewStreamResponse(rec)
	conn, brw, err := stream.Hijack()
	if err != nil {
		t.Fatalf("Hijack: %v", err)
	}
	if conn == nil || brw == nil {
		t.Errorf("Hijack returned nil conn or brw")
	}
}

func TestStreamResponse_Hijack_Unsupported(t *testing.T) {
	stream, _ := NewStreamResponse(newFlusherRecorder())
	if _, _, err := stream.Hijack(); err == nil {
		t.Fatal("expected error on non-Hijacker writer")
	}
}

// nonFlusherWriter implements http.ResponseWriter but NOT
// http.Flusher.
type nonFlusherWriter struct{}

func (nonFlusherWriter) Header() http.Header        { return http.Header{} }
func (nonFlusherWriter) Write(b []byte) (int, error) { return len(b), nil }
func (nonFlusherWriter) WriteHeader(int)            {}

var _ http.ResponseWriter = nonFlusherWriter{}

// hijackableRecorder implements http.Hijacker on top of
// flusherRecorder.
type hijackableRecorder struct {
	*flusherRecorder
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}
