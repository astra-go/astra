package astra

// context_response.go — HTTP response-writing methods for Ctx.
//
// Structured helpers (JSON, XML, String, HTML, Render) set Content-Type,
// Content-Length, and status code in one call. Raw helpers (Blob, NoContent,
// Redirect, File) serve special cases. Streaming / push helpers (SSEvent,
// EarlyHints, JSONStream) are for long-lived or large responses.
//
// JSON vs JSONStream:
//   - JSON encodes into a pooled buffer first → sets Content-Length, no chunked
//     encoding on HTTP/1.1. Use for responses under ~64 KB.
//   - JSONStream writes directly to the wire → no Content-Length, use for bulk
//     or paginated payloads that may exceed the pool threshold.

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// jsonBufPool is a pool of *bytes.Buffer used for zero-extra-allocation JSON
// encoding. Encoding into a buffer first lets us set Content-Length before
// writing the header, which avoids chunked encoding on HTTP/1.1.
var jsonBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// jsonBufMaxCap is the cap threshold above which a buffer is not returned to
// jsonBufPool. Oversized buffers (expanded by large payloads) are dropped so
// their backing arrays can be GC'd, preventing memory bloat in mixed-traffic
// scenarios. Responses larger than this should use JSONStream instead.
const jsonBufMaxCap = 64 * 1024 // 64 KB

// Pre-allocated Content-Type header slices.
// http.Header is map[string][]string; using direct map assignment (h["Key"] = slice)
// instead of h.Set("Key", value) avoids the []string{value} allocation that
// Header.Set creates on every call.  The slices are read-only after init.
var (
	ctJSON  = []string{"application/json; charset=utf-8"}
	ctXML   = []string{"application/xml; charset=utf-8"}
	ctPlain = []string{"text/plain; charset=utf-8"}
	ctHTML  = []string{"text/html; charset=utf-8"}
)

// clCacheSize is the upper bound (exclusive) for the pre-built Content-Length
// header cache.  Responses up to 1023 bytes (covering virtually all JSON API
// responses) will use a pre-allocated []string with no per-request allocation.
const clCacheSize = 1024

// clStrings[i] holds strconv.Itoa(i) for i in [0, clCacheSize).
// clHeaders[i] is a fixed [1]string backing array whose sole element is clStrings[i].
// clSlices[i] is a []string slice header pointing into clHeaders[i][:].
// All three arrays are read-only after package init.
var (
	clStrings [clCacheSize]string
	clHeaders [clCacheSize][1]string
	clSlices  [clCacheSize][]string
)

func init() {
	for i := range clStrings {
		clStrings[i] = strconv.Itoa(i)
		clHeaders[i][0] = clStrings[i]
		clSlices[i] = clHeaders[i][:]
	}
}

// contentLengthSlice returns a pre-built single-element []string for n if
// n < clCacheSize (zero allocs), otherwise allocates a fresh one.
func contentLengthSlice(n int) []string {
	if n < clCacheSize {
		return clSlices[n]
	}
	return []string{strconv.Itoa(n)}
}

// JSON writes a JSON response with the given status code.
//
// Performance: the object is encoded into a pooled buffer first so that the
// Content-Length header can be set before WriteHeader, avoiding chunked
// transfer encoding on HTTP/1.1 and saving one syscall per response.
//
// If the configured Serializer also implements bufEncoder (the default
// goJsonSerializer does), encoding goes directly into the pooled buffer with
// 0 allocs.  Otherwise the fallback Marshal path incurs 1 alloc for the
// intermediate []byte.
func (c *Ctx) JSON(code int, obj any) error {
	c.debugCheckConcurrency()
	buf := jsonBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= jsonBufMaxCap {
			jsonBufPool.Put(buf)
		}
	}()

	ser := c.app.options.serializer()
	if be, ok := ser.(bufEncoder); ok {
		// Fast path (Plan D): encode directly into the pooled buffer.
		// goJsonSerializer pools its Encoder, so this path is 0 allocs after warm-up.
		if err := be.EncodeInto(buf, obj); err != nil {
			return err
		}
	} else {
		// Fallback: serializer doesn't support direct-write; use Marshal.
		b, err := ser.Marshal(obj)
		if err != nil {
			return err
		}
		buf.Write(b)
	}

	h := c.writer.Header()
	h["Content-Type"] = ctJSON
	h["Content-Length"] = contentLengthSlice(buf.Len())
	c.writer.WriteHeader(code)
	_, err := buf.WriteTo(c.writer)
	return err
}

