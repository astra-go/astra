// Package binding provides request data binding and validation for Astra.
//
// Supported binding sources and their struct tags:
//
//	JSON body      → json:"name"
//	XML  body      → xml:"name"
//	Form data      → form:"name"
//	Query params   → form:"name" or query:"name" (query takes precedence)
//	Path params    → uri:"name"
//	Request header → header:"Name"  (canonical key; no field-name fallback)
//
// Validation uses struct tags accepted by go-playground/validator/v10:
//
//	validate:"required"
//	validate:"required,min=2,max=100"
//	validate:"email"
//	validate:"oneof=admin user guest"
//
// Swap the validator via binding.SetDefaultValidator with a pre-configured
// *validator.Validate to add
// custom validators, translations, or tag aliases.
package binding

import (
	"net/http"
	"net/url"

	"github.com/astra-go/astra/contract"
)

// DefaultBinder is the concrete implementation of contract.Binder backed by
// this package's body binders, parameter mappers, and DefaultValidator.
type DefaultBinder struct{}

// Default is the package-level DefaultBinder instance wired into App.Options.
var Default contract.Binder = &DefaultBinder{}

// BindForm implements contract.Binder.
func (b *DefaultBinder) BindForm(r *http.Request, obj any) error {
	return Form.Bind(r, obj)
}

// BindQuery implements contract.Binder.
func (b *DefaultBinder) BindQuery(q url.Values, obj any) error {
	return BindQuery(q, obj)
}

// BindPath implements contract.Binder.
func (b *DefaultBinder) BindPath(params []contract.PathParam, obj any) error {
	return BindPath(params, obj)
}

// BindHeader implements contract.Binder.
func (b *DefaultBinder) BindHeader(h http.Header, obj any) error {
	return BindHeader(h, obj)
}

// Validate implements contract.Binder.
func (b *DefaultBinder) Validate(obj any) error {
	return Validate(obj)
}

var _ contract.Binder = (*DefaultBinder)(nil)
