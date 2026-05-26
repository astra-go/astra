// Package middleware — Gzip compression middleware.
//
// Compresses response bodies using gzip when the client sends
// "Accept-Encoding: gzip". Small responses below MinSize are not
// compressed (the overhead of gzip for tiny payloads is negative).
//
// Usage:
//
//	app.Use(middleware.Compress())
//
//	// Custom configuration:
//	app.Use(middleware.CompressWithConfig(middleware.CompressConfig{
//	    Level:   gzip.BestSpeed,
//	    MinSize: 512,
//	    Skipper: func(c *contract.Context) bool {
//	        return strings.HasPrefix(c.Request.URL.Path, "/stream/")
//	    },
//	}))
package middleware

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/contract"
)

// CompressConfig configures the Compress middleware.
type CompressConfig struct {
	// Level is the gzip compression level.
	// Valid values: gzip.DefaultCompression (-1), gzip.BestSpeed (1) … gzip.BestCompression (9).
	// Default: gzip.DefaultCompression
	Level int

	// MinSize is the minimum response body size (bytes) before compression is applied.
	// Responses smaller than MinSize are sent as-is.
	// Default: 1024
	MinSize int

	// Skipper defines a function to skip middleware for certain requests.
	// Returning true skips compression.
	// Default: skip text/event-stream responses (SSE)
	Skipper func(*astra.Ctx) bool

	// ExcludedExtensions lists file extensions to skip (e.g. ".png", ".jpg").
	// Default: common already-compressed formats
	ExcludedExtensions []string
}

// DefaultCompressConfig is the default configuration.
var DefaultCompressConfig = CompressConfig{
	Level:   gzip.DefaultCompression,
	MinSize: 1024,
	ExcludedExtensions: []string{
		".png", ".jpg", ".jpeg", ".gif", ".webp",
		".mp4", ".webm", ".mp3", ".ogg",
		".zip", ".gz", ".br", ".zst",
		".woff", ".woff2",
	},
}

// Compress returns a middleware that gzip-compresses responses.
func Compress() astra.HandlerFunc {
	return CompressWithConfig(DefaultCompressConfig)
}

// CompressWithConfig returns a compress middleware with custom configuration.
func CompressWithConfig(cfg CompressConfig) astra.HandlerFunc {
	if cfg.Level == 0 {
		cfg.Level = gzip.DefaultCompression
	}
	if cfg.MinSize == 0 {
		cfg.MinSize = 1024
	}
	if cfg.ExcludedExtensions == nil {
		cfg.ExcludedExtensions = DefaultCompressConfig.ExcludedExtensions
	}

	// Build extension lookup map for O(1) checks.
	excluded := make(map[string]struct{}, len(cfg.ExcludedExtensions))
	for _, ext := range cfg.ExcludedExtensions {
		excluded[ext] = struct{}{}
	}

	pool := sync.Pool{
		New: func() any {
			w, _ := gzip.NewWriterLevel(io.Discard, cfg.Level)
			return w
		},
	}

	return func(c *astra.Ctx) error {
		// Skip if client does not accept gzip.
		if !acceptsGzip(c.Request()) {
			return nil
		}

		// Skip via configurable skipper.
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return nil
		}

		// Skip Server-Sent Events — gzip breaks streaming.
		if strings.Contains(c.Request().Header.Get("Accept"), "text/event-stream") {
			return nil
		}

		// Skip already-compressed file extensions.
		path := c.Request().URL.Path
		if dot := strings.LastIndex(path, "."); dot != -1 {
			if _, skip := excluded[path[dot:]]; skip {
				return nil
			}
		}

		gw := pool.Get().(*gzip.Writer)
		defer pool.Put(gw)

		// Wrap the real ResponseWriter with a buffering gzip writer.
		grw := &gzipResponseWriter{
			ResponseWriter: c.Writer(),
			gw:             gw,
			minSize:        cfg.MinSize,
			buf:            make([]byte, 0, cfg.MinSize),
			statusCode:     http.StatusOK, // default; overridden by WriteHeader
		}
		gw.Reset(grw.ResponseWriter)

		// Replace the context's writer so handlers write through the gzip layer.
		original := c.Writer()
		c.SetWriter(grw)
		defer func() {
			c.SetWriter(original)
			grw.finish()
		}()

		// Run the rest of the chain.
		c.Next()
		return nil
	}
}

// acceptsGzip reports whether the request accepts gzip encoding.
func acceptsGzip(r *http.Request) bool {
	for _, val := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		if strings.TrimSpace(val) == "gzip" {
			return true
		}
	}
	return false
}

// ─── gzipResponseWriter ───────────────────────────────────────────────────────

// gzipResponseWriter buffers writes below minSize and flushes through gzip
// once we know the response is large enough to benefit from compression.
type gzipResponseWriter struct {
	contract.ResponseWriter

	gw      *gzip.Writer
	minSize int

	buf        []byte  // accumulates until minSize is reached
	statusCode int     // buffered status code — not yet committed to the wire
	compressed bool
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	// Buffer the status code; we'll commit it together with Content-Encoding
	// once we know whether the response will be compressed or not.
	if g.statusCode == 0 {
		g.statusCode = code
	}
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if g.compressed {
		return g.gw.Write(p)
	}

	// Accumulate until we know the total size or until the buffer is big enough.
	g.buf = append(g.buf, p...)
	if len(g.buf) < g.minSize {
		// Not yet enough data to decide — hold in buffer.
		return len(p), nil
	}

	// Threshold reached — switch to compressed mode.
	g.enableCompression()
	return len(p), nil
}

func (g *gzipResponseWriter) enableCompression() {
	if g.compressed {
		return
	}
	g.compressed = true
	// Set headers before committing the status code.
	h := g.ResponseWriter.Header()
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length") // length will differ after compression
	h.Add("Vary", "Accept-Encoding")
	// Now commit the buffered status code so headers go out with Content-Encoding set.
	if g.statusCode != 0 {
		g.ResponseWriter.WriteHeader(g.statusCode)
	}
	// Flush buffered bytes through gzip.
	if len(g.buf) > 0 {
		_, _ = g.gw.Write(g.buf)
		g.buf = g.buf[:0]
	}
}

func (g *gzipResponseWriter) finish() {
	if g.compressed {
		_ = g.gw.Close()
		return
	}
	// Response was below minSize — write uncompressed.
	if len(g.buf) > 0 {
		// Commit the buffered status code before writing body.
		if g.statusCode != 0 {
			g.ResponseWriter.WriteHeader(g.statusCode)
		}
		_, _ = g.ResponseWriter.Write(g.buf)
	}
}

// Flush implements http.Flusher — forces buffered bytes out with compression.
func (g *gzipResponseWriter) Flush() {
	if !g.compressed {
		g.enableCompression()
	}
	_ = g.gw.Flush()
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket upgrades.
func (g *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := g.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}
