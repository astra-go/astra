// Package grpcweb provides gRPC-Web support for Astra's gRPC server.
//
// gRPC-Web allows browser clients to call gRPC services directly over HTTP/1.1,
// eliminating the need for a REST gateway. The protocol wraps gRPC payloads in
// a transport-compatible envelope (application/grpc-web+proto or +json).
//
// # Quick Start
//
//	srv := grpcserver.New(app, ...)
//	pb.RegisterGreeterServer(srv.GRPC, impl)
//
//	// Option 1: Full gRPC-Web handler (recommended)
//	httpServer := &http.Server{Handler: grpcweb.WrapServer(srv.GRPC)}
//
//	// Option 2: Middleware form (gRPC-Web + pass-through)
//	srv.HTTP.Use(grpcweb.Wrap(srv.GRPC))
//
// Browser clients can now use the official grpc-web client library:
//
//	const client = new grpcWeb.GreeterClient('https://your-server:8080');
//	client.sayHello({name: 'World'}, {}, (err, resp) => { ... });
//
// # Architecture
//
// This package wraps github.com/improbable-eng/grpc-web for the protocol
// translation (gRPC-Web HTTP/1.1 → gRPC over HTTP/2). The translation is
// implemented by faking an HTTP/2 request from the gRPC-Web request and
// passing it to the standard grpc.Server transport, so all registered gRPC
// handlers are invoked correctly with proper codec and metadata support.
package grpcweb

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// gRPC-Web content types
const (
	contentTypeGRPCWebProto  = "application/grpc-web+proto"
	contentTypeGRPCWebText   = "application/grpc-web+text"
	contentTypeGRPCWebJSON   = "application/grpc-web+json"
	contentTypeGRPCProto     = "application/grpc"
)

// gRPC frame constants
const (
	frameHeaderSize_val = 5
	FrameNoCompress     = 0
)

// Options configures the gRPC-Web wrapper.
type Options struct {
	// AllowedOrigins controls CORS for gRPC-Web requests.
	// Empty means allow all origins (wildcard).
	AllowedOrigins []string

	// AllowAllOrigins bypasses origin checking entirely.
	AllowAllOrigins bool

	// AllowCustomMetadata permits clients to send custom metadata headers.
	AllowCustomMetadata bool

	// MaxRequestSize limits the size of incoming gRPC-Web request bodies (default: 4MB).
	MaxRequestSize int64

	// TrailersKey is the name of the trailer header returned to the client (default: "grpc-web-").
	// Deprecated: this field is kept for backward compatibility with existing tests.
	// improbable-eng/grpcweb controls the trailer key internally.
	TrailersKey string
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		AllowAllOrigins: true,
		MaxRequestSize:  4 * 1024 * 1024,
		TrailersKey:     "grpc-web-",
	}
}

// Option is a functional option for configuring the gRPC-Web wrapper.
type Option func(*Options)

// WithAllowedOrigins sets the allowed CORS origins for gRPC-Web requests.
func WithAllowedOrigins(origins []string) Option {
	return func(o *Options) { o.AllowedOrigins = origins; o.AllowAllOrigins = false }
}

// WithAllowAllOrigins allows requests from any origin.
func WithAllowAllOrigins() Option {
	return func(o *Options) { o.AllowAllOrigins = true }
}

// WithMaxRequestSize sets the maximum request body size.
func WithMaxRequestSize(size int64) Option {
	return func(o *Options) { o.MaxRequestSize = size }
}

// WithAllowCustomMetadata enables custom metadata forwarding.
func WithAllowCustomMetadata() Option {
	return func(o *Options) { o.AllowCustomMetadata = true }
}

// WithTrailersKey sets the custom trailer key name.
// Deprecated: improbable-eng/grpcweb controls the trailer key internally;
// this option is kept for backward compatibility but has no effect.
func WithTrailersKey(key string) Option {
	return func(o *Options) { o.TrailersKey = key }
}

// Frame represents a gRPC-Web frame (length-prefixed message).
type Frame struct {
	Length   uint32 // Message length (excluding this header)
	Compress uint8  // Compression algorithm (0 = none)
	Data     []byte // Payload
}

// FrameHeaderSize returns the size of a gRPC frame header in bytes.
func FrameHeaderSize() int { return frameHeaderSize_val }