// XML writes an XML response.
func (c *Ctx) XML(code int, obj any) error {
	c.debugCheckConcurrency()
	h := c.writer.Header()
	h["Content-Type"] = ctXML
	c.writer.WriteHeader(code)
	_, err := c.writer.Write([]byte(xml.Header))
	if err != nil {
		return err
	}
	return xml.NewEncoder(c.writer).Encode(obj)
}

// String writes a plain text response.
// When no format values are supplied the string is written via io.WriteString,
// which delegates to responseWriter.WriteString and avoids the []byte(format)
// heap allocation that Write([]byte(s)) would incur.
func (c *Ctx) String(code int, format string, values ...any) error {
	c.debugCheckConcurrency()
	c.writer.Header()["Content-Type"] = ctPlain
	c.writer.WriteHeader(code)
	if len(values) > 0 {
		_, err := fmt.Fprintf(c.writer, format, values...)
		return err
	}
	_, err := io.WriteString(c.writer, format)
	return err
}

// HTML writes an HTML response.
func (c *Ctx) HTML(code int, html string) error {
	c.debugCheckConcurrency()
	c.writer.Header()["Content-Type"] = ctHTML
	c.writer.WriteHeader(code)
	_, err := io.WriteString(c.writer, html)
	return err
}

// Render renders the named template using the configured Renderer engine and
// writes the result as an HTML response with the given status code.
//
// The Renderer must be registered via astra.WithRenderer before calling Render.
// Use the render sub-package's HTMLEngine for file-based templates:
//
//	return c.Render(200, "pages/index.html", astra.Map{"Title": "Home"})
func (c *Ctx) Render(code int, name string, data any) error {
	c.debugCheckConcurrency()
	r := c.app.options.Renderer
	if r == nil {
		return fmt.Errorf("astra: no renderer registered — use astra.WithRenderer to set one")
	}
	c.writer.Header()["Content-Type"] = ctHTML
	c.writer.WriteHeader(code)
	return r.Render(c.writer, name, data)
}

// JSONStream writes a JSON response by encoding obj directly into the
// ResponseWriter, bypassing the pooled intermediate buffer used by JSON.
//
// Use JSONStream for responses that may exceed jsonBufMaxCap (64 KB), such as
// bulk/list endpoints. For smaller objects, prefer JSON — it sets Content-Length,
// which avoids chunked transfer encoding on HTTP/1.1 and improves proxy and
// keep-alive behaviour.
//
// Trade-off: Content-Length is not set, so HTTP/1.1 connections use chunked
// transfer encoding. HTTP/2 and HTTP/3 are unaffected.
func (c *Ctx) JSONStream(code int, obj any) error {
	c.debugCheckConcurrency()
	h := c.writer.Header()
	h["Content-Type"] = ctJSON
	c.writer.WriteHeader(code)
	ser := c.app.options.serializer()
	if se, ok := ser.(streamEncoder); ok {
		return se.EncodeStream(c.writer, obj)
	}
	// bufEncoder fast path: encode into pooled buffer to avoid the temporary
	// []byte alloc that ser.Marshal(obj) would produce, then write in one shot.
	if be, ok := ser.(bufEncoder); ok {
		buf := jsonBufPool.Get().(*bytes.Buffer)
		buf.Reset()
		err := be.EncodeInto(buf, obj)
		if err != nil {
			if buf.Cap() <= jsonBufMaxCap {
				jsonBufPool.Put(buf)
			}
			return err
		}
		_, err = buf.WriteTo(c.writer)
		if buf.Cap() <= jsonBufMaxCap {
			jsonBufPool.Put(buf)
		}
		return err
	}
	b, err := ser.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = c.writer.Write(b)
	return err
}

// Blob writes raw bytes with the given content type.
func (c *Ctx) Blob(code int, contentType string, data []byte) error {
	c.debugCheckConcurrency()
	c.writer.Header().Set("Content-Type", contentType)
	c.writer.WriteHeader(code)
	_, err := c.writer.Write(data)
	return err
}

// NoContent writes a response with no body.
func (c *Ctx) NoContent(code int) error {
	c.debugCheckConcurrency()
	c.writer.WriteHeader(code)
	return nil
}

