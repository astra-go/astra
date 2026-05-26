package contract

import (
	"net/http"
	"net/url"
)

// PathParam is a URL path parameter key-value pair passed to Binder.BindPath.
// It mirrors binding.Param but lives in contract so that the Binder interface
// can be defined without importing the binding package.
type PathParam struct {
	Key   string
	Value string
}

// Binder is the interface that covers struct population from request data and
// struct validation.  App wires a concrete implementation (binding.DefaultBinder)
// at startup; Ctx delegates all binding calls through this interface.
//
// Implementing Binder lets callers swap the entire binding stack — for example,
// to use a different struct-tag convention or a different validation library —
// without touching context.go or the framework core.
//
// BindJSON and BindXML are intentionally excluded: they use the standard library
// (encoding/json, encoding/xml) and carry no swappable behavior.
//
// Standard tag-to-source mapping:
//
//	uri:"name"    → path parameter   (BindPath)
//	query:"name"  → URL query param  (BindQuery)
//	form:"name"   → form body field  (BindForm)
//	header:"name" → request header   (BindHeader)
//	json:"name"   → JSON body        (BindJSON — on Ctx)
//	xml:"name"    → XML body         (BindXML  — on Ctx)
type Binder interface {
	// BindForm decodes URL-encoded or multipart form data into obj.
	// Uses the "form" struct tag; falls back to the lowercase field name.
	BindForm(r *http.Request, obj any) error

	// BindQuery decodes URL query parameters into obj.
	// Uses the "query" struct tag, then "form", then lowercase field name.
	BindQuery(q url.Values, obj any) error

	// BindPath decodes URL path parameters into obj.
	// Uses the "uri" struct tag; falls back to the lowercase field name.
	BindPath(params []PathParam, obj any) error

	// BindHeader decodes request headers into obj.
	// Uses the "header" struct tag with canonical header key matching
	// (e.g. header:"x-request-id" → "X-Request-Id").
	// Fields without an explicit "header" tag are skipped.
	BindHeader(h http.Header, obj any) error

	// Validate validates obj using its struct tags (e.g. validate:"required").
	// Returns ValidationErrors when one or more fields fail, or another error
	// for unexpected failures.
	Validate(obj any) error
}