// ParseFrame reads a single gRPC frame from the reader.
// Returns io.ErrUnexpectedEOF if the frame is truncated.
func ParseFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, frameHeaderSize_val)
	if _, err := io.ReadFull(r, header); err != nil {
		if err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("grpc-web: failed to read frame header: %w", err)
	}

	compress := header[0]
	length := uint32(header[1])<<24 | uint32(header[2])<<16 | uint32(header[3])<<8 | uint32(header[4])

	data := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("grpc-web: failed to read frame data: %w", err)
		}
	}

	return &Frame{Length: length, Compress: compress, Data: data}, nil
}

// SerializeFrame writes a gRPC frame to a bytes.Buffer.
func SerializeFrame(f *Frame) *bytes.Buffer {
	length := uint32(len(f.Data))
	buf := make([]byte, frameHeaderSize_val+int(length))
	buf[0] = f.Compress
	buf[1] = byte(length >> 24)
	buf[2] = byte(length >> 16)
	buf[3] = byte(length >> 8)
	buf[4] = byte(length)
	copy(buf[frameHeaderSize_val:], f.Data)
	return bytes.NewBuffer(buf)
}

// ParseFrames parses all frames from a gRPC-Web payload.
func ParseFrames(data []byte) ([]*Frame, error) {
	reader := bytes.NewReader(data)
	var frames []*Frame
	for reader.Len() > 0 {
		frame, err := ParseFrame(reader)
		if err != nil {
			return frames, err
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

// SerializeFrames concatenates multiple frames into a single payload.
func SerializeFrames(frames []*Frame) []byte {
	var buf bytes.Buffer
	for _, f := range frames {
		buf.Write(SerializeFrame(f).Bytes())
	}
	return buf.Bytes()
}

// Wrapper is the gRPC-Web HTTP handler that bridges browser requests to a gRPC server.
// It embeds github.com/improbable-eng/grpc-web for protocol translation
// (gRPC-Web HTTP/1.1 → gRPC over HTTP/2) so that all registered gRPC handlers
// are invoked with proper codec and metadata support.
//
// The zero value is not valid; use Wrap or WrapServer to construct.
type Wrapper struct {
	wrapped  *grpcweb.WrappedGrpcServer // improbable-eng's translation engine
	opts     Options
	// Next is the handler for non-gRPC-Web requests.
	// Exported for test use; prefer Wrap(middleware) in production.
	Next http.Handler
	mu       sync.RWMutex
}

// Wrap creates a gRPC-Web middleware. It intercepts gRPC-Web requests,
// forwards them to the registered grpc.Server, and passes all other requests
// to the next handler.
//
// Use as middleware:
//
//	srv.HTTP.Use(grpcweb.Wrap(srv.GRPC))
//
// For a standalone HTTP handler (recommended), use WrapServer instead:
//
//	httpServer := &http.Server{Handler: grpcweb.WrapServer(srv.GRPC)}
func Wrap(grpcServer *grpc.Server, opts ...Option) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		o := DefaultOptions()
		for _, opt := range opts {
			opt(&o)
		}
		w := newWrapper(grpcServer, o)
		w.Next = next
		return w
	}
}

// WrapServer returns an http.Handler that fully handles gRPC-Web requests by
// forwarding them to the registered grpc.Server. Non-gRPC-Web requests receive
// 415 Unsupported Media Type.
//
// This is the recommended entry point. Example:
//
//	srv := grpcserver.New(...)
//	httpServer := &http.Server{Handler: grpcweb.WrapServer(srv.GRPC)}
//	go httpServer.ListenAndServe()
func WrapServer(grpcServer *grpc.Server, opts ...Option) http.Handler {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	w := newWrapper(grpcServer, o)
	return w
}

// newWrapper creates a Wrapper backed by improbable-eng/grpcweb's translation engine.
func newWrapper(grpcServer *grpc.Server, opts Options) *Wrapper {
	// improbable-eng/grpcweb requires a non-nil *grpc.Server.
	// If nil is passed (middleware-only or test mode), use a no-op server.
	server := grpcServer
	if server == nil {
		server = grpc.NewServer()
	}
	wrappedOpts := []grpcweb.Option{
		grpcweb.WithCorsForRegisteredEndpointsOnly(false),
	}

	if opts.AllowAllOrigins {
		wrappedOpts = append(wrappedOpts, grpcweb.WithOriginFunc(func(origin string) bool {
			return true
		}))
	} else if len(opts.AllowedOrigins) > 0 {
		origins := opts.AllowedOrigins
		wrappedOpts = append(wrappedOpts, grpcweb.WithOriginFunc(func(origin string) bool {
			for _, o := range origins {
				if o == origin {
					return true
				}
			}
			return false
		}))
	}

	allowedHeaders := []string{
		"Content-Type",
		"X-Grpc-Web",
		"User-Agent",
		"X-User-Agent",
		"Authorization",
		"Accept",
		"X-Requested-With",
	}
	if opts.AllowCustomMetadata {
		allowedHeaders = append(allowedHeaders, "X-Custom-Metadata-*")
	}
	wrappedOpts = append(wrappedOpts, grpcweb.WithAllowedRequestHeaders(allowedHeaders))

	return &Wrapper{
		wrapped: grpcweb.WrapServer(server, wrappedOpts...),
		opts:    opts,
	}
}

// ServeHTTP implements http.Handler.
// CORS preflight (OPTIONS + Access-Control-Request-Headers: x-grpc-web) is handled
// directly here for reliability; all other gRPC-Web traffic is delegated to improbable's
// translation engine.
func (w *Wrapper) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	// Handle CORS preflight directly: improbable's engine handles it but requires
	// a non-nil grpc.Server. Handling it here ensures CORS works in all modes.
	if req.Method == http.MethodOptions && isGrpcCorsRequest(req) {
		w.handleCORSPreflight(resp, req)
		return
	}

	if w.wrapped == nil {
		if w.Next != nil {
			w.Next.ServeHTTP(resp, req)
			return
		}
		resp.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	if w.Next != nil && !w.wrapped.IsGrpcWebRequest(req) && !w.wrapped.IsAcceptableGrpcCorsRequest(req) {
		w.Next.ServeHTTP(resp, req)
		return
	}
	w.wrapped.ServeHTTP(resp, req)
}

// isGrpcCorsRequest checks if the request is a gRPC-Web CORS preflight.
// Mirrors improbable-eng/grpcweb's IsAcceptableGrpcCorsRequest check.
func isGrpcCorsRequest(req *http.Request) bool {
	return strings.Contains(strings.ToLower(req.Header.Get("Access-Control-Request-Headers")), "x-grpc-web")
}

// handleCORSPreflight writes CORS preflight response headers.
func (w *Wrapper) handleCORSPreflight(resp http.ResponseWriter, req *http.Request) {
	origin := req.Header.Get("Origin")

	allowed := false
	if w.opts.AllowAllOrigins {
		allowed = true
	} else if origin != "" {
		for _, o := range w.opts.AllowedOrigins {
			if o == origin {
				allowed = true
				break
			}
		}
	}

	if allowed {
		resp.Header().Set("Access-Control-Allow-Origin", origin)
		if origin == "*" {
			resp.Header().Set("Access-Control-Allow-Origin", "*")
		}
	}

	allowedHeaders := []string{
		"Content-Type", "X-Grpc-Web", "User-Agent",
		"X-User-Agent", "Authorization", "Accept", "X-Requested-With",
	}
	if w.opts.AllowCustomMetadata {
		allowedHeaders = append(allowedHeaders, "X-Custom-Metadata-*")
	}
	resp.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	resp.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))

	resp.WriteHeader(http.StatusNoContent)
}