// Redirect sends an HTTP redirect response.
func (c *Ctx) Redirect(code int, location string) error {
	c.debugCheckConcurrency()
	if code < 300 || code > 308 {
		return NewHTTPError(http.StatusInternalServerError, "invalid redirect code")
	}
	http.Redirect(c.writer, c.req, location, code)
	return nil
}

// File serves the named file.
func (c *Ctx) File(filepath string) error {
	c.debugCheckConcurrency()
	http.ServeFile(c.writer, c.req, filepath)
	return nil
}

// SSEvent writes a Server-Sent Event to the response.
func (c *Ctx) SSEvent(event, data string) error {
	c.debugCheckConcurrency()
	h := c.writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	if event != "" {
		if _, err := fmt.Fprintf(c.writer, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.writer, "data: %s\n\n", data); err != nil {
		return err
	}
	if f, ok := c.writer.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// Push initiates an HTTP/2 server push for the given target path.
// Returns http.ErrNotSupported when the underlying connection does not
// support push (HTTP/1.1, or push disabled by the client).
//
// Deprecated: HTTP/2 Server Push is no longer supported by major browsers.
// Use EarlyHints instead, which achieves equivalent preload behaviour via
// the standard 103 Early Hints response (RFC 9110).
func (c *Ctx) Push(target string, opts *http.PushOptions) error {
	if p, ok := c.writer.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// EarlyHints sends a 103 Early Hints interim response that instructs the
// client (or an intermediary CDN) to preload the given resource paths before
// the final response is ready.  Each target is emitted as a Link header with
// rel=preload.  The optional opts map allows per-resource attributes such as
// "as" or "crossorigin" (e.g. opts["as"] = "style").
//
// EarlyHints is a no-op if headers have already been written.
// It works on all transport paths (HTTP/1.1 Reactor, HTTP/2, HTTP/3).
func (c *Ctx) EarlyHints(targets []string, opts map[string]string) error {
	c.debugCheckConcurrency()
	if c.writer.Written() {
		return nil
	}
	var sb strings.Builder
	for _, t := range targets {
		sb.Reset()
		sb.WriteString("<")
		sb.WriteString(t)
		sb.WriteString(">; rel=preload")
		for k, v := range opts {
			sb.WriteString("; ")
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(v)
		}
		c.writer.Header().Add("Link", sb.String())
	}
	c.writer.WriteHeader(http.StatusEarlyHints)
	return nil
}

// Flush flushes buffered data to the client if the underlying ResponseWriter
// implements http.Flusher. Returns nil if flushing is not supported.
func (c *Ctx) Flush() error {
	if f, ok := c.writer.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// Stream writes a streaming response by reading from r and writing directly to the wire.
//
// Unlike JSON or Blob, Stream does NOT buffer the entire payload in memory.
// The data is copied from r to the ResponseWriter using io.Copy, making it
// suitable for large files, S3 objects, or any io.Reader source.
//
// Content-Length is NOT set (chunked transfer encoding on HTTP/1.1).
// To set Content-Length, pass an *os.File or an io.ReadSeeker and call
// c.writer.Header()["Content-Length"] = ... before calling Stream.
//
// The copy loop respects the request context: if c.req.Context() is cancelled
// (client disconnect, timeout), the copy stops early.
func (c *Ctx) Stream(code int, contentType string, r io.Reader) error {
	c.debugCheckConcurrency()
	h := c.writer.Header()
	if contentType != "" {
		h["Content-Type"] = []string{contentType}
	}
	c.writer.WriteHeader(code)

	// Use a context-aware copy if the request context supports cancellation.
	w := c.writer
	if cc, ok := w.(writeFlusher); ok {
		// Best-effort: flush after each write to keep streaming responsive.
		defer cc.Flush()
	}

	// Stop early if the client disconnects.
	ctx := c.req.Context()
	done := ctx.Done()
	if done == nil {
		_, err := io.Copy(w, r)
		return err
	}

	// Wrapped reader that checks context cancellation.
	n := &contextReader{r: r, ctx: ctx}
	_, err := io.Copy(w, n)
	return err
}

// writeFlusher is a minimal interface combining Write and Flush,
// satisfied by *bufio.Writer, http.ResponseWriter (when Flusher), etc.
type writeFlusher interface {
	io.Writer
	http.Flusher
}

// contextReader wraps an io.Reader with request-context cancellation.
type contextReader struct {
	r   io.Reader
	ctx context.Context
}

func (cr *contextReader) Read(p []byte) (int, error) {
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
	}
	return cr.r.Read(p)
}
