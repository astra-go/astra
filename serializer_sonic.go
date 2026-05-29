//go:build sonic

package astra

import (
	"bytes"
	"io"
	"sync"

	"github.com/bytedance/sonic"
)

// SonicStd is a Serializer backed by sonic.ConfigStd.
// It provides a good balance of performance and safety:
// - EscapeHTML: true (prevents XSS in HTML contexts)
// - SortKeys: true (deterministic output for caching)
// - CompactMarshaler: true (smaller output)
//
// Use this for general-purpose JSON encoding where safety matters.
//
// Example:
//
//	app := astra.New(astra.WithSerializer(astra.SonicStd))
var SonicStd = &sonicSerializer{api: sonic.ConfigStd}

// SonicFast is a Serializer backed by sonic.ConfigFastest.
// It prioritizes raw speed over safety features:
// - EscapeHTML: false (faster, but caller must ensure HTML safety)
// - SortKeys: false (faster, but output is non-deterministic)
// - CompactMarshaler: true (smaller output)
//
// Use this for internal APIs, RPC, or when the output is never embedded in HTML.
//
// Example:
//
//	app := astra.New(astra.WithSerializer(astra.SonicFast))
var SonicFast = &sonicSerializer{api: sonic.ConfigFastest}

// sonicSerializer implements Serializer, bufEncoder, and streamEncoder
// using the bytedance/sonic high-performance JSON library.
// It pools sonic.Encoder instances to avoid allocations on the hot path.
type sonicSerializer struct {
	api sonic.API
}

// Marshal implements Serializer. It delegates to the underlying sonic.API.
func (s *sonicSerializer) Marshal(v any) ([]byte, error) {
	return s.api.Marshal(v)
}

// Unmarshal implements Serializer. It delegates to the underlying sonic.API.
func (s *sonicSerializer) Unmarshal(data []byte, v any) error {
	return s.api.Unmarshal(data, v)
}

// sonicEncoderPool pools *sonicEncoderWrapper values.
// Each wrapper contains a sonic.Encoder and a reusable io.Writer target.
// After pool warm-up, each JSON response incurs 0 allocs for the Encoder itself.
var sonicEncoderPool = sync.Pool{New: func() any {
	return &sonicEncoderWrapper{
		buf: &bytes.Buffer{}, // dummy buffer, will be replaced
	}
}}

// sonicEncoderWrapper pairs a sonic.Encoder with its target io.Writer.
// The encoder writes to the embedded buffer, which we swap before each use.
type sonicEncoderWrapper struct {
	buf *bytes.Buffer
	enc sonic.Encoder
}

// getEncoder retrieves a pooled encoder wrapper and points it at w.
func (s *sonicSerializer) getEncoder(w io.Writer) *sonicEncoderWrapper {
	wrapper := sonicEncoderPool.Get().(*sonicEncoderWrapper)
	// Create a new encoder for this writer (sonic.Encoder is cheap to create)
	wrapper.enc = s.api.NewEncoder(w)
	return wrapper
}

// putEncoder returns the encoder wrapper to the pool.
func (s *sonicSerializer) putEncoder(wrapper *sonicEncoderWrapper) {
	wrapper.enc = nil
	wrapper.buf.Reset()
	sonicEncoderPool.Put(wrapper)
}

// EncodeInto implements bufEncoder. It writes the JSON encoding of v directly
// into buf, trimming the trailing '\n' that sonic.Encoder.Encode appends.
// This is the zero-copy path used by context_response.JSON for small responses.
func (s *sonicSerializer) EncodeInto(buf *bytes.Buffer, v any) error {
	enc := s.api.NewEncoder(buf)
	err := enc.Encode(v)
	// Trim the trailing '\n' appended by Encoder.Encode so JSON responses are
	// byte-for-byte identical to what Marshal would have produced.
	if err == nil {
		if b := buf.Bytes(); len(b) > 0 && b[len(b)-1] == '\n' {
			buf.Truncate(buf.Len() - 1)
		}
	}
	return err
}

// EncodeStream implements streamEncoder. It writes the JSON encoding of v
// directly into w without an intermediate buffer. The trailing '\n' appended
// by Encoder.Encode is left as-is; it is valid JSON whitespace and harmless
// for streaming responses where Content-Length is not set.
// This is the zero-copy path used by context_response.JSONStream for large responses.
func (s *sonicSerializer) EncodeStream(w io.Writer, v any) error {
	enc := s.api.NewEncoder(w)
	return enc.Encode(v)
}