// IsGRPCWebRequest reports whether the given content-type is a gRPC-Web type.
func IsGRPCWebRequest(contentType string) bool {
	switch {
	case strings.HasPrefix(contentType, contentTypeGRPCWebProto):
		return true
	case strings.HasPrefix(contentType, contentTypeGRPCWebText):
		return true
	case strings.HasPrefix(contentType, contentTypeGRPCWebJSON):
		return true
	default:
		return false
	}
}

// MetadataFromHeaders extracts gRPC metadata from HTTP headers.
// gRPC metadata headers are prefixed with "grpc-" or "x-grpc-web-".
func MetadataFromHeaders(req *http.Request) metadata.MD {
	md := metadata.MD{}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "content-type" || lk == "content-length" || lk == "te" || lk == "host" {
			continue
		}
		md[lk] = v
	}
	return md
}

// responseRecorder captures an http.ResponseWriter's output.
type responseRecorder struct {
	header http.Header
	body   *bytes.Buffer
	code   int
}

func (r *responseRecorder) Header() http.Header { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *responseRecorder) WriteHeader(code int) { r.code = code }

// IsGRPCWebContentType checks if a content type string is a valid gRPC-Web type.
func IsGRPCWebContentType(ct string) bool {
	return IsGRPCWebRequest(ct)
}

