package astra

import (
	"errors"
	"net/http"
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

	// Business-layer error: structured response with Code + Message.
	if ae, ok := err.(*AppError); ok {
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
		_ = c.JSON(status, body)
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
