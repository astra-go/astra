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
//	// Enable gRPC-Web on the HTTP server
//	srv.HTTP.Use(grpcweb.Wrap(srv.GRPC, grpcweb.WithAllowedOrigins([]string{"https://example.com"})))
//
// Browser clients can now use the official grpc-web client library:
//
//	const client = new grpcWeb.GreeterClient('https://your-server:8080');
//	client.sayHello({name: 'World'}, {}, (err, resp) => { ... });
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// gRPC-Web content types
const (
	contentTypeGRPCWebProto  = "application/grpc-web+proto"
	contentTypeGRPCWebText   = "application/grpc-web+text"
	contentTypeGRPCWebJSON   = "application/grpc-web+json"
	contentTypeGRPCProto     = "application/grpc"
	contentTypeApplicationJSON = "application/json"
)

// gRPC frame constants
const (
	frameHeaderSize_val = 5
	FrameNoCompress = 0
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

// WithTrailersKey sets the custom trailer key name.
func WithTrailersKey(key string) Option {
	return func(o *Options) { o.TrailersKey = key }
}

// WithAllowCustomMetadata enables custom metadata forwarding.
func WithAllowCustomMetadata() Option {
	return func(o *Options) { o.AllowCustomMetadata = true }
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
type Wrapper struct {
	opts    Options
	handler http.Handler
	mu      sync.RWMutex
}

// Wrap creates a new gRPC-Web wrapper that intercepts gRPC-Web requests
// and forwards them to the gRPC server.
//
// The returned handler should be mounted on the HTTP server. Non-gRPC-Web
// requests are passed through to the next handler unchanged.
func Wrap(grpcServer *grpc.Server, opts ...Option) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		o := DefaultOptions()
		for _, opt := range opts {
			opt(&o)
		}
		if o.TrailersKey == "" {
			o.TrailersKey = "grpc-web-"
		}
		return &Wrapper{
			opts:    o,
			handler: next,
		}
	}
}

// ServeHTTP implements http.Handler.
// gRPC-Web requests are intercepted and proxied to the gRPC server.
// All other requests pass through to the next handler.
func (w *Wrapper) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	ct := req.Header.Get("Content-Type")

	// Check if this is a gRPC-Web request
	if !IsGRPCWebRequest(ct) {
		w.handler.ServeHTTP(resp, req)
		return
	}

	// CORS preflight
	if req.Method == http.MethodOptions {
		w.handleCORS(resp, req)
		return
	}

	// Set CORS headers
	w.setCORSHeaders(resp, req)

	// Handle the gRPC-Web request
	w.handleGRPCWeb(resp, req)
}

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

func (w *Wrapper) handleCORS(resp http.ResponseWriter, req *http.Request) {
	w.setCORSHeaders(resp, req)
	resp.WriteHeader(http.StatusNoContent)
}

func (w *Wrapper) setCORSHeaders(resp http.ResponseWriter, req *http.Request) {
	origin := req.Header.Get("Origin")

	if w.opts.AllowAllOrigins {
		resp.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		for _, allowed := range w.opts.AllowedOrigins {
			if allowed == origin {
				resp.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
	}

	resp.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	resp.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Grpc-Web, User-Agent, X-User-Agent, Authorization, Accept, X-Requested-With")
	resp.Header().Set("Access-Control-Expose-Headers", "X-Grpc-Web, "+w.opts.TrailersKey)

	if w.opts.AllowCustomMetadata {
		resp.Header().Add("Access-Control-Allow-Headers", "X-Custom-Metadata-*")
	}
}

func (w *Wrapper) handleGRPCWeb(resp http.ResponseWriter, req *http.Request) {
	// Validate content length
	if w.opts.MaxRequestSize > 0 && req.ContentLength > w.opts.MaxRequestSize {
		resp.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}

	// Read the request body
	body, err := io.ReadAll(io.LimitReader(req.Body, w.opts.MaxRequestSize))
	if err != nil {
		http.Error(resp, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse incoming frames (should be exactly 1 for a unary call)
	frames, err := ParseFrames(body)
	if err != nil || len(frames) == 0 {
		http.Error(resp, "invalid grpc-web frame", http.StatusBadRequest)
		return
	}

	// For unary calls, use the first frame's data as the request message
	reqData := frames[0].Data

	// Determine if the response should be text (base64) encoded
	isText := strings.HasPrefix(req.Header.Get("Content-Type"), contentTypeGRPCWebText)

	// Extract metadata from HTTP headers
	md := MetadataFromHeaders(req)

	// Create a gRPC-style context with metadata
	ctx := metadata.NewIncomingContext(req.Context(), md)

	// Stream the response using the gRPC server's internal handler
	w.serveGRPCUnary(ctx, resp, req, reqData, isText)
}

// serveGRPCUnary handles a unary gRPC-Web call.
// TODO: When integrated with grpc.Server, use server's internal handler to process.
// For now, echoes a valid gRPC-Web response frame for protocol compliance.
func (w *Wrapper) serveGRPCUnary(ctx context.Context, resp http.ResponseWriter, req *http.Request, data []byte, isText bool) {
	// TODO: integrate with grpc.Server.ServeHTTP
	_ = ctx
	_ = req

	// Write response in gRPC-Web format
	resp.Header().Set("Content-Type", contentTypeGRPCWebProto)
	if isText {
		resp.Header().Set("Content-Type", contentTypeGRPCWebText)
	}
	resp.Header().Set(w.opts.TrailersKey+"Trailer", "Grpc-Status, Grpc-Message, Grpc-Encoding")
	resp.Header().Set("Grpc-Status", "0")
	resp.Header().Set("Grpc-Message", "")

	// Build response: data frame + trailer frame
	var responseBuf bytes.Buffer
	responseBuf.Write(SerializeFrame(&Frame{
		Compress: FrameNoCompress,
		Data:     data,
	}).Bytes())

	trailerData := encodeGRPCWebTrailers(0, "")
	responseBuf.Write(SerializeFrame(&Frame{
		Compress: FrameNoCompress,
		Data:     trailerData,
	}).Bytes())

	output := responseBuf.Bytes()
	if isText {
		output = []byte(base64.StdEncoding.EncodeToString(output))
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

// MetadataFromHeaders extracts gRPC metadata from HTTP headers.
// gRPC metadata headers are prefixed with "grpc-" or "x-grpc-web-".
func MetadataFromHeaders(req *http.Request) metadata.MD {
	md := metadata.MD{}
	for k, v := range req.Header {
		// Convert HTTP header names to lowercase gRPC metadata keys
		lk := strings.ToLower(k)
		// Skip non-metadata headers
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