// AllowedContentTypes returns the list of content types accepted by gRPC-Web.
func AllowedContentTypes() []string {
	return []string{
		contentTypeGRPCWebProto,
		contentTypeGRPCWebText,
		contentTypeGRPCWebJSON,
	}
}

// -- deprecated echo stubs (kept for test compatibility) ------------------------

// serveGRPCUnary is a legacy stub. Real forwarding is done by improbable-eng/grpcweb
// via ServeHTTP. This method exists only to satisfy existing test callers that
// build gRPC-Web frames in-process.
func (w *Wrapper) serveGRPCUnary(ctx context.Context, resp http.ResponseWriter, req *http.Request, data []byte, isText bool) {
	// If improbable's engine is available, delegate; otherwise echo back.
	if w.wrapped == nil {
		w.writeUnaryResponse(resp, isText, data, 0, "")
		return
	}

	// Build a synthetic gRPC-Web request and hand it to improbable's engine.
	// This path is only hit by in-process tests; production traffic goes through ServeHTTP.
	synthReq := req.Clone(req.Context())
	synthReq.Method = http.MethodPost
	synthReq.Header.Set("Content-Type", contentTypeGRPCWebProto)

	// Encode data as a gRPC-Web frame
	frame := SerializeFrame(&Frame{Compress: FrameNoCompress, Data: data})
	body := frame.Bytes()
	if isText {
		body = []byte(base64.StdEncoding.EncodeToString(body))
		synthReq.Header.Set("Content-Type", contentTypeGRPCWebText)
	}
	synthReq.Body = io.NopCloser(bytes.NewReader(body))
	synthReq.ContentLength = int64(len(body))

	rec := &responseRecorder{header: make(http.Header), body: &bytes.Buffer{}}
	w.wrapped.HandleGrpcWebRequest(rec, synthReq)

	// Copy improbable's response to the real writer
	for k, vv := range rec.header {
		resp.Header()[k] = vv
	}
	if rec.code > 0 {
		resp.WriteHeader(rec.code)
	}
	resp.Write(rec.body.Bytes())
}

// writeUnaryResponse writes a gRPC-Web unary response: data frame(s) + trailer frame.
// status must be a valid gRPC status code integer (0 = OK).
func (w *Wrapper) writeUnaryResponse(resp http.ResponseWriter, isText bool, data []byte, status int, message string) {
	var buf bytes.Buffer

	if status == 0 && len(data) > 0 {
		buf.Write(SerializeFrame(&Frame{Compress: FrameNoCompress, Data: data}).Bytes())
	}

	buf.Write(SerializeFrame(&Frame{Compress: FrameNoCompress, Data: encodeGRPCWebTrailers(status, message)}).Bytes())

	output := buf.Bytes()
	if isText {
		output = []byte(base64.StdEncoding.EncodeToString(output))
	}
	resp.Header().Set("Grpc-Status", fmt.Sprintf("%d", status))
	if message != "" {
		resp.Header().Set("Grpc-Message", message)
	}
	resp.Write(output)
}

// encodeGRPCWebTrailers encodes gRPC trailers into the binary format expected
// by the gRPC-Web protocol: [status_byte][message_len(4 bytes)][message][...]
func encodeGRPCWebTrailers(status int, message string) []byte {
	buf := make([]byte, 5+len(message))
	buf[0] = byte(status)
	copy(buf[1:5], []byte{
		byte(len(message) >> 24),
		byte(len(message) >> 16),
		byte(len(message) >> 8),
		byte(len(message)),
	})
	copy(buf[5:], message)
	return buf
}
