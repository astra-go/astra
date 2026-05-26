package astra

// context_bind.go — request binding and validation methods for Ctx.
//
// Three tiers of binding API:
//   - Bind* / BindPath: decode only, no validation — use when validation
//     is done separately or via a different mechanism.
//   - ShouldBind*: decode + validate — use when you want to inspect the
//     error and craft the HTTP response yourself.
//   - MustBind*: decode + validate + auto-abort — use when you want the
//     framework to send a 400/422 response on failure.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/astra-go/astra/contract"
)

// bindBodyLRPool pools *io.LimitedReader to avoid the per-request heap
// allocation that io.LimitReader / http.MaxBytesReader would otherwise incur.
var bindBodyLRPool = sync.Pool{New: func() any { return new(io.LimitedReader) }}

// xmlBufPool pools *bytes.Buffer for BindXML, symmetric with jsonBufPool used
// by BindJSON. Reusing the buffer avoids one heap allocation per XML request.
var xmlBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// xmlBufMaxCap is the cap threshold above which an XML buffer is not returned
// to xmlBufPool. Same policy as jsonBufMaxCap.
const xmlBufMaxCap = 64 * 1024 // 64 KB

// ─── Binding ─────────────────────────────────────────────────────────────────

// Bind automatically detects the content type and binds the request body to obj.
// It does NOT validate — use ShouldBind for binding + validation.
func (c *Ctx) Bind(obj any) error {
	ct := c.ContentType()
	switch {
	case strings.HasPrefix(ct, "application/json"):
		return c.BindJSON(obj)
	case strings.HasPrefix(ct, "application/xml"), strings.HasPrefix(ct, "text/xml"):
		return c.BindXML(obj)
	default:
		return c.BindForm(obj)
	}
}

// BindJSON decodes the JSON request body into obj.
// Limits the body to Options.MaxJSONBodySize (default 1 MiB) to prevent
// memory exhaustion attacks. Override globally with WithMaxJSONBodySize.
//
// Alloc optimizations applied:
//   - *io.LimitedReader is pooled (Plan B) → saves 1 alloc vs io.LimitReader.
//   - *bytes.Buffer is pooled via jsonBufPool (Plan B) → saves 1 alloc vs io.ReadAll.
//   - Unmarshal from []byte (Plan A) → saves 2 allocs vs json.NewDecoder(*bufio.Reader).
func (c *Ctx) BindJSON(obj any) error {
	if c.req.Body == nil {
		return NewHTTPError(http.StatusBadRequest, "empty request body")
	}
	// Pool a LimitedReader to cap the body at the configured limit.
	lr := bindBodyLRPool.Get().(*io.LimitedReader)
	lr.R, lr.N = c.req.Body, c.app.options.MaxJSONBodySize

	// Reuse the same pool as JSON response encoding; both are short-lived and sequential.
	buf := jsonBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	_, readErr := buf.ReadFrom(lr)
	lr.R = nil
	bindBodyLRPool.Put(lr)

	if readErr != nil {
		jsonBufPool.Put(buf)
		return NewHTTPError(http.StatusBadRequest, readErr.Error()).WithInternal(readErr)
	}

	// Unmarshal from the in-memory slice: eliminates json.NewDecoder + bufio.Reader allocs.
	// The configured serializer (goccy/go-json by default) copies all string fields before
	// returning, so buf can be safely returned to the pool immediately after.
	err := c.app.options.serializer().Unmarshal(buf.Bytes(), obj)
	jsonBufPool.Put(buf)
	if err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error()).WithInternal(err)
	}
	return nil
}

