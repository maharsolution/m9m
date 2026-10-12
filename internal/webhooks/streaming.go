package webhooks

// Streaming response support for BUG-ARCH-2026-10-12-007.
//
// m9m's webhook `sendResponse` writes the body in one shot
// (json.Marshal → w.Write). For long-running workflows that
// produce data incrementally (LLM token streams, log
// tailing, real-time aggregations) the caller wants to
// flush partial output to the client as it's produced.
//
// This file adds `StreamResponse`, a thin wrapper that:
//   1. Sets the response status code + headers
//      (callers do this once).
//   2. Writes chunks via `Write`, calling `Flush` after
//      each one so the client sees progress without
//      waiting for the full body.
//   3. Disables the proxy buffering heuristics that
//      otherwise hold the response in memory on common
//      front-ends (nginx `proxy_buffering off` is a
//      documented requirement; this wrapper just sets
//      `X-Accel-Buffering: no` to ask the proxy to
//      cooperate).
//
// The `StreamResponse` is intentionally a small
// abstraction; the heavy lifting is `http.Flusher`,
// which is part of the standard library.

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

// StreamResponse is the streaming-friendly response
// writer. It wraps an `http.ResponseWriter` and exposes
// `Write` + `Flush`. Construct with `NewStreamResponse`
// once you've called `w.Header().Set(...)` and
// `w.WriteHeader(...)`.
type StreamResponse struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewStreamResponse wraps `w`. The caller MUST have
// called `w.WriteHeader` before constructing the
// stream (otherwise the status defaults to 200 once
// the first byte is written).
//
// Errors:
//   - `ErrStreamingUnsupported` if `w` doesn't expose
//     `http.Flusher` (e.g. the test recorder).
func NewStreamResponse(w http.ResponseWriter) (*StreamResponse, error) {
	if w == nil {
		return nil, errors.New("webhooks: nil ResponseWriter")
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, ErrStreamingUnsupported
	}
	// Ask well-known reverse proxies to not buffer.
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "no-cache")
	return &StreamResponse{w: w, flusher: flusher}, nil
}

// ErrStreamingUnsupported is returned by NewStreamResponse
// when the wrapped ResponseWriter doesn't implement
// http.Flusher.
var ErrStreamingUnsupported = errors.New("webhooks: ResponseWriter does not support streaming")

// Write writes `chunk` to the underlying ResponseWriter
// and flushes so the client sees the bytes immediately.
// Each chunk MUST be a complete protocol unit (one JSON
// object, one SSE event, etc.) — `Write` does not
// chunk-boundary itself.
func (s *StreamResponse) Write(chunk []byte) (int, error) {
	n, err := s.w.Write(chunk)
	s.flusher.Flush()
	return n, err
}

// WriteString is the string-flavoured Write. Same flush
// behaviour.
func (s *StreamResponse) WriteString(s2 string) (int, error) {
	n, err := s.w.Write([]byte(s2))
	s.flusher.Flush()
	return n, err
}

// WriteSSE writes one Server-Sent Event. The `data` field
// is the literal text the client receives after the
// `data: ` prefix. `event` is optional; pass "" to omit
// the `event:` line.
//
// SSE format per the WHATWG spec:
//   event: <event>\n
//   data: <data>\n
//   \n
func (s *StreamResponse) WriteSSE(event, data string) error {
	if event != "" {
		if _, err := s.WriteString("event: " + event + "\n"); err != nil {
			return err
		}
	}
	// SSE requires newlines inside `data` to be split
	// across multiple `data:` lines.
	for _, line := range splitLines(data) {
		if _, err := s.WriteString("data: " + line + "\n"); err != nil {
			return err
		}
	}
	// The blank line terminator.
	_, err := s.WriteString("\n")
	return err
}

// Hijack exposes the underlying connection for low-level
// protocols (WebSockets, raw TCP). Most streaming use
// cases don't need this.
func (s *StreamResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := s.w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("webhooks: ResponseWriter does not support Hijacker")
	}
	return hj.Hijack()
}

func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// flusherRecorder is a tiny `http.ResponseWriter` that
// implements `http.Flusher` and records every flush, for
// the test. It is intentionally minimal.
type flusherRecorder struct {
	header  http.Header
	written []byte
	flushed int
	status  int
}

func newFlusherRecorder() *flusherRecorder {
	return &flusherRecorder{header: http.Header{}, status: 200}
}
func (f *flusherRecorder) Header() http.Header        { return f.header }
func (f *flusherRecorder) Write(b []byte) (int, error) { f.written = append(f.written, b...); return len(b), nil }
func (f *flusherRecorder) WriteHeader(s int)           { f.status = s }
func (f *flusherRecorder) Flush()                     { f.flushed++ }
var _ http.ResponseWriter = (*flusherRecorder)(nil)
var _ http.Flusher = (*flusherRecorder)(nil)

func exampleStreamingHandler(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	stream, err := NewStreamResponse(w)
	if err != nil {
		return err
	}
	stream.WriteString("event: start\n")
	stream.WriteSSE("chunk", "{\"i\":1}")
	stream.WriteSSE("chunk", "{\"i\":2}")
	return stream.WriteSSE("end", "done")
}

