package middleware

import (
	"bytes"
	"net/http"
	"strconv"
	"sync"

	"github.com/astra-go/astra"
	"github.com/goccy/go-json"
)

// ResponseFormatConfig configures the ResponseFormat middleware.
type ResponseFormatConfig struct {
	// Skipper skips wrapping for matched requests (e.g. health checks, SSE).
	Skipper Skipper

	// SuccessCode is the business code written on 2xx responses. Default: 0.
	SuccessCode int

	// ErrorCodeMapper maps an HTTP status code to a business error code.
	// Default: returns the HTTP status code as-is.
	ErrorCodeMapper func(httpStatus int) int

	// RequestIDKey is the context key used to read the request ID.
	// Set to "" to omit the request_id field. Default: "requestID".
	RequestIDKey string

	// OnlyJSON limits wrapping to responses whose Content-Type is
	// application/json. Non-JSON responses pass through unchanged.
	// Default: true.
	OnlyJSON bool
}

// DefaultResponseFormatConfig is the out-of-the-box configuration.
var DefaultResponseFormatConfig = ResponseFormatConfig{
	SuccessCode:     0,
	ErrorCodeMapper: func(s int) int { return s },
	RequestIDKey:    "requestID",
	OnlyJSON:        true,
}

// ResponseFormat wraps every JSON response in a unified envelope:
//
//	{"code": 0, "message": "ok", "data": <original body>, "request_id": "..."}
//
// Error responses (4xx/5xx) use the original body as the message value:
//
//	{"code": 404, "message": {"error": "Not Found"}, "data": null, "request_id": "..."}
//
// Pair with middleware.RequestID() to populate the request_id field.
func ResponseFormat() astra.HandlerFunc {
	return ResponseFormatWithConfig(DefaultResponseFormatConfig)
}

// ResponseFormatWithConfig returns a ResponseFormat middleware with custom config.
func ResponseFormatWithConfig(cfg ResponseFormatConfig) astra.HandlerFunc {
	if cfg.ErrorCodeMapper == nil {
		cfg.ErrorCodeMapper = func(s int) int { return s }
	}

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			c.Next()
			return nil
		}

		// Swap in a buffering writer so we can capture the handler's output.
		bw := acquireBufWriter(c.Writer())
		c.SetWriter(bw)

		c.Next()

		// Restore the original writer before writing the final response.
		c.SetWriter(bw.orig)
		defer releaseBufWriter(bw)

		status := bw.status
		if status == 0 {
			status = http.StatusOK
		}

		// Skip non-JSON responses when OnlyJSON is set.
		if cfg.OnlyJSON && !isJSONContentType(bw.header.Get("Content-Type")) {
			replayResponse(c.Writer(), bw.header, status, bw.buf.Bytes())
			return nil
		}

		// Build the envelope.
		var requestID string
		if cfg.RequestIDKey != "" {
			requestID = c.GetString(cfg.RequestIDKey)
		}

		var envelope envelopeDoc
		if status >= 200 && status < 300 {
			envelope.Code = cfg.SuccessCode
			envelope.Message = "ok"
			if bw.buf.Len() > 0 {
				raw := json.RawMessage(bw.buf.Bytes())
				envelope.Data = &raw
			}
		} else {
			envelope.Code = cfg.ErrorCodeMapper(status)
			if bw.buf.Len() > 0 {
				raw := json.RawMessage(bw.buf.Bytes())
				envelope.Message = &raw
			} else {
				envelope.Message = http.StatusText(status)
			}
		}
		if requestID != "" {
			envelope.RequestID = requestID
		}

		body, encErr := json.Marshal(envelope)
		if encErr != nil {
			// Encoding failure: replay the original response unchanged.
			replayResponse(c.Writer(), bw.header, status, bw.buf.Bytes())
			return nil
		}

		h := c.Writer().Header()
		for k, vv := range bw.header {
			if k == "Content-Length" {
				continue // recalculated below
			}
			for _, v := range vv {
				h.Add(k, v)
			}
		}
		h["Content-Type"] = []string{"application/json; charset=utf-8"}
		h["Content-Length"] = []string{strconv.Itoa(len(body))}
		c.Writer().WriteHeader(status)
		_, _ = c.Writer().Write(body)
		return nil
	}
}

// envelopeDoc is the unified response envelope.
// Message is any because it holds either a string (success/generic error)
// or a *json.RawMessage (structured error body from the handler).
type envelopeDoc struct {
	Code      int    `json:"code"`
	Message   any    `json:"message"`
	Data      any    `json:"data"`
	RequestID string `json:"request_id,omitempty"`
}

// replayResponse writes headers, status, and body to w unchanged.
func replayResponse(w astra.ResponseWriter, header http.Header, status int, body []byte) {
	for k, vv := range header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// ─── buffering ResponseWriter ─────────────────────────────────────────────────

type bufWriter struct {
	orig   astra.ResponseWriter
	header http.Header
	buf    bytes.Buffer
	status int
}

var bufWriterPool = sync.Pool{New: func() any {
	return &bufWriter{header: make(http.Header)}
}}

func acquireBufWriter(orig astra.ResponseWriter) *bufWriter {
	bw := bufWriterPool.Get().(*bufWriter)
	bw.orig = orig
	bw.status = 0
	bw.buf.Reset()
	for k := range bw.header {
		delete(bw.header, k)
	}
	return bw
}

func releaseBufWriter(bw *bufWriter) {
	bw.orig = nil
	bufWriterPool.Put(bw)
}

func (bw *bufWriter) Header() http.Header { return bw.header }

func (bw *bufWriter) WriteHeader(code int) {
	if bw.status == 0 {
		bw.status = code
	}
}

func (bw *bufWriter) Write(b []byte) (int, error) {
	if bw.status == 0 {
		bw.status = http.StatusOK
	}
	return bw.buf.Write(b)
}

func (bw *bufWriter) WriteString(s string) (int, error) {
	if bw.status == 0 {
		bw.status = http.StatusOK
	}
	return bw.buf.WriteString(s)
}

func (bw *bufWriter) Status() int   { return bw.status }
func (bw *bufWriter) Size() int     { return bw.buf.Len() }
func (bw *bufWriter) Written() bool { return bw.status != 0 }

// ─── helpers ──────────────────────────────────────────────────────────────────

func isJSONContentType(ct string) bool {
	return len(ct) >= 16 && ct[:16] == "application/json"
}