// BindXML decodes the XML request body into obj.
// Limits the body to 1 MiB to prevent memory exhaustion attacks.
// Uses a pooled *bytes.Buffer (xmlBufPool) to avoid per-request allocations,
// symmetric with the pooling strategy in BindJSON.
func (c *Ctx) BindXML(obj any) error {
	if c.req.Body == nil {
		return NewHTTPError(http.StatusBadRequest, "empty request body")
	}
	lr := bindBodyLRPool.Get().(*io.LimitedReader)
	lr.R, lr.N = c.req.Body, 1<<20

	buf := xmlBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	_, readErr := buf.ReadFrom(lr)
	lr.R = nil
	bindBodyLRPool.Put(lr)

	if readErr != nil {
		if buf.Cap() <= xmlBufMaxCap {
			xmlBufPool.Put(buf)
		}
		return NewHTTPError(http.StatusBadRequest, readErr.Error()).WithInternal(readErr)
	}

	err := xml.Unmarshal(buf.Bytes(), obj)
	if buf.Cap() <= xmlBufMaxCap {
		xmlBufPool.Put(buf)
	}
	if err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error()).WithInternal(err)
	}
	return nil
}

// BindForm parses form data and maps fields to obj via "form" struct tags.
func (c *Ctx) BindForm(obj any) error {
	if c.app.options.Binder == nil {
		return ErrSlimMode
	}
	if err := c.req.ParseForm(); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.app.options.Binder.BindForm(c.req, obj)
}

// BindQuery binds URL query parameters to obj.
//
// Tag lookup: `query:"name"` → `form:"name"` → lowercase field name.
// Reuses the per-request queryCache populated by Query/QueryMap to avoid
// re-parsing the URL on repeated calls within the same request.
func (c *Ctx) BindQuery(obj any) error {
	if c.app.options.Binder == nil {
		return ErrSlimMode
	}
	if c.queryCache == nil {
		c.queryCache = c.req.URL.Query()
	}
	return c.app.options.Binder.BindQuery(c.queryCache, obj)
}

// BindPath binds URL path parameters to obj using "uri" struct tags.
//
//	type UserURI struct {
//	    ID int64 `uri:"id"`
//	}
//	var u UserURI
//	if err := c.BindPath(&u); err != nil { ... }
func (c *Ctx) BindPath(obj any) error {
	if c.app.options.Binder == nil {
		return ErrSlimMode
	}
	// c.params is backed by c.paramsArr (inline array) — zero heap allocation.
	// The type conversion to []contract.PathParam is valid because Param is an
	// alias for contract.PathParam, so both types share the same underlying type.
	return c.app.options.Binder.BindPath([]contract.PathParam(c.params), obj)
}

// BindHeader binds request headers into obj using "header" struct tags.
// Tag values are matched canonically (e.g. header:"x-request-id" → "X-Request-Id").
// Fields without an explicit "header" tag are skipped.
//
//	type AuthHeader struct {
//	    Token string `header:"Authorization"`
//	}
func (c *Ctx) BindHeader(obj any) error {
	if c.app.options.Binder == nil {
		return ErrSlimMode
	}
	return c.app.options.Binder.BindHeader(c.req.Header, obj)
}

// BindAll binds path params, query params, and the request body into obj in
// one call. Sources are applied in order: path → query → body.
//
// Use struct tags to indicate each field's source:
//
//	type CreateUserReq struct {
//	    ID     int64  `uri:"id"`           // path
//	    Page   int    `query:"page"`        // query
//	    Name   string `json:"name"`         // body (JSON)
//	}
//	if err := c.BindAll(&req); err != nil { ... }
func (c *Ctx) BindAll(obj any) error {
	if err := c.BindPath(obj); err != nil {
		return err
	}
	if err := c.BindQuery(obj); err != nil {
		return err
	}
	return c.Bind(obj)
}

// ─── Binding + Validation ─────────────────────────────────────────────────────

// ShouldBind detects content type, binds the request to obj, and validates using
// struct tags (validate:"..."). Returns an error without modifying the response.
//
//	type CreateUserReq struct {
//	    Name  string `json:"name"  validate:"required,min=2,max=100"`
//	    Email string `json:"email" validate:"required,email"`
//	}
func (c *Ctx) ShouldBind(obj any) error {
	if err := c.Bind(obj); err != nil {
		return err
	}
	return c.Validate(obj)
}

