package astra

import (
	"errors"
	"net/http"
	"time"
)

// Sentinel errors for the two most common failure paths.
// Returned by the default 404/405 handlers so that defaultErrorHandler can
// detect them by pointer equality and write a pre-built response with 0 allocs.
var (
	errDefaultNotFound         = NewHTTPError(http.StatusNotFound, "404 Not Found")
	errDefaultMethodNotAllowed = NewHTTPError(http.StatusMethodNotAllowed, "405 Method Not Allowed")
)

// Pre-built JSON bodies for the sentinel errors — written directly to the wire
// without any map creation or JSON encoding.
var (
	prebuiltBody404 = []byte(`{"error":"404 Not Found"}`)
	prebuiltBody405 = []byte(`{"error":"405 Method Not Allowed"}`)
)

// writePrebuiltError writes a fixed JSON error body directly to the response
// writer, bypassing map creation and JSON encoding entirely (0 allocs).
func writePrebuiltError(ctx *Ctx, code int, body []byte) {
	h := ctx.writer.Header()
	h["Content-Type"] = ctJSON
	h["Content-Length"] = contentLengthSlice(len(body))
	ctx.writer.WriteHeader(code)
	ctx.writer.Write(body) //nolint:errcheck
}

// injectErrorContext enriches an *AppError with trace/request IDs and service
// name from the request context.  Returns the error unchanged if it is not
// an *AppError, making it safe to call on any error type.
func injectErrorContext(c *Ctx, err error) error {
	ae, ok := err.(*AppError)
	if !ok {
		return err
	}
	clone := *ae
	if clone.TraceID == "" {
		if tid, exists := c.Get("trace_id"); exists {
			if s, ok := tid.(string); ok {
				clone.TraceID = s
			}
		}
	}
	if clone.RequestID == "" {
		clone.RequestID = c.GetString("requestID")
	}
	if clone.Service == "" {
		if svc, exists := c.Get("service"); exists {
			if s, ok := svc.(string); ok {
				clone.Service = s
			}
		}
	}
	if clone.Timestamp.IsZero() {
		clone.Timestamp = time.Now().UTC()
	}
	return &clone
}

// defaultErrorHandler writes a structured JSON error response.
//
// Priority order:
//  1. *AppError — business-layer error with Code + Message + optional Data
//  2. ValidationErrors — field-level validation failures (422)
//  3. *HTTPError — protocol-layer error with status code
//  4. unknown — generic 500; exposes raw message only in dev mode
//
// In prod/staging, 5xx messages are replaced with generic HTTP status text to
// prevent leaking internal details (file paths, SQL, stack frames) to clients.
func defaultErrorHandler(c *Ctx, err error) {
	// Fast paths for sentinel errors: write pre-built bytes, 0 allocs.
	if err == errDefaultNotFound {
		writePrebuiltError(c, http.StatusNotFound, prebuiltBody404)
		return
	}
	if err == errDefaultMethodNotAllowed {
		if allow := c.AllowedMethods(); allow != "" {
			c.SetHeader("Allow", allow)
		}
		writePrebuiltError(c, http.StatusMethodNotAllowed, prebuiltBody405)
		return
	}

	isProdLike := c.app.options.Mode == ModeProd || c.app.options.Mode == ModeStaging

	// Inject context metadata (trace_id, request_id, service) into AppError.
	// Use errors.As to handle embedded *AppError (e.g. *gms/pkg/errors.AppError
	// embeds *AppError — direct type assertion would miss it).
	var injected *AppError
	if errors.As(err, &injected) {
		err = injectErrorContext(c, err)
	}

	// Business-layer error: structured response with Code + Message.
	// Use errors.As to correctly handle embedded AppError types.
	var ae *AppError
	if errors.As(err, &ae) {
		status := ae.HTTPStatus
		if status <= 0 {
			status = http.StatusBadRequest
		}
		msg := ae.Message
		// In prod/staging, suppress 5xx AppError messages — they may contain
		// internal details (DB errors, file paths) that leaked into Message.
		if isProdLike && status >= 500 {
			msg = http.StatusText(status)
		}
		body := Map{
			"code":    ae.Code,
			"message": msg,
		}
		if ae.Data != nil && status < 500 {
			body["data"] = ae.Data
		}
		if len(ae.Details) > 0 && status < 500 {
			body["details"] = ae.Details
		}
		if ae.TraceID != "" {
			body["trace_id"] = ae.TraceID
		}
		if ae.RequestID != "" {
			body["request_id"] = ae.RequestID
		}
		if ae.Service != "" {
			body["service"] = ae.Service
		}
		if !ae.Timestamp.IsZero() {
			body["timestamp"] = ae.Timestamp.Format(time.RFC3339)
		}
		// Only include message_i18n in non-prod or 4xx responses
		if len(ae.MessageI18n) > 0 && (!isProdLike || status < 500) {
			body["message_i18n"] = ae.MessageI18n
		}
		// Wrap in "error" envelope for standardized format
		_ = c.JSON(status, Map{"error": body})
		return
	}

	// Validation errors: 422 with field-level details.
	var ve ValidationErrors
	if errors.As(err, &ve) {
		_ = c.JSON(http.StatusUnprocessableEntity, Map{
			"error":  "Validation failed",
			"fields": ve,
		})
		return
	}

	// HTTP-layer error: status + message.
	if he, ok := err.(*HTTPError); ok {
		msg := he.Message
		// In prod/staging, replace 5xx messages with generic text to prevent
		// leaking internal error details to external clients.
		if isProdLike && he.Code >= 500 {
			msg = http.StatusText(he.Code)
		}
		_ = c.JSON(he.Code, Map{"error": msg})
		return
	}

	// Unknown error: generic 500.
	body := Map{"error": "Internal Server Error"}
	if c.app.options.Mode == ModeDev {
		// In dev mode, expose the raw error message to speed up debugging.
		body["detail"] = err.Error()
	}
	_ = c.JSON(http.StatusInternalServerError, body)
}

func defaultNotFoundHandler(c *Ctx) error {
	return errDefaultNotFound
}

func defaultMethodNotAllowedHandler(c *Ctx) error {
	return errDefaultMethodNotAllowed
}