// ShouldBindJSON binds a JSON body to obj and validates.
func (c *Ctx) ShouldBindJSON(obj any) error {
	if err := c.BindJSON(obj); err != nil {
		return err
	}
	return c.Validate(obj)
}

// ShouldBindXML binds an XML body to obj and validates.
func (c *Ctx) ShouldBindXML(obj any) error {
	if err := c.BindXML(obj); err != nil {
		return err
	}
	return c.Validate(obj)
}

// ShouldBindForm binds form data to obj and validates.
func (c *Ctx) ShouldBindForm(obj any) error {
	if err := c.BindForm(obj); err != nil {
		return err
	}
	return c.Validate(obj)
}

// ShouldBindQuery binds URL query parameters to obj and validates.
func (c *Ctx) ShouldBindQuery(obj any) error {
	if err := c.BindQuery(obj); err != nil {
		return err
	}
	return c.Validate(obj)
}

// ShouldBindPath binds URL path parameters to obj and validates.
//
//	type UserURI struct {
//	    ID int64 `uri:"id" validate:"required,gt=0"`
//	}
func (c *Ctx) ShouldBindPath(obj any) error {
	if err := c.BindPath(obj); err != nil {
		return err
	}
	return c.Validate(obj)
}

// ShouldBindHeader binds request headers to obj and validates.
func (c *Ctx) ShouldBindHeader(obj any) error {
	if err := c.BindHeader(obj); err != nil {
		return err
	}
	return c.Validate(obj)
}

// ShouldBindAll binds path params, query params, and the request body into obj,
// then validates. Equivalent to BindAll + Validate.
func (c *Ctx) ShouldBindAll(obj any) error {
	if err := c.BindAll(obj); err != nil {
		return err
	}
	return c.Validate(obj)
}

// mustValidateAndAbort validates obj and, on failure, aborts the context,
// writes the error response via ErrorHandler, and returns the HTTPError.
// Callers (MustBind*) return nil after this call — the response is already sent.
func (c *Ctx) mustValidateAndAbort(obj any) error {
	if err := c.Validate(obj); err != nil {
		c.Abort()
		var ve ValidationErrors
		var httpErr *HTTPError
		if errors.As(err, &ve) {
			httpErr = ToValidationHTTPError(ve)
		} else {
			httpErr = NewHTTPError(http.StatusUnprocessableEntity, err.Error())
		}
		c.app.options.ErrorHandler(c, httpErr)
	}
	return nil
}

// MustBind is like ShouldBind but automatically aborts and writes the error
// response (via ErrorHandler) on binding or validation failure.
// The returned error is always nil; the caller need not inspect it.
func (c *Ctx) MustBind(obj any) error {
	if err := c.Bind(obj); err != nil {
		c.Abort()
		c.app.options.ErrorHandler(c, err)
		return nil
	}
	return c.mustValidateAndAbort(obj)
}

// MustBindJSON is like ShouldBindJSON but automatically aborts and writes the
// error response on failure. The returned error is always nil.
func (c *Ctx) MustBindJSON(obj any) error {
	if err := c.BindJSON(obj); err != nil {
		c.Abort()
		c.app.options.ErrorHandler(c, err)
		return nil
	}
	return c.mustValidateAndAbort(obj)
}

// MustBindAll is like ShouldBindAll but automatically aborts and writes the
// error response on binding or validation failure. The returned error is always nil.
func (c *Ctx) MustBindAll(obj any) error {
	if err := c.BindAll(obj); err != nil {
		c.Abort()
		c.app.options.ErrorHandler(c, err)
		return nil
	}
	return c.mustValidateAndAbort(obj)
}

// Validate validates obj using its struct tags without any binding.
// Useful for validating objects that were populated by other means.
// Returns ErrSlimMode when called on an App created by NewSlim().
func (c *Ctx) Validate(obj any) error {
	if c.app.options.Binder == nil {
		return ErrSlimMode
	}
	return c.app.options.Binder.Validate(obj)
}
